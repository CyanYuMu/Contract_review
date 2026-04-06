package es

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	jsoniter "github.com/json-iterator/go"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

type Kind string

const (
	Create Kind = "create"
	Update Kind = "update"
	Delete Kind = "delete"
)

type ChangeItem struct {
	Kind   Kind
	Id     string
	Parent string
	Data   interface{}
}

type UpdateMode string

const (
	// 如果不存在则跳过
	UpdateModeSkipNotExist UpdateMode = "skip_not_exist"
	// 如果不存在则创建
	UpdateModeCreateNotExist UpdateMode = "create_not_exist"
)

// SyncOption 同步选项配置
type SyncOption struct {
	ForceFlush bool
	// 重试配置
	MaxRetries            int           // 最大重试次数
	RetryDelay            time.Duration // 初始重试延迟
	MaxDelay              time.Duration // 最大重试延迟
	UpdateMode            UpdateMode    // 更新的策略
	IgnoreVersionConflict bool          // 是否忽略版本冲突, 默认 false
}

// 默认配置
var defaultSyncOption = &SyncOption{
	// 默认不强制刷新
	ForceFlush: false,
	// 默认不重试
	MaxRetries: 0,
	// 默认重试延迟
	RetryDelay: 100 * time.Millisecond,
	// 默认最大重试延迟
	MaxDelay:              2 * time.Second,
	IgnoreVersionConflict: false,
}

// getOption 获取同步选项，如果未提供则使用默认值
func getOption(opts ...*SyncOption) *SyncOption {
	if len(opts) > 0 && opts[0] != nil {
		opt := opts[0]
		// 使用默认值填充未设置的字段
		if opt.MaxRetries == 0 {
			opt.MaxRetries = defaultSyncOption.MaxRetries
		}
		if opt.RetryDelay == 0 {
			opt.RetryDelay = defaultSyncOption.RetryDelay
		}
		if opt.MaxDelay == 0 {
			opt.MaxDelay = defaultSyncOption.MaxDelay
		}

		return opt
	}
	return defaultSyncOption
}

// 定义错误类型
var (
	ErrEmptyRequest = errors.New("empty request")
	ErrBulkFailed   = errors.New("bulk operation failed")
	ErrInvalidData  = errors.New("invalid data format")
)

// ProcessStats 处理统计信息
type ProcessStats struct {
	IndexOps   int // 索引操作数量
	UpdateOps  int // 更新操作数量
	DeleteOps  int // 删除操作数量
	SkippedOps int // 跳过操作数量
}

// BulkResponse ES bulk 操作的响应结构
type BulkResponse struct {
	Took   int  `json:"took"`
	Errors bool `json:"errors"`
	Items  []struct {
		Index  *BulkItemResponse `json:"index,omitempty"`
		Update *BulkItemResponse `json:"update,omitempty"`
		Delete *BulkItemResponse `json:"delete,omitempty"`
	} `json:"items"`
}

type BulkItemResponse struct {
	ID     string `json:"_id"`
	Status int    `json:"status"`
	Error  struct {
		Type   string `json:"type"`
		Reason string `json:"reason"`
	} `json:"error,omitempty"`
}

