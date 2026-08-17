package qa

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"contract_review/app/internal/contract"
	"contract_review/app/internal/gateway"
	"contract_review/app/internal/global"
	"contract_review/app/internal/rag"
	"contract_review/app/pkg/utils"

	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// QAService 合同问答服务
type QAService struct {
	db              *gorm.DB
	repo            *Repo
	contractService *contract.ContractService

	knowledgeMu     sync.Mutex
	knowledgeIndex  *rag.SimpleKeywordIndex
	knowledgeLoaded bool
}

func NewQAService(db *gorm.DB, contractService *contract.ContractService) *QAService {
	return &QAService{db: db, repo: NewRepo(db), contractService: contractService}
}

// sessionInfo 会话最小信息
type sessionInfo struct {
	ID     uint64
	UserID uint64
	FileID uint64
	Title  string
}

// getSession 校验会话归属并返回会话信息
func (s *QAService) getSession(ctx context.Context, sessionID, userID uint64) (*sessionInfo, error) {
	var sess sessionInfo
	err := s.db.WithContext(ctx).Table("sessions").
		Select("id, user_id, file_id, title").
		Where("id = ?", sessionID).First(&sess).Error
	if err != nil {
		return nil, fmt.Errorf("会话不存在: %w", err)
	}
	if sess.UserID != userID {
		return nil, fmt.Errorf("无权访问该会话")
	}
	if sess.FileID == 0 {
		return nil, fmt.Errorf("该会话未绑定合同，无法问答")
	}
	return &sess, nil
}

// ensureKnowledgeIndex 懒加载知识库关键词索引（法规/风险点/审阅指引）
func (s *QAService) ensureKnowledgeIndex(ctx context.Context) *rag.SimpleKeywordIndex {
	s.knowledgeMu.Lock()
	defer s.knowledgeMu.Unlock()
	if s.knowledgeLoaded {
		return s.knowledgeIndex
	}
	idx := rag.NewSimpleKeywordIndex()
	if global.DB != nil {
		chunks, err := rag.LoadKnowledgeChunksFromDB(ctx, global.DB)
		if err != nil {
			global.Log.Warn("问答加载知识库失败", zap.Error(err))
		} else if len(chunks) > 0 {
			if err := idx.Index(chunks); err != nil {
				global.Log.Warn("问答知识库入索引失败", zap.Error(err))
			} else {
				global.Log.Info("问答知识库已加载", zap.Int("chunks", len(chunks)))
			}
		}
	}
	s.knowledgeIndex = idx
	s.knowledgeLoaded = true
	return idx
}

// Ask 提问并流式返回答案
func (s *QAService) Ask(ctx context.Context, sessionID, userID uint64, account, question string, onDelta func(string)) (*QAMessage, error) {
	start := time.Now()

	// 1. 校验会话
	sess, err := s.getSession(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}

	// 2. 读取合同内容
	contractInfo, err := s.contractService.GetContractByIDWithTypeForAccount(ctx, account, sess.FileID)
	if err != nil || contractInfo == nil {
		return nil, fmt.Errorf("获取合同信息失败: %w", err)
	}
	// 优先使用已持久化的提取文本（上传时已提取），避免每次 QA 请求重复解析文件
	contractText := ""
	if contractInfo.RawText != "" {
		contractText = contractInfo.RawText
	} else if contractInfo.FilePath != "" {
		contractText, err = utils.ExtractText(contract.LocalFilePath(contractInfo.FilePath))
		if err != nil {
			return nil, fmt.Errorf("读取合同内容失败: %w", err)
		}
	}

	contractType := ""
	if contractInfo.ContractType != nil {
		contractType = contractInfo.ContractType.Name
	}

	// 3. 加载多轮历史（最近 10 轮）
	history, err := s.repo.ListBySession(ctx, sessionID, 20)
	if err != nil {
		global.Log.Warn("加载问答历史失败", zap.Error(err))
		history = nil
	}

	// 4. RAG 检索：知识库风险点/法规 + 合同相关条款
	knowledgeCtx := s.retrieveKnowledge(ctx, question, contractType)
	contractCtx := retrieveContractChunks(contractText, question)

	// 5. 构建消息
	systemPrompt := buildQASystemPrompt(contractInfo, contractType, knowledgeCtx, contractCtx)
	messages := buildQAMessages(systemPrompt, history, question)

	// 6. 持久化用户消息（先存，即使模型失败也保留提问）
	userMsg := &QAMessage{
		SessionID: sessionID,
		UserID:    userID,
		Role:      RoleUser,
		Content:   question,
	}
	if err := s.repo.Create(ctx, userMsg); err != nil {
		global.Log.Warn("保存用户提问失败", zap.Error(err))
	}

	// 7. 经网关流式调用
	gwCtx := gateway.WithCaller(ctx, userID, account, gateway.FeatureQA)
	resp, err := global.Gateway.StreamChat(gwCtx, gateway.GatewayRequest{
		Feature:  gateway.FeatureQA,
		UserID:   userID,
		Account:  account,
		Messages: messages,
	}, onDelta)
	if err != nil {
		// 记录失败的占位 assistant 消息，便于前端展示错误
		failMsg := &QAMessage{
			SessionID: sessionID,
			UserID:    userID,
			Role:      RoleAssistant,
			Content:   "（回答生成失败：" + err.Error() + "）",
		}
		_ = s.repo.Create(ctx, failMsg)
		return nil, err
	}

	// 8. 持久化 assistant 消息
	assistantMsg := &QAMessage{
		SessionID: sessionID,
		UserID:    userID,
		Role:      RoleAssistant,
		Content:   resp.Message.Content,
		Tokens:    resp.Usage.TotalTokens,
	}
	if err := s.repo.Create(ctx, assistantMsg); err != nil {
		global.Log.Warn("保存回答失败", zap.Error(err))
	}

	global.Log.Info("合同问答完成",
		zap.Uint64("session", sessionID),
		zap.Int("tokens", resp.Usage.TotalTokens),
		zap.Bool("cache_hit", resp.CacheHit),
		zap.Int64("latency_ms", time.Since(start).Milliseconds()))
	return assistantMsg, nil
}

