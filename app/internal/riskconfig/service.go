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
	riskPoint := &RiskPoint{
		ContractTypeID:   contractType.ID,
		ContractTypeName: contractType.Name,
		RiskContent:      strings.TrimSpace(req.RiskContent),
		RiskType:         limitString(defaultString(req.RiskType, "通用风险"), 64),
		RiskLevel:        normalizeRiskLevel(req.RiskLevel),
		ApplicableScope:  normalizeScope(req.ApplicableScope),
		Departments:      string(departments),
		Creator:          limitString(creator, 64),
		Status:           status,
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
	existing.ContractTypeID = contractType.ID
	existing.ContractTypeName = contractType.Name
	existing.RiskContent = strings.TrimSpace(req.RiskContent)
	existing.RiskType = limitString(defaultString(req.RiskType, "通用风险"), 64)
	existing.RiskLevel = normalizeRiskLevel(req.RiskLevel)
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
			"contract_type_id":   existing.ContractTypeID,
			"contract_type_name": existing.ContractTypeName,
			"risk_content":       existing.RiskContent,
			"risk_type":          existing.RiskType,
			"risk_level":         existing.RiskLevel,
			"applicable_scope":   existing.ApplicableScope,
			"departments":        existing.Departments,
			"status":             existing.Status,
			"knowledge_doc_id":   existing.KnowledgeDocID,
			"updated_at":         existing.UpdatedAt,
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
	}
	if err := tx.WithContext(ctx).Create(&chunk).Error; err != nil {
		return 0, err
	}
	return doc.ID, nil
}

func deleteKnowledgeDoc(ctx context.Context, tx *gorm.DB, docID uint64) error {
	if err := tx.WithContext(ctx).Where("doc_id = ?", docID).
		Delete(&knowledge.ReviewKnowledgeChunk{}).Error; err != nil {
		return err
	}
	return tx.WithContext(ctx).Delete(&knowledge.ReviewKnowledgeDoc{}, docID).Error
}

func buildKnowledgeContent(riskPoint *RiskPoint) string {
	return fmt.Sprintf(`【风险点配置】
合同类型：%s
风险类型：%s
风险等级：%s
适用范围：%s
风险点ID：%s

风险内容：
%s

审阅使用要求：
审阅%s时，应检索并比对本风险点。若合同条款出现相同或相近风险，请结合上下文输出风险分析、风险等级、法律或审阅依据，以及可执行的修订建议。`,
		riskPoint.ContractTypeName,
		riskPoint.RiskType,
		riskPoint.RiskLevel,
		riskPoint.ApplicableScope,
		riskCode(riskPoint.ID),
		riskPoint.RiskContent,
		riskPoint.ContractTypeName,
	)
}

func ToResponse(riskPoint *RiskPoint) RiskPointResponse {
	departments := make([]string, 0)
	if strings.TrimSpace(riskPoint.Departments) != "" {
		_ = json.Unmarshal([]byte(riskPoint.Departments), &departments)
	}
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
