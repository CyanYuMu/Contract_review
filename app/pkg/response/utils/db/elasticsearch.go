package db

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	es "github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	jsoniter "github.com/json-iterator/go"
	"github.com/tidwall/gjson"
	es2 "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/es"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

// 常量定义
const (
	maxRecursionDepth = 3
	esTagKey          = "es"
	sourceField       = "_source"
	idField           = "_id"
	foundField        = "found"
	hitsField         = "hits.hits"
	countField        = "count"
	responsePrefix    = "[200 OK] "
)

// 错误定义
var (
	ErrInvalidRowType  = errors.New("data must implement Row interface")
	ErrNoSourceData    = errors.New("no _source data in document")
	ErrInvalidResponse = errors.New("invalid elasticsearch response")
)

// FieldInfo 字段信息缓存，优化内存对齐
type FieldInfo struct {
	EsTag    string
	Type     reflect.Type
	Path     []int
	IsStruct bool
}

// TypeCache 类型缓存，使用更高效的数据结构
type TypeCache struct {
	Fields    []FieldInfo
	fieldMap  map[string]*FieldInfo // 快速查找
	hasStruct bool                  // 是否包含结构体字段
}

// 全局缓存，使用读写锁优化并发性能
var (
	fieldCache = sync.Map{} // map[reflect.Type]*TypeCache
)

// DocumentProcessor 文档处理器接口
type DocumentProcessor interface {
	ProcessDocument(hit gjson.Result) (*DocumentRef, error)
}

// ResponseParser ES响应解析器接口
type ResponseParser interface {
	ParseSearchResponse(body string) ([]gjson.Result, error)
	ParseGetResponse(body string) (gjson.Result, error)
	ParseCountResponse(body string) (int64, error)
}

// FieldExtractor 字段提取器接口
type FieldExtractor interface {
	ExtractFields(row Row) (map[string]interface{}, error)
	ExtractUpdateFields(data interface{}, options *UpdateOptions) ([]Update, error)
}

// DefaultDocumentProcessor 默认文档处理器
type DefaultDocumentProcessor struct{}

func (p *DefaultDocumentProcessor) ProcessDocument(hit gjson.Result) (*DocumentRef, error) {
	docID := hit.Get(idField).String()
	sourceData := hit.Get(sourceField).String()

	if sourceData == "" {
		return nil, nil // 跳过无效文档
	}

	return &DocumentRef{
		ID:   docID,
		Data: RowData([]byte(sourceData)),
	}, nil
}

// DefaultResponseParser 默认响应解析器
type DefaultResponseParser struct{}

func (p *DefaultResponseParser) ParseSearchResponse(body string) ([]gjson.Result, error) {
	result := gjson.Parse(body)
	return result.Get(hitsField).Array(), nil
}

func (p *DefaultResponseParser) ParseGetResponse(body string) (gjson.Result, error) {
	return gjson.Parse(body), nil
}

func (p *DefaultResponseParser) ParseCountResponse(body string) (int64, error) {
	result := gjson.Parse(body)
	return result.Get(countField).Int(), nil
}

// CachedFieldExtractor 缓存字段提取器
type CachedFieldExtractor struct{}

func (e *CachedFieldExtractor) ExtractFields(row Row) (map[string]interface{}, error) {
	typ := reflect.TypeOf(row)
	val := reflect.ValueOf(row)

	cache := getOrCreateTypeCache(typ)
	result := make(map[string]interface{}, len(cache.fieldMap))

	return result, extractFieldsWithCache(val, cache.Fields, result)
}

func (e *CachedFieldExtractor) ExtractUpdateFields(data interface{}, options *UpdateOptions) ([]Update, error) {
	var updates []Update
	err := extractEsFieldsGeneric(reflect.ValueOf(data), reflect.TypeOf(data), options, func(fieldName string, value interface{}) {
		updates = append(updates, Update{
			Field: fieldName,
			Value: value,
		})
	})
	return updates, err
}

// EsDB ES数据库实现，优化结构布局
type ElasticsearchDB struct {
	client         *es.Client
	docProcessor   DocumentProcessor
	responseParser ResponseParser
	fieldExtractor FieldExtractor
}

