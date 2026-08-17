package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

const (
	milvusFieldChunkID     = "chunk_id"
	milvusFieldDocID       = "doc_id"
	milvusFieldContent     = "content"
	milvusFieldSource      = "source"
	milvusFieldCategory    = "category"
	milvusFieldSubCategory = "sub_category"
	milvusFieldTitle       = "title"
	milvusFieldMetadata    = "metadata_json"
	milvusFieldEmbedding   = "embedding"

	milvusMaxContentLen  = 8192
	milvusMaxMetadataLen = 4096
)

type MilvusVectorStoreConfig struct {
	Address    string
	Username   string
	Password   string
	DBName     string
	Collection string
	Dimension  int
	MetricType string
	UseTLS     bool
}

// MilvusVectorStore 是审阅知识库的 Milvus 向量检索实现。
//
// 支持两种检索路径：
//   - Dense Vector: FloatVector + AUTOINDEX (COSINE/L2/IP) — 语义检索
//   - BM25 Text Match: 基于 Milvus content 列的 LIKE 表达式全文检索
//
// 升级到 Milvus 2.5+ SDK 后可启用原生 BM25 Function (content → SparseVector)
type MilvusVectorStore struct {
	client      client.Client
	collection  string
	dimension   int
	metricType  entity.MetricType
	bm25Enabled bool // 全文字段检索是否可用
}

func NewMilvusVectorStore(ctx context.Context, cfg MilvusVectorStoreConfig) (*MilvusVectorStore, error) {
	if strings.TrimSpace(cfg.Address) == "" {
		return nil, fmt.Errorf("Milvus address 不能为空")
	}
	if strings.TrimSpace(cfg.Collection) == "" {
		cfg.Collection = "contract_review_knowledge"
	}
	if cfg.Dimension <= 0 {
		return nil, fmt.Errorf("Milvus dimension 必须大于 0")
	}

	c, err := client.NewClient(ctx, client.Config{
		Address:       cfg.Address,
		Username:      cfg.Username,
		Password:      cfg.Password,
		DBName:        cfg.DBName,
		EnableTLSAuth: cfg.UseTLS,
	})
	if err != nil {
		return nil, err
	}

	store := &MilvusVectorStore{
		client:      c,
		collection:  cfg.Collection,
		dimension:   cfg.Dimension,
		metricType:  parseMilvusMetricType(cfg.MetricType),
		bm25Enabled: true, // 基于 LIKE 表达式的文本检索始终可用
	}
	if err := store.ensureCollection(ctx); err != nil {
		c.Close()
		return nil, err
	}
	return store, nil
}

func (s *MilvusVectorStore) Close() {
	if s != nil && s.client != nil {
		s.client.Close()
	}
}

func (s *MilvusVectorStore) BM25Enabled() bool {
	return s.bm25Enabled
}

func (s *MilvusVectorStore) Insert(chunks []Chunk) error {
	if len(chunks) == 0 {
		return nil
	}

	ids := make([]string, 0, len(chunks))
	docIDs := make([]string, 0, len(chunks))
	contents := make([]string, 0, len(chunks))
	sources := make([]string, 0, len(chunks))
	categories := make([]string, 0, len(chunks))
	subCategories := make([]string, 0, len(chunks))
	titles := make([]string, 0, len(chunks))
	metadataValues := make([]string, 0, len(chunks))
	vectors := make([][]float32, 0, len(chunks))

	for _, chunk := range chunks {
		if len(chunk.Embedding) != s.dimension {
			continue
		}
		id := strings.TrimSpace(chunk.ID)
		if id == "" {
			continue
		}
		ids = append(ids, limitRunes(id, 512))
		docIDs = append(docIDs, limitRunes(chunk.DocID, 512))
		contents = append(contents, limitRunes(chunk.Content, milvusMaxContentLen))
		sources = append(sources, limitRunes(chunk.Metadata["source"], 512))
		categories = append(categories, limitRunes(chunk.Metadata["category"], 128))
		subCategories = append(subCategories, limitRunes(chunk.Metadata["sub_category"], 128))
		titles = append(titles, limitRunes(chunk.Metadata["title"], 512))
		metadataJSON, _ := json.Marshal(chunk.Metadata)
		metadataValues = append(metadataValues, limitRunes(string(metadataJSON), milvusMaxMetadataLen))
		vectors = append(vectors, chunk.Embedding)
	}
	if len(ids) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	_, err := s.client.Upsert(
		ctx,
		s.collection,
		"",
		entity.NewColumnVarChar(milvusFieldChunkID, ids),
		entity.NewColumnVarChar(milvusFieldDocID, docIDs),
		entity.NewColumnVarChar(milvusFieldContent, contents),
		entity.NewColumnVarChar(milvusFieldSource, sources),
		entity.NewColumnVarChar(milvusFieldCategory, categories),
		entity.NewColumnVarChar(milvusFieldSubCategory, subCategories),
		entity.NewColumnVarChar(milvusFieldTitle, titles),
		entity.NewColumnVarChar(milvusFieldMetadata, metadataValues),
		entity.NewColumnFloatVector(milvusFieldEmbedding, s.dimension, vectors),
	)
	if err != nil {
		return err
	}
	return s.client.Flush(ctx, s.collection, false)
}

