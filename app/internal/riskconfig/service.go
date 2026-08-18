package riskconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"contract_review/app/internal/contract"
	"contract_review/app/internal/knowledge"

	"gorm.io/gorm"
)

type Service struct {
	repo *Repo
	db   *gorm.DB
}

func NewService(repo *Repo, db *gorm.DB) *Service {
	return &Service{repo: repo, db: db}
}

func (s *Service) Create(ctx context.Context, creator string, req RiskPointRequest) (*RiskPoint, error) {
	contractType, err := s.resolveContractType(ctx, req.ContractTypeID, req.ContractTypeName)
	if err != nil {
		return nil, err
	}

	status := normalizeStatus(req.Status, req.IsEnabled)
	departments, _ := json.Marshal(req.Departments)
	keywords, _ := json.Marshal(cleanStringList(req.Keywords, 16, 32))
	applicableClauses, _ := json.Marshal(cleanStringList(req.ApplicableClauses, 16, 64))
	riskPoint := &RiskPoint{
		ContractTypeID:      contractType.ID,
		ContractTypeName:    contractType.Name,
		RiskContent:         strings.TrimSpace(req.RiskContent),
		RiskType:            limitString(defaultString(req.RiskType, "通用风险"), 64),
		RiskLevel:           normalizeRiskLevel(req.RiskLevel),
		TriggerCondition:    strings.TrimSpace(req.TriggerCondition),
		Keywords:            string(keywords),
		ApplicableClauses:   string(applicableClauses),
		LegalBasis:          strings.TrimSpace(req.LegalBasis),
		RecommendedTemplate: strings.TrimSpace(req.RecommendedTemplate),
		ApplicableScope:     normalizeScope(req.ApplicableScope),
		Departments:         string(departments),
		Creator:             limitString(creator, 64),
		Status:              status,
	}
	if riskPoint.RiskContent == "" {
		return nil, errors.New("风险点内容不能为空")
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(riskPoint).Error; err != nil {
			return err
		}
		docID, err := syncKnowledgeDoc(ctx, tx, riskPoint)
		if err != nil {
			return err
		}
		riskPoint.KnowledgeDocID = docID
		return tx.Model(&RiskPoint{}).Where("id = ?", riskPoint.ID).
			Update("knowledge_doc_id", docID).Error
	})
	if err != nil {
		return nil, err
	}
	return riskPoint, nil
}

func (s *Service) Update(ctx context.Context, id uint64, req RiskPointRequest) (*RiskPoint, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	contractType, err := s.resolveContractType(ctx, req.ContractTypeID, req.ContractTypeName)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.RiskContent) == "" {
		return nil, errors.New("风险点内容不能为空")
	}

	departments, _ := json.Marshal(req.Departments)
	keywords, _ := json.Marshal(cleanStringList(req.Keywords, 16, 32))
	applicableClauses, _ := json.Marshal(cleanStringList(req.ApplicableClauses, 16, 64))
	existing.ContractTypeID = contractType.ID
	existing.ContractTypeName = contractType.Name
	existing.RiskContent = strings.TrimSpace(req.RiskContent)
	existing.RiskType = limitString(defaultString(req.RiskType, "通用风险"), 64)
	existing.RiskLevel = normalizeRiskLevel(req.RiskLevel)
	existing.TriggerCondition = strings.TrimSpace(req.TriggerCondition)
	existing.Keywords = string(keywords)
	existing.ApplicableClauses = string(applicableClauses)
	existing.LegalBasis = strings.TrimSpace(req.LegalBasis)
	existing.RecommendedTemplate = strings.TrimSpace(req.RecommendedTemplate)
	existing.ApplicableScope = normalizeScope(req.ApplicableScope)
	existing.Departments = string(departments)
	existing.Status = normalizeStatus(req.Status, req.IsEnabled)
	existing.UpdatedAt = time.Now()

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		docID, err := syncKnowledgeDoc(ctx, tx, existing)
		if err != nil {
			return err
		}
		existing.KnowledgeDocID = docID
		return tx.Model(&RiskPoint{}).Where("id = ?", existing.ID).Updates(map[string]interface{}{
			"contract_type_id":     existing.ContractTypeID,
			"contract_type_name":   existing.ContractTypeName,
			"risk_content":         existing.RiskContent,
			"risk_type":            existing.RiskType,
			"risk_level":           existing.RiskLevel,
			"trigger_condition":    existing.TriggerCondition,
			"keywords":             existing.Keywords,
			"applicable_clauses":   existing.ApplicableClauses,
			"legal_basis":          existing.LegalBasis,
			"recommended_template": existing.RecommendedTemplate,
			"applicable_scope":     existing.ApplicableScope,
			"departments":          existing.Departments,
			"status":               existing.Status,
			"knowledge_doc_id":     existing.KnowledgeDocID,
			"updated_at":           existing.UpdatedAt,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return existing, nil
}

func (s *Service) Get(ctx context.Context, id uint64) (*RiskPoint, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) List(ctx context.Context, filters ListFilters, page, pageSize int) ([]RiskPoint, int64, error) {
	offset := (page - 1) * pageSize
	return s.repo.List(ctx, filters, offset, pageSize)
}

func (s *Service) Delete(ctx context.Context, id uint64) error {
	riskPoint, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if riskPoint.KnowledgeDocID > 0 {
			if err := deleteKnowledgeDoc(ctx, tx, riskPoint.KnowledgeDocID); err != nil {
				return err
			}
		}
		return tx.Delete(&RiskPoint{}, id).Error
	})
}