// retrieveKnowledge 检索知识库相关风险点/法规
func (s *QAService) retrieveKnowledge(ctx context.Context, question, contractType string) string {
	idx := s.ensureKnowledgeIndex(ctx)
	if idx == nil {
		return ""
	}
	filters := map[string]string{}
	results, err := idx.Search(question, 5, filters)
	if err != nil || len(results) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n\n## 相关审阅规范与法规参考\n")
	for i, r := range results {
		source := r.Source
		if source == "" {
			source = r.Metadata["title"]
		}
		sb.WriteString(fmt.Sprintf("【参考%d】(来源: %s)\n%s\n\n", i+1, source, r.Content))
	}
	return sb.String()
}

// retrieveContractChunks 按关键词检索合同相关条款片段
func retrieveContractChunks(contractText, question string) string {
	if strings.TrimSpace(contractText) == "" {
		return ""
	}
	chunks := splitContractText(contractText, 600)
	if len(chunks) == 0 {
		return ""
	}
	ragChunks := make([]rag.Chunk, 0, len(chunks))
	for i, c := range chunks {
		ragChunks = append(ragChunks, rag.Chunk{
			ID:       fmt.Sprintf("contract-%d", i),
			DocID:    "contract",
			Content:  c,
			Metadata: map[string]string{"chunk_index": fmt.Sprintf("%d", i)},
		})
	}
	idx := rag.NewSimpleKeywordIndex()
	if err := idx.Index(ragChunks); err != nil {
		return ""
	}
	results, err := idx.Search(question, 5, nil)
	if err != nil || len(results) == 0 {
		// 兜底：返回合同开头
		return "\n\n## 合同内容（节选）\n" + truncateRunes(contractText, 1200)
	}
	var sb strings.Builder
	sb.WriteString("\n\n## 合同相关条款\n")
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("【条款%d】\n%s\n\n", i+1, truncateRunes(r.Content, 500)))
	}
	return sb.String()
}

func splitContractText(text string, size int) []string {
	runes := []rune(text)
	var chunks []string
	for i := 0; i < len(runes); i += size {
		end := i + size
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
	}
	return chunks
}

func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}

func buildQASystemPrompt(c *contract.Contract, contractType, knowledgeCtx, contractCtx string) string {
	var meta strings.Builder
	meta.WriteString("你是一名专业的合同法律助手。请基于用户提供的合同内容回答问题，遵循以下要求：\n")
	meta.WriteString("1. 优先依据合同条款作答，引用条款时标注『据合同相关条款』。\n")
	meta.WriteString("2. 若合同未涉及或信息不足，明确说明并给出一般性法律建议。\n")
	meta.WriteString("3. 不得虚构法律条文；引用法规须准确。\n")
	meta.WriteString("4. 回答简洁专业，必要时分点陈述。\n\n")

	meta.WriteString("## 合同基本信息\n")
	meta.WriteString(fmt.Sprintf("- 标题: %s\n", c.Title))
	if contractType != "" {
		meta.WriteString(fmt.Sprintf("- 合同类型: %s\n", contractType))
	}
	if c.PartyA != "" || c.PartyB != "" {
		meta.WriteString(fmt.Sprintf("- 甲方: %s；乙方: %s\n", c.PartyA, c.PartyB))
	}
	if c.Amount > 0 {
		meta.WriteString(fmt.Sprintf("- 合同金额: %.2f\n", c.Amount))
	}

	if knowledgeCtx != "" {
		meta.WriteString(knowledgeCtx)
	}
	if contractCtx != "" {
		meta.WriteString(contractCtx)
	}
	return meta.String()
}

func buildQAMessages(systemPrompt string, history []QAMessage, question string) []*schema.Message {
	msgs := []*schema.Message{{Role: schema.System, Content: systemPrompt}}
	for _, m := range history {
		role := schema.User
		if m.Role == RoleAssistant {
			role = schema.Assistant
		}
		msgs = append(msgs, &schema.Message{Role: role, Content: m.Content})
	}
	msgs = append(msgs, &schema.Message{Role: schema.User, Content: question})
	return msgs
}

// ListMessages 获取会话消息历史
func (s *QAService) ListMessages(ctx context.Context, sessionID, userID uint64, limit int) ([]MessageResponse, error) {
	if _, err := s.getSession(ctx, sessionID, userID); err != nil {
		return nil, err
	}
	msgs, err := s.repo.ListBySession(ctx, sessionID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]MessageResponse, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, MessageResponse{
			ID:        m.ID,
			SessionID: m.SessionID,
			Role:      m.Role,
			Content:   m.Content,
			Tokens:    m.Tokens,
			CreatedAt: m.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return out, nil
}

// DeleteMessages 删除会话全部问答消息（用于重置对话）
func (s *QAService) DeleteMessages(ctx context.Context, sessionID, userID uint64) error {
	if _, err := s.getSession(ctx, sessionID, userID); err != nil {
		return err
	}
	return s.repo.DeleteBySession(ctx, sessionID)
}