func (s *MilvusVectorStore) Search(query []float32, topK int, filters map[string]string) ([]SearchResult, error) {
	if len(query) != s.dimension {
		return nil, fmt.Errorf("向量维度不匹配: got %d want %d", len(query), s.dimension)
	}
	if topK <= 0 {
		topK = 5
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sp, err := entity.NewIndexAUTOINDEXSearchParam(10)
	if err != nil {
		return nil, err
	}
	results, err := s.client.Search(
		ctx,
		s.collection,
		nil,
		buildMilvusExpr(filters),
		[]string{
			milvusFieldDocID,
			milvusFieldContent,
			milvusFieldSource,
			milvusFieldCategory,
			milvusFieldSubCategory,
			milvusFieldTitle,
			milvusFieldMetadata,
		},
		[]entity.Vector{entity.FloatVector(query)},
		milvusFieldEmbedding,
		s.metricType,
		topK,
		sp,
	)
	if err != nil {
		return nil, err
	}

	out := make([]SearchResult, 0, results[0].ResultCount)
	for _, result := range results {
		for i := 0; i < result.ResultCount; i++ {
			chunkID := columnValueString(result.IDs, i)
			metadata := metadataFromMilvusResult(result.Fields, i)
			content := columnByNameString(result.Fields, milvusFieldContent, i)
			source := columnByNameString(result.Fields, milvusFieldSource, i)
			if source == "" {
				source = metadata["source"]
			}
			score := 0.0
			if i < len(result.Scores) {
				score = float64(result.Scores[i])
			}
			out = append(out, SearchResult{
				ChunkID:       chunkID,
				DocID:         columnByNameString(result.Fields, milvusFieldDocID, i),
				Content:       content,
				Score:         score,
				BaseScore:     score,
				Source:        source,
				Metadata:      metadata,
				ParentChunkID: metadata["parent_chunk_id"],
			})
		}
	}
	return out, nil
}

// SearchBM25 基于 Milvus LIKE 表达式的全文检索
//
// 策略: 将查询分词后，用 OR 连接多个 LIKE 条件对 content 列进行文本匹配
// 升级到 Milvus 2.5+ SDK 后可切换为原生 BM25 Function + SparseVector 搜索
func (s *MilvusVectorStore) SearchBM25(query string, topK int, filters map[string]string) ([]SearchResult, error) {
	if !s.bm25Enabled {
		return nil, nil
	}
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	if topK <= 0 {
		topK = 5
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 构建全文检索表达式
	expr := buildBM25Expr(query)
	if filterExpr := buildMilvusExpr(filters); filterExpr != "" {
		if expr != "" {
			expr = "(" + expr + ") && (" + filterExpr + ")"
		} else {
			expr = filterExpr
		}
	}

	if expr == "" {
		return nil, nil
	}

	results, err := s.client.Query(
		ctx,
		s.collection,
		nil,
		expr,
		[]string{
			milvusFieldChunkID,
			milvusFieldDocID,
			milvusFieldContent,
			milvusFieldSource,
			milvusFieldCategory,
			milvusFieldSubCategory,
			milvusFieldTitle,
			milvusFieldMetadata,
		},
		client.WithLimit(int64(topK)),
	)
	if err != nil {
		return nil, fmt.Errorf("BM25 全文检索失败: %w", err)
	}

	count := results.Len()
	if count > topK {
		count = topK
	}
	out := make([]SearchResult, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, SearchResult{
			ChunkID:   columnByNameString(results, milvusFieldChunkID, i),
			DocID:     columnByNameString(results, milvusFieldDocID, i),
			Content:   columnByNameString(results, milvusFieldContent, i),
			Source:    columnByNameString(results, milvusFieldSource, i),
			Score:     0.5, // 文本匹配固定中间分，由 RRF 融合最终排序
			BaseScore: 0.5,
			Metadata:  metadataFromMilvusResult(results, i),
		})
	}
	return out, nil
}

// buildBM25Expr 构建全文检索 LIKE 表达式
// 对查询进行双字 bigram 分词 + 法律关键词提取，OR 连接
func buildBM25Expr(query string) string {
	terms := tokenizeForBM25(query)
	if len(terms) == 0 {
		return ""
	}

	exprParts := make([]string, 0, len(terms))
	for _, term := range terms {
		term = strings.ReplaceAll(term, `"`, `\"`)
		exprParts = append(exprParts, fmt.Sprintf(`%s like "%%%s%%"`, milvusFieldContent, term))
	}

	return strings.Join(exprParts, " || ")
}

// tokenizeForBM25 为 BM25 检索对查询分词
func tokenizeForBM25(query string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}

	terms := make([]string, 0, 16)
	seen := make(map[string]bool)

	// 1. CJK bigram
	runes := []rune(query)
	for i := 0; i < len(runes)-1; i++ {
		if isCJK(runes[i]) && isCJK(runes[i+1]) {
			bigram := string(runes[i : i+2])
			if !seen[bigram] {
				seen[bigram] = true
				terms = append(terms, bigram)
			}
		}
	}

	// 2. 法律关键词
	legalKeywords := []string{
		"付款", "支付", "验收", "交付", "违约", "赔偿", "违约金", "解除", "终止",
		"知识产权", "著作权", "专利", "商标", "保密", "管辖", "争议", "发票", "逾期",
		"单方", "服务范围", "服务内容", "质量", "期限", "义务", "责任", "免责",
		"合同主体", "甲乙方", "不可抗力", "变更", "转让", "仲裁", "诉讼",
	}
	queryLower := strings.ToLower(query)
	for _, kw := range legalKeywords {
		if strings.Contains(queryLower, strings.ToLower(kw)) && !seen[kw] {
			seen[kw] = true
			terms = append(terms, kw)
		}
	}

	// 3. 英文单词
	for _, word := range strings.Fields(queryLower) {
		if len(word) >= 2 && !isAllCJK(word) && !seen[word] {
			seen[word] = true
			terms = append(terms, word)
		}
	}

	// 限制数量
	if len(terms) > 15 {
		terms = terms[:15]
	}
	return terms
}