func (s *Service) BatchDelete(ctx context.Context, ids []uint64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var riskPoints []RiskPoint
		if err := tx.Where("id IN ?", ids).Find(&riskPoints).Error; err != nil {
			return err
		}
		for _, riskPoint := range riskPoints {
			if riskPoint.KnowledgeDocID > 0 {
				if err := deleteKnowledgeDoc(ctx, tx, riskPoint.KnowledgeDocID); err != nil {
					return err
				}
			}
		}
		return tx.Where("id IN ?", ids).Delete(&RiskPoint{}).Error
	})
}

func (s *Service) Stats(ctx context.Context, contractTypeName string) (RiskPointStatsResponse, error) {
	return s.repo.Stats(ctx, contractTypeName)
}

// BackfillRiskPointMetadata 为存量风险点分块补齐结构化 metadata（P0 收尾迁移）。
// 幂等：仅更新 metadata 为 NULL 的分块（旧数据），已补齐的不再重复写。
// 返回实际更新的分块数量。
func (s *Service) BackfillRiskPointMetadata(ctx context.Context) (int, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("riskconfig service: db is nil")
	}

	var points []RiskPoint
	if err := s.db.WithContext(ctx).Model(&RiskPoint{}).Find(&points).Error; err != nil {
		return 0, err
	}

	updated := 0
	for _, rp := range points {
		if rp.KnowledgeDocID == 0 {
			continue
		}
		meta := buildRiskPointMetadataJSON(&rp)
		res := s.db.WithContext(ctx).
			Model(&knowledge.ReviewKnowledgeChunk{}).
			Where("doc_id = ? AND metadata IS NULL", rp.KnowledgeDocID).
			Update("metadata", meta)
		if res.Error != nil {
			return updated, res.Error
		}
		updated += int(res.RowsAffected)
	}
	return updated, nil
}

func (s *Service) resolveContractType(ctx context.Context, id uint64, name string) (*contract.ContractType, error) {
	var contractType contract.ContractType
	query := s.db.WithContext(ctx)
	if id > 0 {
		if err := query.First(&contractType, id).Error; err != nil {
			return nil, errors.New("合同类型不存在")
		}
		return &contractType, nil
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("请选择合同类型")
	}
	if err := query.Where("name = ?", name).First(&contractType).Error; err != nil {
		return nil, errors.New("合同类型不存在")
	}
	return &contractType, nil
}