// Sync 优化后的实现
func Sync(ctx context.Context, es *elasticsearch.Client, idx string, items []*ChangeItem, opt ...*SyncOption) error {
	if len(items) == 0 {
		return nil
	}

	option := getOption(opt...)

	// 在进入存在性检查与构建 Bulk 请求之前，先做“仅冲突合并”预处理：
	// 场景：同一批次内，同一文档 id 同时出现 Create 与 Update 时，将数据进行深度合并，
	// 以减少 Update 因文档尚不存在而被忽略的问题。为降低开销，仅对这类冲突文档执行合并。
	items = mergeConflictingItems(items)

	// 如果配置了 UpdateMode，需要先检查更新项的存在性
	var existingDocs map[string]bool
	if option.UpdateMode != "" {
		var err error
		existingDocs, err = checkUpdateItemsExistence(ctx, es, idx, items, option)
		if err != nil {
			su_logger.Error(ctx, err, "check update items existence failed", su_logger.E().String("index", idx))
			return fmt.Errorf("check update items existence failed: %w", err)
		}
	}

	// 使用 strings.Builder 预分配内存
	var (
		indexBody  = newStringBuilderWithSize(len(items) * 2 * 100)
		updateBody = newStringBuilderWithSize(len(items) * 2 * 100)
		deleteBody = newStringBuilderWithSize(len(items) * 50)
	)

	// 处理每个变更项
	stats, err := processItems(items, indexBody, updateBody, deleteBody, option, existingDocs)
	if err != nil {
		return fmt.Errorf("process items failed: %w", err)
	}

	// 记录处理统计信息
	su_logger.Debug(ctx, "processed sync items", su_logger.E().
		String("index", idx).
		Int("total", len(items)).
		Int("index_ops", stats.IndexOps).
		Int("update_ops", stats.UpdateOps).
		Int("delete_ops", stats.DeleteOps).
		Int("skipped_ops", stats.SkippedOps))

	// 合并所有请求
	allRequest := strings.Builder{}
	allRequest.WriteString(indexBody.String())
	allRequest.WriteString(updateBody.String())
	allRequest.WriteString(deleteBody.String())

	if allRequest.Len() == 0 {
		return ErrEmptyRequest
	}

	// 执行 bulk 请求
	resp, err := executeBulkRequest(ctx, es, idx, allRequest.String(), option)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 处理响应
	if err := handleBulkResponse(ctx, resp, option); err != nil {
		return err
	}

	// 处理刷新选项
	if option.ForceFlush {
		if err := Flush(ctx, es, idx); err != nil {
			return err
		}
	}

	return nil
}

func newStringBuilderWithSize(size int) *strings.Builder {
	sb := &strings.Builder{}
	sb.Grow(size)
	return sb
}

// checkUpdateItemsExistence 检查更新项的存在性
func checkUpdateItemsExistence(ctx context.Context, es *elasticsearch.Client, idx string, items []*ChangeItem, option *SyncOption) (map[string]bool, error) {
	// 收集所有需要检查的更新项
	var updateItems []*ChangeItem
	for _, item := range items {
		if item.Kind == Update {
			updateItems = append(updateItems, item)
		}
	}

	if len(updateItems) == 0 {
		return map[string]bool{}, nil
	}

	// 提取所有需要检查的文档ID
	ids := make([]string, 0, len(updateItems))
	for _, item := range updateItems {
		// 处理可能包含路由信息的ID
		if item.Parent != "" {
			ids = append(ids, item.Parent+"/"+item.Id)
		} else {
			ids = append(ids, item.Id)
		}
	}

	// 检查文档存在性
	existingDocs, err := CheckDocsExist(ctx, es, idx, ids)
	if err != nil {
		su_logger.Error(ctx, err, "check docs exist failed", su_logger.E().String("index", idx).Int("count", len(ids)))
		return nil, err
	}

	su_logger.Debug(ctx, "checked update items existence", su_logger.E().String("index", idx).Int("total", len(ids)).Int("existing", countExisting(existingDocs)))

	return existingDocs, nil
}

// countExisting 统计存在的文档数量
func countExisting(existingDocs map[string]bool) int {
	count := 0
	for _, exists := range existingDocs {
		if exists {
			count++
		}
	}
	return count
}

func processItems(items []*ChangeItem, indexBody, updateBody, deleteBody *strings.Builder, option *SyncOption, existingDocs map[string]bool) (*ProcessStats, error) {
	stats := &ProcessStats{}

	// 处理每个变更项
	for _, item := range items {
		switch item.Kind {
		case Delete:
			writeDeleteAction(deleteBody, item)
			stats.DeleteOps++
		case Update:
			skipped, err := writeUpdateAction(updateBody, item, option, existingDocs)
			if err != nil {
				return nil, fmt.Errorf("write update action failed: %w", err)
			}
			if skipped {
				stats.SkippedOps++
			} else {
				stats.UpdateOps++
			}
		default: // Create
			if err := writeIndexAction(indexBody, item); err != nil {
				return nil, fmt.Errorf("write index action failed: %w", err)
			}
			stats.IndexOps++
		}
	}
	return stats, nil
}

func writeDeleteAction(b *strings.Builder, item *ChangeItem) {
	b.WriteString(`{"delete":{"_id":"`)
	b.WriteString(item.Id)
	b.WriteString(`"}}`)
	b.WriteString("\n")
}