// NewEsDB 创建优化的EsDB实例
func NewElasticsearchDB(client *es.Client) *ElasticsearchDB {
	return &ElasticsearchDB{
		client:         client,
		docProcessor:   &DefaultDocumentProcessor{},
		responseParser: &DefaultResponseParser{},
		fieldExtractor: &CachedFieldExtractor{},
	}
}

// NewEsDBWithOptions 创建带选项的EsDB实例
func NewEsDBWithOptions(client *es.Client, processor DocumentProcessor, parser ResponseParser, extractor FieldExtractor) *ElasticsearchDB {
	return &ElasticsearchDB{
		client:         client,
		docProcessor:   processor,
		responseParser: parser,
		fieldExtractor: extractor,
	}
}

// Insert 插入数据，使用优化的字段提取
func (e *ElasticsearchDB) Insert(ctx context.Context, table string, row Row, options ...*InsertOptions) (string, error) {
	result, err := e.Create(ctx, table, row, nil)
	if err != nil {
		return "", fmt.Errorf("insert failed: %w", err)
	}
	return result.ID, nil
}

// Delete 删除数据，使用批量删除优化单个删除
func (e *ElasticsearchDB) Delete(ctx context.Context, table string, id string, options ...*DeleteOptions) (int, error) {
	if id == "" {
		return 0, errors.New("id cannot be empty")
	}

	changeItem := &es2.ChangeItem{
		Kind:   es2.Delete,
		Id:     id,
		Parent: "",
	}

	if err := es2.Sync(ctx, e.client, table, []*es2.ChangeItem{changeItem}); err != nil {
		return 0, fmt.Errorf("delete failed for id %s: %w", id, err)
	}

	return 1, nil
}

// Create 创建数据，使用优化的字段提取和错误处理
func (e *ElasticsearchDB) Create(ctx context.Context, table string, row Row, options ...*CreateOptions) (*CreateResult, error) {
	if row == nil {
		return nil, errors.New("row cannot be nil")
	}

	// 使用优化的字段提取器
	esData, err := e.fieldExtractor.ExtractFields(row)
	if err != nil {
		return nil, fmt.Errorf("failed to extract fields: %w", err)
	}

	// 使用对象池优化JSON序列化
	jsonData, err := jsoniter.MarshalToString(esData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	changeItem := &es2.ChangeItem{
		Kind:   es2.Create,
		Id:     row.ID(),
		Parent: "",
		Data:   jsonData,
	}

	if err := es2.Sync(ctx, e.client, table, []*es2.ChangeItem{changeItem}); err != nil {
		return nil, fmt.Errorf("sync failed: %w", err)
	}

	return &CreateResult{ID: row.ID()}, nil
}

// Update 更新数据，使用优化的数据构建和错误处理
func (e *ElasticsearchDB) Update(ctx context.Context, table string, update UpdateOne, options ...*UpdateOptions) error {
	if update.ID == "" {
		return errors.New("update ID cannot be empty")
	}
	if len(update.Updates) == 0 {
		return errors.New("no update fields provided")
	}

	// 使用预分配的map优化内存
	updateData := make(map[string]interface{}, len(update.Updates))
	for _, u := range update.Updates {
		if u.Field != "" { // 防止空字段名
			updateData[u.Field] = u.Value
		}
	}

	if len(updateData) == 0 {
		return errors.New("no valid update fields")
	}

	// 序列化更新数据
	jsonData, err := jsoniter.MarshalToString(updateData)
	if err != nil {
		return fmt.Errorf("failed to marshal update data: %w", err)
	}

	changeItem := &es2.ChangeItem{
		Kind:   es2.Update,
		Id:     update.ID,
		Parent: "",
		Data:   jsonData,
	}

	if err := es2.Sync(ctx, e.client, table, []*es2.ChangeItem{changeItem}); err != nil {
		return fmt.Errorf("update sync failed for id %s: %w", update.ID, err)
	}

	return nil
}

// Get 获取单个文档，使用优化的响应解析
func (e *ElasticsearchDB) Get(ctx context.Context, table string, id string, options ...*GetOptions) (*DocumentRef, error) {
	if id == "" {
		return nil, errors.New("document ID cannot be empty")
	}
	if table == "" {
		return nil, errors.New("table name cannot be empty")
	}

	request := esapi.GetRequest{
		Index:      table,
		DocumentID: id,
	}

	response, err := request.Do(ctx, e.client)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch get request failed: %w", err)
	}
	defer response.Body.Close()

	// 使用优化的响应解析
	responseBody, err := parseESResponse(response, true)
	if err != nil {
		return nil, err
	}

	// 使用响应解析器
	result, err := e.responseParser.ParseGetResponse(responseBody)
	if err != nil {
		return nil, fmt.Errorf("failed to parse get response: %w", err)
	}

	// 检查文档是否存在
	if !result.Get(foundField).Bool() {
		return nil, ErrDocNotFound
	}

	// 获取源数据
	sourceData := result.Get(sourceField).String()
	if sourceData == "" {
		return nil, ErrNoSourceData
	}

	return &DocumentRef{
		ID:   id,
		Data: RowData([]byte(sourceData)),
	}, nil
}