func isAllCJK(s string) bool {
	for _, r := range s {
		if !isCJK(r) {
			return false
		}
	}
	return len(s) > 0
}

func (s *MilvusVectorStore) Delete(ids []string) error {
	cleanIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			cleanIDs = append(cleanIDs, trimmed)
		}
	}
	if len(cleanIDs) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.client.DeleteByPks(ctx, s.collection, "", entity.NewColumnVarChar(milvusFieldChunkID, cleanIDs))
}

func (s *MilvusVectorStore) ensureCollection(ctx context.Context) error {
	has, err := s.client.HasCollection(ctx, s.collection)
	if err != nil {
		return err
	}
	if !has {
		schema := entity.NewSchema().
			WithName(s.collection).
			WithDescription("contract review knowledge chunks").
			WithAutoID(false).
			WithField(entity.NewField().WithName(milvusFieldChunkID).
				WithDataType(entity.FieldTypeVarChar).
				WithIsPrimaryKey(true).
				WithMaxLength(512)).
			WithField(entity.NewField().WithName(milvusFieldDocID).
				WithDataType(entity.FieldTypeVarChar).
				WithMaxLength(512)).
			WithField(entity.NewField().WithName(milvusFieldContent).
				WithDataType(entity.FieldTypeVarChar).
				WithMaxLength(milvusMaxContentLen)).
			WithField(entity.NewField().WithName(milvusFieldSource).
				WithDataType(entity.FieldTypeVarChar).
				WithMaxLength(512)).
			WithField(entity.NewField().WithName(milvusFieldCategory).
				WithDataType(entity.FieldTypeVarChar).
				WithMaxLength(128)).
			WithField(entity.NewField().WithName(milvusFieldSubCategory).
				WithDataType(entity.FieldTypeVarChar).
				WithMaxLength(128)).
			WithField(entity.NewField().WithName(milvusFieldTitle).
				WithDataType(entity.FieldTypeVarChar).
				WithMaxLength(512)).
			WithField(entity.NewField().WithName(milvusFieldMetadata).
				WithDataType(entity.FieldTypeVarChar).
				WithMaxLength(milvusMaxMetadataLen)).
			WithField(entity.NewField().WithName(milvusFieldEmbedding).
				WithDataType(entity.FieldTypeFloatVector).
				WithDim(int64(s.dimension)))
		if err := s.client.CreateCollection(ctx, schema, entity.DefaultShardNumber); err != nil {
			return err
		}
	}

	idx, err := entity.NewIndexAUTOINDEX(s.metricType)
	if err != nil {
		return err
	}
	if err := s.client.CreateIndex(ctx, s.collection, milvusFieldEmbedding, idx, false); err != nil {
		lowerErr := strings.ToLower(err.Error())
		if !strings.Contains(lowerErr, "already") && !strings.Contains(lowerErr, "exist") {
			return err
		}
	}
	return s.client.LoadCollection(ctx, s.collection, false)
}