func ConvertDataToString(data interface{}) (string, error) {
	switch v := data.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	default:
		return jsoniter.MarshalToString(data)
	}
}

func writeUpdateAction(b *strings.Builder, item *ChangeItem, option *SyncOption, existingDocs map[string]bool) (bool, error) {
	// 如果配置了 UpdateMode，需要检查文档是否存在
	if option.UpdateMode != "" {
		// 构建用于查找的键
		var lookupKey string
		if item.Parent != "" {
			lookupKey = item.Parent + "/" + item.Id
		} else {
			lookupKey = item.Id
		}

		// 检查文档是否存在
		exists, found := existingDocs[lookupKey]
		if !found {
			// 如果找不到记录，说明检查过程中出现了问题
			return false, fmt.Errorf("document existence check failed for id: %s", item.Id)
		}

		// 根据 UpdateMode 策略处理
		switch option.UpdateMode {
		case UpdateModeSkipNotExist:
			if !exists {
				// 文档不存在，跳过更新
				su_logger.Debug(context.Background(), "skipping update for non-existent document", su_logger.E().String("id", item.Id).String("parent", item.Parent))
				return true, nil
			}
		case UpdateModeCreateNotExist:
			if !exists {
				// 文档不存在，改为创建操作
				su_logger.Debug(context.Background(), "converting update to create for non-existent document", su_logger.E().String("id", item.Id).String("parent", item.Parent))
				err := writeIndexAction(b, item)
				return false, err
			}
		}
	}

	// 写入 update action 元数据
	if item.Parent != "" {
		fmt.Fprintf(b, `{"update":{"_id":"%s","routing":"%s"}}`, item.Id, item.Parent)
	} else {
		fmt.Fprintf(b, `{"update":{"_id":"%s"}}`, item.Id)
	}
	b.WriteString("\n")

	// 写入数据
	data, err := ConvertDataToString(item.Data)
	if err != nil {
		return false, fmt.Errorf("convert update data failed: %w", err)
	}
	b.WriteString(`{"doc":`)
	b.WriteString(data)
	// UpdateModeCreateNotExist: 防止存在性预检查与 bulk update 之间出现竞态（或误判）导致的 document_missing_exception
	// 使用 doc_as_upsert 让 update 在文档不存在时也能创建。
	if option.UpdateMode == UpdateModeCreateNotExist {
		b.WriteString(`,"doc_as_upsert":true`)
	}
	b.WriteString("}")
	b.WriteString("\n")
	return false, nil
}

func writeIndexAction(b *strings.Builder, item *ChangeItem) error {
	// 写入 action 元数据
	if item.Parent != "" {
		fmt.Fprintf(b, `{"index":{"_id":"%s","routing":"%s"}}`, item.Id, item.Parent)
	} else {
		fmt.Fprintf(b, `{"index":{"_id":"%s"}}`, item.Id)
	}
	b.WriteString("\n")

	// 写入数据
	data, err := ConvertDataToString(item.Data)
	if err != nil {
		return fmt.Errorf("convert index data failed: %w", err)
	}
	b.WriteString(data)
	b.WriteString("\n")
	return nil
}