// Find 查询数据，使用优化的查询构建和结果处理
func (e *ElasticsearchDB) Find(ctx context.Context, table string, conds Conds, options ...*FindOptions) (*Iterator, error) {
	if table == "" {
		return nil, errors.New("table name cannot be empty")
	}

	// 构建优化的查询
	query, err := e.buildSearchQuery(conds, options...)
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}
	su_logger.Debug(ctx, "search:"+table+" query="+query)

	// 执行搜索请求
	request := esapi.SearchRequest{
		Index:          []string{table},
		Body:           strings.NewReader(query),
		TrackTotalHits: false, // 优化性能，不计算总数
	}

	// 处理返回字段选项
	if len(options) > 0 && options[0] != nil && len(options[0].Fields) > 0 {
		request.Source = options[0].Fields
	}

	response, err := request.Do(ctx, e.client)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer response.Body.Close()

	// 解析响应
	responseBody, err := parseESResponse(response, false)
	if err != nil {
		return nil, fmt.Errorf("failed to parse search response: %w", err)
	}

	// 使用响应解析器处理结果
	hits, err := e.responseParser.ParseSearchResponse(responseBody)
	if err != nil {
		return nil, fmt.Errorf("failed to parse search hits: %w", err)
	}

	// 优化的文档处理
	documentRefs, err := e.processSearchHits(hits)
	if err != nil {
		return nil, fmt.Errorf("failed to process search hits: %w", err)
	}

	// 创建Iterator，使用unsafe设置私有字段
	return e.createIterator(documentRefs), nil
}

// buildSearchQuery 构建搜索查询，提取为独立方法提升可读性
func (e *ElasticsearchDB) buildSearchQuery(conds Conds, options ...*FindOptions) (string, error) {
	builder := es2.NewBuilder(e.client)

	// 添加查询条件
	for _, cond := range conds {
		if cond.Field == "" {
			continue // 跳过空字段
		}
		builder.Where(cond.Field, cond.Cond, cond.Value)
	}

	// 处理查询选项
	var opts *FindOptions
	if len(options) > 0 && options[0] != nil {
		opts = options[0]
	}

	if opts != nil {
		// 设置分页，使用明确的方法名
		if opts.Limit > 0 {
			builder.SetFrom(int(opts.Offset))
			builder.Size(int(opts.Limit))
		}

		// 设置排序
		for _, sort := range opts.Sorts {
			if sort.Field == "" {
				continue // 跳过空字段排序
			}
			order := "asc"
			if sort.Order < 0 {
				order = "desc"
			}
			builder.OrderBy(sort.Field, order)
		}

		// 设置返回字段
		if len(opts.Fields) > 0 {
			builder.Source(opts.Fields)
		}
	}

	return builder.BuildQuery(), nil
}

// processSearchHits 处理搜索结果，使用文档处理器
func (e *ElasticsearchDB) processSearchHits(hits []gjson.Result) ([]*DocumentRef, error) {
	if len(hits) == 0 {
		return []*DocumentRef{}, nil
	}

	documentRefs := make([]*DocumentRef, 0, len(hits))

	for _, hit := range hits {
		docRef, err := e.docProcessor.ProcessDocument(hit)
		if err != nil {
			return nil, fmt.Errorf("failed to process document: %w", err)
		}

		if docRef != nil { // 文档处理器可能返回nil表示跳过
			documentRefs = append(documentRefs, docRef)
		}
	}

	return documentRefs, nil
}

// createIterator 创建Iterator，直接设置items字段
func (e *ElasticsearchDB) createIterator(documentRefs []*DocumentRef) *Iterator {
	iterator := &Iterator{}
	iterator.items = documentRefs
	return iterator
}