func syncKnowledgeDoc(ctx context.Context, tx *gorm.DB, riskPoint *RiskPoint) (uint64, error) {
	content := buildKnowledgeContent(riskPoint)
	status := "indexed"
	if riskPoint.Status != "enabled" {
		status = "pending"
	}

	doc := knowledge.ReviewKnowledgeDoc{
		Title:       fmt.Sprintf("风险点配置-%s-%s", riskPoint.ContractTypeName, riskCode(riskPoint.ID)),
		Category:    "风险点",
		SubCategory: riskPoint.ContractTypeName,
		Source:      "风险点配置",
		Content:     content,
		ChunkCount:  1,
		Status:      status,
	}

	if riskPoint.KnowledgeDocID > 0 {
		if err := tx.WithContext(ctx).Model(&knowledge.ReviewKnowledgeDoc{}).
			Where("id = ?", riskPoint.KnowledgeDocID).
			Updates(map[string]interface{}{
				"title":        doc.Title,
				"category":     doc.Category,
				"sub_category": doc.SubCategory,
				"source":       doc.Source,
				"content":      doc.Content,
				"chunk_count":  doc.ChunkCount,
				"status":       doc.Status,
				"updated_at":   time.Now(),
			}).Error; err != nil {
			return 0, err
		}
		if err := tx.WithContext(ctx).Where("doc_id = ?", riskPoint.KnowledgeDocID).
			Delete(&knowledge.ReviewKnowledgeChunk{}).Error; err != nil {
			return 0, err
		}
		doc.ID = riskPoint.KnowledgeDocID
	} else {
		if err := tx.WithContext(ctx).Create(&doc).Error; err != nil {
			return 0, err
		}
	}

	chunk := knowledge.ReviewKnowledgeChunk{
		DocID:      doc.ID,
		ChunkIndex: 0,
		Content:    content,
		VectorID:   fmt.Sprintf("risk-%d-chunk-0", riskPoint.ID),
		Metadata:   buildRiskPointMetadataJSON(riskPoint),
	}
	if err := tx.WithContext(ctx).Create(&chunk).Error; err != nil {
		return 0, err
	}
	return doc.ID, nil
}

// buildRiskPointMetadata 将风险点的结构化字段构建为分块元数据。
// RAG 命中后直接读取这些字段，避免从 buildKnowledgeContent 拍扁的文本里再正则反解。
// keywords / applicable_clauses 存"、"连接串，与 splitListField 的分隔符保持一致。
func buildRiskPointMetadata(riskPoint *RiskPoint) map[string]string {
	keywords := decodeStringList(riskPoint.Keywords)
	applicableClauses := decodeStringList(riskPoint.ApplicableClauses)
	return map[string]string{
		"risk_id":              riskCode(riskPoint.ID),
		"contract_type":        riskPoint.ContractTypeName,
		"risk_type":            riskPoint.RiskType,
		"risk_level":           riskPoint.RiskLevel,
		"applicable_scope":     riskPoint.ApplicableScope,
		"trigger_condition":    riskPoint.TriggerCondition,
		"keywords":             strings.Join(keywords, "、"),
		"applicable_clauses":   strings.Join(applicableClauses, "、"),
		"legal_basis":          riskPoint.LegalBasis,
		"recommended_template": riskPoint.RecommendedTemplate,
		"risk_content":         riskPoint.RiskContent,
	}
}

// buildRiskPointMetadataJSON 将结构化元数据序列化为 JSON 字符串写入分块 metadata 列。
func buildRiskPointMetadataJSON(riskPoint *RiskPoint) string {
	b, err := json.Marshal(buildRiskPointMetadata(riskPoint))
	if err != nil {
		return "{}"
	}
	return string(b)
}

func deleteKnowledgeDoc(ctx context.Context, tx *gorm.DB, docID uint64) error {
	if err := tx.WithContext(ctx).Where("doc_id = ?", docID).
		Delete(&knowledge.ReviewKnowledgeChunk{}).Error; err != nil {
		return err
	}
	return tx.WithContext(ctx).Delete(&knowledge.ReviewKnowledgeDoc{}, docID).Error
}