// mergeConflictingItems 仅对同一批次内出现“同一文档同时 Create 与 Update”的冲突进行合并，
// 将多个 Update 的字段按出现顺序叠加覆盖到 Create 的文档上，然后删除对应的 Update 项，
// 以避免 Update 因文档尚未存在而被忽略。对无冲突的文档不做任何处理以降低额外开销。
func mergeConflictingItems(items []*ChangeItem) []*ChangeItem {
	// key: parent/id 唯一标识，空 parent 用空字符串
	type key struct{ parent, id string }

	// 聚合每个文档的 create 与 update
	type agg struct {
		createIdx   int
		createData  interface{}
		updateIdxes []int
		updateData  []interface{}
	}

	aggs := make(map[key]*agg, 8)

	for i, it := range items {
		k := key{parent: it.Parent, id: it.Id}
		a := aggs[k]
		if a == nil {
			a = &agg{createIdx: -1}
			aggs[k] = a
		}
		switch it.Kind {
		case Create:
			// 只保留第一条 create，后续 create 视为一次更新覆盖
			if a.createIdx == -1 {
				a.createIdx = i
				a.createData = it.Data
			} else {
				// 多个 create 罕见，按 update 叠加语义处理
				a.updateIdxes = append(a.updateIdxes, i)
				a.updateData = append(a.updateData, it.Data)
			}
		case Update:
			a.updateIdxes = append(a.updateIdxes, i)
			a.updateData = append(a.updateData, it.Data)
		}
	}

	// 仅对同时存在 create 与 update 的 key 做合并
	skip := make([]bool, len(items))
	for k, a := range aggs {
		if a.createIdx < 0 || len(a.updateIdxes) == 0 {
			continue
		}

		// 时序约束：如存在任意 update 在 create 之前，则不处理该 key 的合并
		hasUpdateBeforeCreate := false
		for _, uidx := range a.updateIdxes {
			if uidx < a.createIdx {
				hasUpdateBeforeCreate = true
				break
			}
		}
		if hasUpdateBeforeCreate {
			continue
		}

		// 仅合并 create 之后的 update（此时所有 update 都在 create 之后）
		// 解析 create 文档为 map
		base, err := parseToMap(a.createData)
		if err != nil {
			su_logger.Error(context.Background(), err, "merge create+update parse create failed",
				su_logger.E().String("id", k.id).String("parent", k.parent))
			continue // 出错则放弃合并，保持原样
		}

		merged := base
		for _, ud := range a.updateData {
			uMap, err := parseToMap(ud)
			if err != nil {
				su_logger.Error(context.Background(), err, "merge create+update parse update failed",
					su_logger.E().String("id", k.id).String("parent", k.parent))
				continue // 跳过解析失败的该条 update
			}
			merged = deepMergeMap(merged, uMap)
		}

		// 将合并结果写回 create 项的数据
		items[a.createIdx].Data = merged

		// 标记所有 update 项为跳过（这些 update 均位于 create 之后）
		for _, idx := range a.updateIdxes {
			skip[idx] = true
		}
	}

	// 生成过滤后的结果切片
	res := make([]*ChangeItem, 0, len(items))
	for i, it := range items {
		if skip[i] {
			continue
		}
		res = append(res, it)
	}
	return res
}

// parseToMap 将任意数据转为 map[string]interface{}，用于合并
func parseToMap(v interface{}) (map[string]interface{}, error) {
	switch t := v.(type) {
	case nil:
		return map[string]interface{}{}, nil
	case map[string]any:
		m := make(map[string]interface{}, len(t))
		for k, vv := range t {
			m[k] = vv
		}
		return m, nil
	case []byte:
		var m map[string]interface{}
		if err := jsoniter.Unmarshal(t, &m); err != nil {
			return nil, err
		}
		return m, nil
	case string:
		var m map[string]interface{}
		if err := jsoniter.UnmarshalFromString(t, &m); err != nil {
			return nil, err
		}
		return m, nil
	default:
		// 其他结构体等，先序列化再反序列化
		bs, err := jsoniter.Marshal(v)
		if err != nil {
			return nil, err
		}
		var m map[string]interface{}
		if err := jsoniter.Unmarshal(bs, &m); err != nil {
			return nil, err
		}
		return m, nil
	}
}

// deepMergeMap 深度合并 src 到 dst，返回新的 map（在 dst 基础上修改并返回）
func deepMergeMap(dst, src map[string]interface{}) map[string]interface{} {
	if dst == nil {
		dst = map[string]interface{}{}
	}
	for k, sv := range src {
		if dv, ok := dst[k]; ok {
			dMap, dIsMap := dv.(map[string]interface{})
			sMap, sIsMap := sv.(map[string]interface{})
			if !dIsMap {
				// 兼容 map[string]any
				if mm, ok2 := dv.(map[string]any); ok2 {
					dMap = make(map[string]interface{}, len(mm))
					for kk, vv := range mm {
						dMap[kk] = vv
					}
					dIsMap = true
				}
			}
			if !sIsMap {
				if mm, ok2 := sv.(map[string]any); ok2 {
					sMap = make(map[string]interface{}, len(mm))
					for kk, vv := range mm {
						sMap[kk] = vv
					}
					sIsMap = true
				}
			}
			if dIsMap && sIsMap {
				dst[k] = deepMergeMap(dMap, sMap)
				continue
			}
		}
		// 非嵌套或不存在，直接覆盖
		dst[k] = sv
	}
	return dst
}