// BatchCreate 批量创建数据，使用优化的并发处理和错误收集
func (e *ElasticsearchDB) BatchCreate(ctx context.Context, table string, rows []Row, options ...*BatchWriteOptions) *BatchWriteResult {
	if len(rows) == 0 {
		return &BatchWriteResult{}
	}
	if table == "" {
		return &BatchWriteResult{
			Errors: []error{errors.New("table name cannot be empty")},
		}
	}

	// 预分配切片，避免多次扩容
	changeItems := make([]*es2.ChangeItem, 0, len(rows))
	var processingErrors []error

	// 批量处理行数据
	for i, row := range rows {
		if row == nil {
			processingErrors = append(processingErrors, fmt.Errorf("row at index %d is nil", i))
			continue
		}

		// 使用字段提取器
		esData, err := e.fieldExtractor.ExtractFields(row)
		if err != nil {
			processingErrors = append(processingErrors, fmt.Errorf("failed to extract fields for row %d: %w", i, err))
			continue
		}

		// 序列化数据
		jsonData, err := jsoniter.MarshalToString(esData)
		if err != nil {
			processingErrors = append(processingErrors, fmt.Errorf("failed to marshal row %d: %w", i, err))
			continue
		}

		changeItems = append(changeItems, &es2.ChangeItem{
			Kind:   es2.Create,
			Id:     row.ID(),
			Parent: "",
			Data:   jsonData,
		})
	}

	// 构建结果
	result := &BatchWriteResult{
		Errors: processingErrors,
	}

	// 如果没有有效的变更项，返回错误结果
	if len(changeItems) == 0 {
		if len(processingErrors) == 0 {
			result.Errors = append(result.Errors, errors.New("no valid rows to process"))
		}
		return result
	}

	// 执行批量同步
	if err := es2.Sync(ctx, e.client, table, changeItems); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("batch sync failed: %w", err))
	}

	return result
}

// BatchInsert 批量插入，复用BatchCreate逻辑
func (e *ElasticsearchDB) BatchInsert(ctx context.Context, table string, rows []Row, options ...*BatchWriteOptions) *BatchWriteResult {
	return e.BatchCreate(ctx, table, rows, options...)
}

// BatchDelete 批量删除数据，使用优化的ID验证和错误处理
func (e *ElasticsearchDB) BatchDelete(ctx context.Context, table string, ids []string, options ...*BatchWriteOptions) *BatchWriteResult {
	if len(ids) == 0 {
		return &BatchWriteResult{}
	}
	if table == "" {
		return &BatchWriteResult{
			Errors: []error{errors.New("table name cannot be empty")},
		}
	}

	// 过滤和验证ID
	validIDs := make([]string, 0, len(ids))
	var processingErrors []error

	for i, id := range ids {
		if id == "" {
			processingErrors = append(processingErrors, fmt.Errorf("id at index %d is empty", i))
			continue
		}
		validIDs = append(validIDs, id)
	}

	result := &BatchWriteResult{
		Errors: processingErrors,
	}

	if len(validIDs) == 0 {
		if len(processingErrors) == 0 {
			result.Errors = append(result.Errors, errors.New("no valid IDs to delete"))
		}
		return result
	}

	// 构建删除操作项
	changeItems := make([]*es2.ChangeItem, 0, len(validIDs))
	for _, id := range validIDs {
		changeItems = append(changeItems, &es2.ChangeItem{
			Kind:   es2.Delete,
			Id:     id,
			Parent: "",
		})
	}

	// 执行批量同步
	if err := es2.Sync(ctx, e.client, table, changeItems); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("batch delete sync failed: %w", err))
	}

	return result
}