func buildKnowledgeContent(riskPoint *RiskPoint) string {
	keywords := decodeStringList(riskPoint.Keywords)
	applicableClauses := decodeStringList(riskPoint.ApplicableClauses)
	return fmt.Sprintf(`【风险点配置】
合同类型：%s
风险类型：%s
风险等级：%s
适用范围：%s
风险点ID：%s
触发条件：%s
关键词：%s
适用条款：%s
法律依据：%s
推荐修改模板：%s

风险内容：
%s

审阅使用要求：
审阅%s时，应检索并比对本风险点。若合同条款出现相同或相近风险，请结合上下文输出风险分析、风险等级、法律或审阅依据，以及可执行的修订建议。`,
		riskPoint.ContractTypeName,
		riskPoint.RiskType,
		riskPoint.RiskLevel,
		riskPoint.ApplicableScope,
		riskCode(riskPoint.ID),
		defaultString(riskPoint.TriggerCondition, "未配置"),
		strings.Join(keywords, "、"),
		strings.Join(applicableClauses, "、"),
		defaultString(riskPoint.LegalBasis, "未配置"),
		defaultString(riskPoint.RecommendedTemplate, "未配置"),
		riskPoint.RiskContent,
		riskPoint.ContractTypeName,
	)
}

func ToResponse(riskPoint *RiskPoint) RiskPointResponse {
	departments := make([]string, 0)
	if strings.TrimSpace(riskPoint.Departments) != "" {
		_ = json.Unmarshal([]byte(riskPoint.Departments), &departments)
	}
	keywords := decodeStringList(riskPoint.Keywords)
	applicableClauses := decodeStringList(riskPoint.ApplicableClauses)
	updateDate := riskPoint.UpdatedAt.Format("2006-01-02 15:04:05")
	statusText := "启用"
	if riskPoint.Status == "disabled" {
		statusText = "停用"
	}
	ragStatus := "未入库"
	if riskPoint.Status == "enabled" && riskPoint.KnowledgeDocID > 0 {
		ragStatus = "已入库"
	} else if riskPoint.Status == "disabled" && riskPoint.KnowledgeDocID > 0 {
		ragStatus = "已停用"
	}
	return RiskPointResponse{
		ID:                     riskPoint.ID,
		Key:                    strconv.FormatUint(riskPoint.ID, 10),
		RiskID:                 riskCode(riskPoint.ID),
		RiskContent:            riskPoint.RiskContent,
		RiskType:               riskPoint.RiskType,
		RiskLevel:              riskPoint.RiskLevel,
		TriggerCondition:       riskPoint.TriggerCondition,
		Keywords:               keywords,
		ApplicableClauses:      applicableClauses,
		LegalBasis:             riskPoint.LegalBasis,
		RecommendedTemplate:    riskPoint.RecommendedTemplate,
		ContractTypeID:         riskPoint.ContractTypeID,
		ContractType:           riskPoint.ContractTypeName,
		ApplicableContractType: riskPoint.ContractTypeName,
		ApplicableScope:        riskPoint.ApplicableScope,
		Department:             departments,
		Creator:                riskPoint.Creator,
		Status:                 statusText,
		IsEnabled:              riskPoint.Status,
		KnowledgeDocID:         riskPoint.KnowledgeDocID,
		RAGStatus:              ragStatus,
		UpdateDate:             updateDate,
		CreatedAt:              riskPoint.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}

func riskCode(id uint64) string {
	return fmt.Sprintf("RP%06d", id)
}

func normalizeStatus(status, isEnabled string) string {
	value := strings.TrimSpace(status)
	if value == "" {
		value = strings.TrimSpace(isEnabled)
	}
	switch value {
	case "停用", "disabled", "disable":
		return "disabled"
	default:
		return "enabled"
	}
}

func normalizeScope(scope string) string {
	switch strings.TrimSpace(scope) {
	case "individual", "department", "platform":
		return scope
	default:
		return "platform"
	}
}

func normalizeRiskLevel(level string) string {
	if strings.Contains(level, "高") {
		return "高"
	}
	if strings.Contains(level, "低") {
		return "低"
	}
	return "中"
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func limitString(value string, max int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max])
}

func cleanStringList(values []string, maxItems, maxLen int) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		value = limitString(value, maxLen)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
		if maxItems > 0 && len(out) >= maxItems {
			break
		}
	}
	return out
}

func decodeStringList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return []string{}
	}
	return cleanStringList(values, 0, 128)
}