func parseMilvusMetricType(metricType string) entity.MetricType {
	switch strings.ToUpper(strings.TrimSpace(metricType)) {
	case "L2":
		return entity.L2
	case "IP":
		return entity.IP
	default:
		return entity.COSINE
	}
}

func buildMilvusExpr(filters map[string]string) string {
	var exprs []string
	for key, value := range filters {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		switch key {
		case "category":
			exprs = append(exprs, fmt.Sprintf(`%s == "%s"`, milvusFieldCategory, escapeMilvusString(value)))
		case "sub_category":
			exprs = append(exprs, fmt.Sprintf(`%s == "%s"`, milvusFieldSubCategory, escapeMilvusString(value)))
		case "source":
			exprs = append(exprs, fmt.Sprintf(`%s == "%s"`, milvusFieldSource, escapeMilvusString(value)))
		}
	}
	return strings.Join(exprs, " && ")
}

func metadataFromMilvusResult(fields client.ResultSet, idx int) map[string]string {
	metadata := map[string]string{
		"source":       columnByNameString(fields, milvusFieldSource, idx),
		"category":     columnByNameString(fields, milvusFieldCategory, idx),
		"sub_category": columnByNameString(fields, milvusFieldSubCategory, idx),
		"title":        columnByNameString(fields, milvusFieldTitle, idx),
	}
	rawMetadata := columnByNameString(fields, milvusFieldMetadata, idx)
	if rawMetadata != "" {
		var parsed map[string]string
		if err := json.Unmarshal([]byte(rawMetadata), &parsed); err == nil {
			for k, v := range parsed {
				metadata[k] = v
			}
		}
	}
	return metadata
}

func columnByNameString(fields client.ResultSet, name string, idx int) string {
	return columnValueString(fields.GetColumn(name), idx)
}

func columnValueString(column entity.Column, idx int) string {
	if column == nil {
		return ""
	}
	value, err := column.Get(idx)
	if err != nil || value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func escapeMilvusString(value string) string {
	return strings.ReplaceAll(value, `"`, `\"`)
}

func limitRunes(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || utf8.RuneCountInString(value) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max])
}