// BatchUpdate 批量更新数据，使用优化的数据序列化和错误处理
func (e *ElasticsearchDB) BatchUpdate(ctx context.Context, table string, updates []UpdateOne, options ...*BatchWriteOptions) *BatchWriteResult {
	if len(updates) == 0 {
		return &BatchWriteResult{}
	}
	if table == "" {
		return &BatchWriteResult{
			Errors: []error{errors.New("table name cannot be empty")},
		}
	}

	// 预分配切片和错误收集
	changeItems := make([]*es2.ChangeItem, 0, len(updates))
	var processingErrors []error

	for i, update := range updates {
		if update.ID == "" {
			processingErrors = append(processingErrors, fmt.Errorf("update at index %d has empty ID", i))
			continue
		}
		if len(update.Updates) == 0 {
			processingErrors = append(processingErrors, fmt.Errorf("update at index %d has no update fields", i))
			continue
		}

		// 构建更新数据
		updateData := make(map[string]interface{}, len(update.Updates))
		for _, u := range update.Updates {
			if u.Field != "" {
				updateData[u.Field] = u.Value
			}
		}

		if len(updateData) == 0 {
			processingErrors = append(processingErrors, fmt.Errorf("update at index %d has no valid fields", i))
			continue
		}

		// 序列化更新数据
		jsonData, err := jsoniter.MarshalToString(updateData)
		if err != nil {
			processingErrors = append(processingErrors, fmt.Errorf("failed to marshal update %d: %w", i, err))
			continue
		}

		changeItems = append(changeItems, &es2.ChangeItem{
			Kind:   es2.Update,
			Id:     update.ID,
			Parent: "",
			Data:   jsonData,
		})
	}

	result := &BatchWriteResult{
		Errors: processingErrors,
	}

	if len(changeItems) == 0 {
		if len(processingErrors) == 0 {
			result.Errors = append(result.Errors, errors.New("no valid updates to process"))
		}
		return result
	}

	// 执行批量同步
	if err := es2.Sync(ctx, e.client, table, changeItems); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("batch update sync failed: %w", err))
	}

	return result
}

// BatchGet 批量获取数据，使用优化的Multi Get API
func (e *ElasticsearchDB) BatchGet(ctx context.Context, table string, ids []string, options ...*GetOptions) (*Iterator, error) {
	if len(ids) == 0 {
		return e.createIterator([]*DocumentRef{}), nil
	}
	if table == "" {
		return nil, errors.New("table name cannot be empty")
	}

	// 过滤和验证ID
	validIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" {
			validIDs = append(validIDs, id)
		}
	}

	if len(validIDs) == 0 {
		return e.createIterator([]*DocumentRef{}), nil
	}

	// 构建Multi Get请求体，使用预分配优化
	docs := make([]map[string]interface{}, 0, len(validIDs))
	for _, id := range validIDs {
		docs = append(docs, map[string]interface{}{
			"_index": table,
			"_id":    id,
		})
	}

	requestBody := map[string]interface{}{
		"docs": docs,
	}

	// 序列化请求体
	requestData, err := jsoniter.MarshalToString(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal mget request: %w", err)
	}

	// 执行Multi Get请求
	request := esapi.MgetRequest{
		Body: strings.NewReader(requestData),
	}

	response, err := request.Do(ctx, e.client)
	if err != nil {
		return nil, fmt.Errorf("mget request failed: %w", err)
	}
	defer response.Body.Close()

	// 解析响应
	responseBody, err := parseESResponse(response, false)
	if err != nil {
		return nil, fmt.Errorf("failed to parse mget response: %w", err)
	}

	// 解析文档数组
	result := gjson.Parse(responseBody)
	docsArray := result.Get("docs").Array()

	// 处理文档，使用优化的批量处理
	documentRefs := make([]*DocumentRef, 0, len(docsArray))
	for _, doc := range docsArray {
		if !doc.Get(foundField).Bool() {
			continue // 跳过不存在的文档
		}

		sourceData := doc.Get(sourceField).String()
		if sourceData == "" {
			continue // 跳过无效文档
		}

		docID := doc.Get(idField).String()
		documentRefs = append(documentRefs, &DocumentRef{
			ID:   docID,
			Data: RowData([]byte(sourceData)),
		})
	}

	return e.createIterator(documentRefs), nil
}

// Count 计数查询，使用优化的查询构建和响应解析
func (e *ElasticsearchDB) Count(ctx context.Context, table string, conds Conds) (int64, error) {
	if table == "" {
		return 0, errors.New("table name cannot be empty")
	}

	// 构建计数查询
	query, err := e.buildCountQuery(conds)
	if err != nil {
		return 0, fmt.Errorf("failed to build count query: %w", err)
	}

	// 执行计数请求
	request := esapi.CountRequest{
		Index: []string{table},
		Body:  strings.NewReader(query),
	}

	response, err := request.Do(ctx, e.client)
	if err != nil {
		return 0, fmt.Errorf("count request failed: %w", err)
	}
	defer response.Body.Close()

	// 解析响应
	responseBody, err := parseESResponse(response, false)
	if err != nil {
		return 0, fmt.Errorf("failed to parse count response: %w", err)
	}

	// 使用响应解析器提取计数
	count, err := e.responseParser.ParseCountResponse(responseBody)
	if err != nil {
		return 0, fmt.Errorf("failed to extract count: %w", err)
	}

	return count, nil
}