func executeBulkRequest(ctx context.Context, es *elasticsearch.Client, idx, body string, opt *SyncOption) (*esapi.Response, error) {
	request := esapi.BulkRequest{
		Index: idx,
		Body:  strings.NewReader(body),
	}

	var lastErr error
	delay := opt.RetryDelay

	for retry := 0; retry <= opt.MaxRetries; retry++ {
		resp, err := request.Do(ctx, es)
		if err == nil {
			return resp, nil
		}

		lastErr = err
		if retry == opt.MaxRetries {
			break
		}

		// 指数退避
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}

		delay *= 2
		if delay > opt.MaxDelay {
			delay = opt.MaxDelay
		}
	}

	return nil, fmt.Errorf("after %d retries: %w", opt.MaxRetries, lastErr)
}

func handleBulkResponse(ctx context.Context, resp *esapi.Response, opt *SyncOption) error {
	if resp.IsError() {
		return fmt.Errorf("%s: %s", ErrBulkFailed, resp.String())
	}

	var bulkResp BulkResponse
	if err := jsoniter.NewDecoder(resp.Body).Decode(&bulkResp); err != nil {
		return fmt.Errorf("decode response failed: %w", err)
	}

	if bulkResp.Errors {
		// 收集所有失败的操作
		var failures []string
		ignored := 0
		for _, item := range bulkResp.Items {
			if isIgnorableBulkItemFailure(item, opt) {
				ignored++
				continue
			}
			if failure := getItemFailure(item); failure != "" {
				failures = append(failures, failure)
			}
		}
		if ignored > 0 {
			su_logger.Debug(ctx, "ignored bulk item failures", su_logger.E().Int("ignored", ignored))
		}
		if len(failures) > 0 {
			return fmt.Errorf("%s: %s", ErrBulkFailed, strings.Join(failures, "; "))
		}
	}

	return nil
}

func isIgnorableBulkItemFailure(item struct {
	Index  *BulkItemResponse `json:"index,omitempty"`
	Update *BulkItemResponse `json:"update,omitempty"`
	Delete *BulkItemResponse `json:"delete,omitempty"`
}, opt *SyncOption) bool {
	if opt == nil || !opt.IgnoreVersionConflict {
		return false
	}

	var resp *BulkItemResponse
	if item.Index != nil {
		resp = item.Index
	} else if item.Update != nil {
		resp = item.Update
	} else if item.Delete != nil {
		resp = item.Delete
	}
	if resp == nil || resp.Error.Type == "" {
		return false
	}

	// ES 常见版本冲突错误：status=409 + type=version_conflict_engine_exception
	if resp.Status == 409 || resp.Error.Type == "version_conflict_engine_exception" {
		return true
	}
	return false
}

func getItemFailure(item struct {
	Index  *BulkItemResponse `json:"index,omitempty"`
	Update *BulkItemResponse `json:"update,omitempty"`
	Delete *BulkItemResponse `json:"delete,omitempty"`
}) string {
	var resp *BulkItemResponse
	if item.Index != nil {
		resp = item.Index
	} else if item.Update != nil {
		resp = item.Update
	} else if item.Delete != nil {
		resp = item.Delete
	}

	if resp != nil && resp.Error.Type != "" {
		return fmt.Sprintf("id=%s, type=%s, reason=%s",
			resp.ID, resp.Error.Type, resp.Error.Reason)
	}
	return ""
}