// buildCountQuery 构建计数查询，复用搜索查询逻辑
func (e *ElasticsearchDB) buildCountQuery(conds Conds) (string, error) {
	builder := es2.NewBuilder(e.client)

	// 添加查询条件
	for _, cond := range conds {
		if cond.Field == "" {
			continue // 跳过空字段
		}
		builder.Where(cond.Field, cond.Cond, cond.Value)
	}

	return builder.BuildQuery(), nil
}

// Upsert 更新插入操作，ES中通常使用index API实现
func (e *ElasticsearchDB) Upsert(ctx context.Context, table string, row UpsertRow, options ...*UpsertOptions) (*UpsertRs, error) {
	// ES的upsert通常通过index操作实现，暂时返回未实现
	return &UpsertRs{}, errors.New("upsert operation not implemented for Elasticsearch")
}

// BatchUpsert 批量更新插入操作
func (e *ElasticsearchDB) BatchUpsert(ctx context.Context, table string, rows []UpsertRow, options ...*BatchWriteOptions) (*BatchUpsertRs, error) {
	// ES的批量upsert通常通过bulk API实现，暂时返回未实现
	return &BatchUpsertRs{}, errors.New("batch upsert operation not implemented for Elasticsearch")
}

// UpsertSingleField 仅为满足接口签名，暂未实现
func (e *ElasticsearchDB) UpsertSingleField(ctx context.Context, table string, row UpsertSingleFields, options ...*UpsertSingleFieldRowOptions) error {
	return errors.New("upsert single field not implemented for Elasticsearch")
}

// ToUpdateOne 将数据转换为UpdateOne结构，使用优化的字段提取
func (e *ElasticsearchDB) ToUpdateOne(data any, options *UpdateOptions) (upData UpdateOne) {
	// 类型安全检查
	row, ok := data.(Row)
	if !ok {
		// 使用错误返回而不是panic，提高代码健壮性
		return UpdateOne{
			ID:      "",
			Updates: []Update{},
		}
	}

	// 获取ID
	upData.ID = row.ID()

	// 使用字段提取器提取更新字段
	updates, err := e.fieldExtractor.ExtractUpdateFields(data, options)
	if err != nil {
		// 记录错误但不中断执行
		upData.Updates = []Update{}
		return
	}

	upData.Updates = updates
	return upData
}

// getOrCreateTypeCache 获取或创建类型缓存，使用优化的缓存策略
func getOrCreateTypeCache(typ reflect.Type) *TypeCache {
	if cache, ok := fieldCache.Load(typ); ok {
		return cache.(*TypeCache)
	}

	// 创建新的缓存，使用优化的字段分析
	fields := analyzeTypeFields(typ, []int{}, 0, maxRecursionDepth)

	// 构建字段映射以提高查找性能
	fieldMap := make(map[string]*FieldInfo, len(fields))
	hasStruct := false

	for i := range fields {
		fieldMap[fields[i].EsTag] = &fields[i]
		if fields[i].IsStruct {
			hasStruct = true
		}
	}

	cache := &TypeCache{
		Fields:    fields,
		fieldMap:  fieldMap,
		hasStruct: hasStruct,
	}

	fieldCache.Store(typ, cache)
	return cache
}

// analyzeTypeFields 分析类型字段，使用优化的递归分析
func analyzeTypeFields(typ reflect.Type, path []int, depth, maxDepth int) []FieldInfo {
	if depth > maxDepth {
		return nil
	}

	// 处理指针类型
	for typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	if typ.Kind() != reflect.Struct {
		return nil
	}

	numField := typ.NumField()
	if numField == 0 {
		return nil
	}

	// 预分配字段切片，优化内存分配
	fields := make([]FieldInfo, 0, numField)

	for i := 0; i < numField; i++ {
		field := typ.Field(i)

		// 跳过未导出字段
		if !field.IsExported() {
			continue
		}

		fieldPath := make([]int, len(path)+1)
		copy(fieldPath, path)
		fieldPath[len(path)] = i

		// 检查es标签
		esTag := field.Tag.Get(esTagKey)
		if esTag != "" && esTag != "-" {
			fieldType := field.Type
			isStruct := isStructType(fieldType)

			fieldInfo := FieldInfo{
				EsTag:    esTag,
				Path:     fieldPath,
				Type:     fieldType,
				IsStruct: isStruct,
			}
			fields = append(fields, fieldInfo)
		} else if field.Anonymous {
			// 处理匿名嵌入字段
			nestedFields := analyzeTypeFields(field.Type, fieldPath, depth+1, maxDepth)
			fields = append(fields, nestedFields...)
		}
	}

	return fields
}

// isStructType 判断类型是否为结构体类型
func isStructType(typ reflect.Type) bool {
	kind := typ.Kind()
	if kind == reflect.Struct {
		return true
	}
	if kind == reflect.Ptr && typ.Elem().Kind() == reflect.Struct {
		return true
	}
	return false
}

// extractFieldsWithCache 基于缓存的字段提取，使用优化的错误处理
func extractFieldsWithCache(val reflect.Value, fields []FieldInfo, result map[string]interface{}) error {
	// 安全地解引用指针
	for val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return nil
		}
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return errors.New("value is not a struct")
	}

	for _, fieldInfo := range fields {
		// 安全地获取字段值
		fieldVal, err := getFieldByPath(val, fieldInfo.Path)
		if err != nil {
			continue // 跳过无法访问的字段
		}

		if !fieldVal.IsValid() || !fieldVal.CanInterface() {
			continue
		}

		// 优化的零值检查
		if isZeroValueReflect(fieldVal) && !shouldIncludeZeroValue(fieldInfo.Type) {
			continue
		}

		if fieldInfo.IsStruct {
			// 结构体类型，递归提取
			nestedResult := make(map[string]interface{})
			nestedCache := getOrCreateTypeCache(fieldInfo.Type)
			if err := extractFieldsWithCache(fieldVal, nestedCache.Fields, nestedResult); err == nil && len(nestedResult) > 0 {
				result[fieldInfo.EsTag] = nestedResult
			}
		} else {
			// 普通字段，直接提取
			result[fieldInfo.EsTag] = fieldVal.Interface()
		}
	}

	return nil
}

// getFieldByPath 通过路径安全地获取字段值
func getFieldByPath(val reflect.Value, path []int) (reflect.Value, error) {
	current := val

	for i, index := range path {
		// 安全地处理指针类型
		for current.Kind() == reflect.Ptr {
			if current.IsNil() {
				return reflect.Value{}, fmt.Errorf("nil pointer at path index %d", i)
			}
			current = current.Elem()
		}

		// 检查是否为结构体
		if current.Kind() != reflect.Struct {
			return reflect.Value{}, fmt.Errorf("not a struct at path index %d", i)
		}

		// 检查字段索引范围
		if index < 0 || index >= current.NumField() {
			return reflect.Value{}, fmt.Errorf("field index %d out of range at path index %d", index, i)
		}

		current = current.Field(index)
	}

	return current, nil
}

// extractEsFieldsGeneric 通用的字段提取函数，使用优化的错误处理
func extractEsFieldsGeneric(val reflect.Value, typ reflect.Type, options *UpdateOptions, handler func(fieldName string, value interface{})) error {
	return extractEsFieldsGenericWithDepth(val, typ, options, handler, 0, maxRecursionDepth)
}