func Flush(ctx context.Context, es *elasticsearch.Client, idx string) error {
	req := esapi.IndicesRefreshRequest{
		Index: []string{idx},
	}
	resp, err := req.Do(ctx, es)
	if err != nil {
		return fmt.Errorf("refresh index failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.IsError() {
		return fmt.Errorf("refresh index failed: %s", resp.String())
	}
	return nil
}

// func isSuccess(s string) bool {
// 	return strings.Contains(s, "\"errors\":false")
// }

// 解析响应
type CheckDocsExistResult struct {
	Docs []struct {
		ID    string `json:"_id"`
		Found bool   `json:"found"`
	} `json:"docs"`
}

func CheckDocsExist(ctx context.Context, client *elasticsearch.Client, index string, ids []string) (map[string]bool, error) {
	if len(ids) == 0 {
		return map[string]bool{}, nil
	}

	// 构建 mget 请求
	body := strings.Builder{}
	body.WriteString(`{"docs":[`)

	for i, id := range ids {
		if i > 0 {
			body.WriteString(",")
		}

		// 检查ID是否包含路由信息（格式：路由/ID）
		parts := strings.SplitN(id, "/", 2)
		if len(parts) == 2 {
			// 包含路由信息，使用 routing 参数
			body.WriteString(fmt.Sprintf(`{"_id":"%s","routing":"%s"}`, parts[1], parts[0]))
		} else {
			// 不包含路由信息，只使用 ID
			body.WriteString(fmt.Sprintf(`{"_id":"%s"}`, id))
		}
	}

	body.WriteString(`]}`)

	// 执行 mget 请求
	res, err := client.Mget(
		strings.NewReader(body.String()),
		client.Mget.WithContext(ctx),
		client.Mget.WithIndex(index),
		client.Mget.WithSourceIncludes("_id"), // 只需要检查存在性
	)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	var result CheckDocsExistResult
	if err := jsoniter.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, err
	}

	// 构建结果映射
	exists := make(map[string]bool)
	for i, doc := range result.Docs {
		// 使用原始ID作为键，包括可能的路由信息
		exists[ids[i]] = doc.Found
	}

	return exists, nil
}

// BatchDelete 批量删除文档
// 返回删除操作的响应和错误
func BatchDelete(ctx context.Context, client *elasticsearch.Client, index string, ids []string) (*BulkResponse, error) {
	if len(ids) == 0 {
		return nil, ErrEmptyRequest
	}

	// 构建批量删除请求体
	body := strings.Builder{}
	body.Grow(len(ids) * 50) // 预分配内存

	for _, id := range ids {
		// 检查ID是否包含路由信息（格式：路由/ID）
		parts := strings.SplitN(id, "/", 2)
		if len(parts) == 2 {
			// 包含路由信息
			body.WriteString(`{"delete":{"_id":"`)
			body.WriteString(parts[1])
			body.WriteString(`","routing":"`)
			body.WriteString(parts[0])
			body.WriteString(`"}}`)
		} else {
			// 不包含路由信息
			body.WriteString(`{"delete":{"_id":"`)
			body.WriteString(id)
			body.WriteString(`"}}`)
		}
		body.WriteString("\n")
	}

	// 执行 bulk 请求
	request := esapi.BulkRequest{
		Index: index,
		Body:  strings.NewReader(body.String()),
	}

	resp, err := request.Do(ctx, client)
	if err != nil {
		su_logger.Error(ctx, err, "batch delete failed", su_logger.E().String("index", index).Int("count", len(ids)))
		return nil, fmt.Errorf("batch delete failed: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.IsError() {
		su_logger.Error(ctx, nil, "batch delete response error", su_logger.E().String("index", index).String("status", resp.Status()))
		return nil, fmt.Errorf("%s: %s", ErrBulkFailed, resp.String())
	}

	// 解析响应
	var bulkResp BulkResponse
	if err := jsoniter.NewDecoder(resp.Body).Decode(&bulkResp); err != nil {
		su_logger.Error(ctx, err, "decode batch delete response failed", su_logger.E().String("index", index))
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	// 检查是否有错误
	if bulkResp.Errors {
		var failures []string
		for _, item := range bulkResp.Items {
			if failure := getItemFailure(item); failure != "" {
				failures = append(failures, failure)
			}
		}
		if len(failures) > 0 {
			su_logger.Error(ctx, nil, "batch delete has failures", su_logger.E().String("index", index).Int("failures", len(failures)))
			return &bulkResp, fmt.Errorf("%s: %s", ErrBulkFailed, strings.Join(failures, "; "))
		}
	}

	su_logger.Debug(ctx, "batch delete succeeded", su_logger.E().String("index", index).Int("count", len(ids)))
	return &bulkResp, nil
}