// extractEsFieldsGenericWithDepth 带深度限制的字段提取函数，使用优化的递归逻辑
func extractEsFieldsGenericWithDepth(val reflect.Value, typ reflect.Type, options *UpdateOptions, handler func(fieldName string, value interface{}), depth, maxDepth int) error {
	if depth > maxDepth {
		return errors.New("maximum recursion depth exceeded")
	}

	// 安全地处理指针类型
	for val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return nil
		}
		val = val.Elem()
		typ = typ.Elem()
	}

	if val.Kind() != reflect.Struct {
		return nil
	}

	numField := typ.NumField()
	for i := 0; i < numField; i++ {
		field := typ.Field(i)
		fieldValue := val.Field(i)

		// 跳过未导出的字段
		if !field.IsExported() || !fieldValue.CanInterface() {
			continue
		}

		// 检查es标签
		esTag := field.Tag.Get(esTagKey)
		if esTag != "" && esTag != "-" {
			value := fieldValue.Interface()

			// 优化的零值检查逻辑
			shouldInclude := shouldIncludeField(fieldValue, field.Type, esTag, value, options)

			if shouldInclude {
				if isStructType(field.Type) {
					// 结构体类型，递归提取
					nestedResult := make(map[string]interface{})
					nestedHandler := func(nestedFieldName string, nestedValue interface{}) {
						nestedResult[nestedFieldName] = nestedValue
					}

					if err := extractEsFieldsGenericWithDepth(fieldValue, field.Type, options, nestedHandler, depth+1, maxDepth); err != nil {
						continue // 忽略嵌套错误，继续处理其他字段
					}

					if len(nestedResult) > 0 {
						handler(esTag, nestedResult)
					}
				} else {
					// 普通字段
					handler(esTag, value)
				}
			}
		} else if field.Anonymous {
			// 处理匿名嵌入字段
			if err := extractEsFieldsGenericWithDepth(fieldValue, field.Type, options, handler, depth+1, maxDepth); err != nil {
				continue // 忽略嵌套错误
			}
		}
	}

	return nil
}

// shouldIncludeField 判断是否应该包含字段，使用优化的逻辑
func shouldIncludeField(fieldValue reflect.Value, fieldType reflect.Type, esTag string, value interface{}, options *UpdateOptions) bool {
	isZero := isZeroValueReflect(fieldValue)

	// 如果不是零值，直接包含
	if !isZero {
		return true
	}

	// 使用options的EmptyIgnore来判断零值是否包含
	if options != nil && options.EmptyIgnore != nil {
		return !options.EmptyIgnore(esTag, value)
	}

	// 回退到默认逻辑：某些类型的零值也应该包含
	return shouldIncludeZeroValue(fieldType)
}

// shouldIncludeZeroValue 判断是否应该包含零值，使用优化的类型判断
func shouldIncludeZeroValue(t reflect.Type) bool {
	kind := t.Kind()
	switch kind {
	case reflect.Bool:
		return true // bool的false值是有意义的
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// 可以根据需要开启，有时候0也是有意义的值
		return false
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return false
	case reflect.Float32, reflect.Float64:
		return false
	default:
		return false
	}
}

// isZeroValueReflect 检查值是否为零值，使用优化的类型检查
func isZeroValueReflect(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}

	switch v.Kind() {
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Complex64, reflect.Complex128:
		return v.Complex() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	case reflect.Array:
		// 数组长度为0或所有元素都是零值
		if v.Len() == 0 {
			return true
		}
		for i := 0; i < v.Len(); i++ {
			if !isZeroValueReflect(v.Index(i)) {
				return false
			}
		}
		return true
	case reflect.Slice, reflect.Map, reflect.String:
		return v.Len() == 0
	case reflect.Chan, reflect.Func:
		return v.IsNil()
	case reflect.Struct:
		// 优化：对于常见的时间类型等，使用更高效的检查
		if v.Type() == reflect.TypeOf((*interface{})(nil)).Elem() {
			return v.IsNil()
		}

		// 对于结构体，检查所有可导出字段
		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			if field.CanInterface() && !isZeroValueReflect(field) {
				return false
			}
		}
		return true
	default:
		// 使用reflect.Value的零值接口
		return v.IsZero()
	}
}

// parseESResponse 解析ES响应，使用优化的错误处理和字符串操作
func parseESResponse(response *esapi.Response, allow404 bool) (string, error) {
	if response == nil {
		return "", ErrInvalidResponse
	}

	// 检查响应状态
	if response.IsError() {
		if allow404 && response.StatusCode == 404 {
			return "", ErrDocNotFound
		}
		return "", fmt.Errorf("elasticsearch error [%d]: %s", response.StatusCode, response.String())
	}

	// 获取响应体
	responseBody := response.String()
	if responseBody == "" {
		return "", errors.New("empty response body")
	}

	// 优化的前缀检查和清理
	responseBody = strings.TrimPrefix(responseBody, responsePrefix)

	return responseBody, nil
}
