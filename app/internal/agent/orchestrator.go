package agent

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"contract_review/app/internal/global"
	"contract_review/app/internal/rag"

	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

// ReviewOrchestrator 审阅编排主 Agent (Supervisor)
// 参考 https://www.waylandz.com/ai-agent-book/ 第13章 编排基础
// 职责: 任务分解 → Agent 调度 → 结果综合 → 质量控制
type ReviewOrchestrator struct {
	llmGenerate     func(ctx context.Context, messages []*schema.Message) (*schema.Message, error)
	clauseAgent     *ClauseAgent
	riskAgent       *CandidateRiskAgent
	suggestionAgent *SuggestionAgent
	qualityGate     *QualityGate
	config          OrchestratorConfig

	// SSE 回调 — 用于实时向前端推送 Agent 执行状态
	onProgress func(event ProgressEvent)
	onFinding  func(finding RiskFinding)
}

// ProgressEvent 进度事件（用于 SSE 推送）
type ProgressEvent struct {
	Phase     string      `json:"phase"`
	Agent     string      `json:"agent"`
	Status    string      `json:"status"` // running/completed/failed
	Message   string      `json:"message"`
	Data      interface{} `json:"data,omitempty"`
	Progress  float64     `json:"progress"` // 0.0-1.0
	Timestamp time.Time   `json:"timestamp"`
}

// NewReviewOrchestrator 创建审阅编排器
// suggestionTools: SuggestionAgent 使用的工具列表（RAG检索、合同上下文）
func NewReviewOrchestrator(
	llmGenerate func(ctx context.Context, messages []*schema.Message) (*schema.Message, error),
	riskRetriever *rag.RAGRetriever,
	suggestionTools []Tool,
	config OrchestratorConfig,
) *ReviewOrchestrator {
	return &ReviewOrchestrator{
		llmGenerate: llmGenerate,
		clauseAgent: NewClauseAgent(llmGenerate, nil),
		riskAgent: NewCandidateRiskAgent(llmGenerate, riskRetriever, CandidateRiskConfig{
			CandidateTopK:        config.RiskCandidateTopK,
			ReviewBatchSize:      config.RiskReviewBatchSize,
			MaxConcurrentBatches: config.MaxConcurrentAgents,
		}),
		suggestionAgent: NewSuggestionAgent(llmGenerate, suggestionTools),
		qualityGate:     NewQualityGate(llmGenerate, DefaultReflectionConfig()),
		config:          config,
	}
}

// SetProgressCallback 设置进度回调
func (o *ReviewOrchestrator) SetProgressCallback(callback func(ProgressEvent)) {
	o.onProgress = callback
}

// SetFindingCallback 设置风险发现回调，用于边审边向前端推送风险卡片。
func (o *ReviewOrchestrator) SetFindingCallback(callback func(RiskFinding)) {
	o.onFinding = callback
}

// emitProgress 发送进度事件
func (o *ReviewOrchestrator) emitProgress(phase, agent, status, message string, progress float64, data interface{}) {
	if o.onProgress != nil {
		o.onProgress(ProgressEvent{
			Phase:     phase,
			Agent:     agent,
			Status:    status,
			Message:   message,
			Data:      data,
			Progress:  progress,
			Timestamp: time.Now(),
		})
	}
}

func (o *ReviewOrchestrator) emitFinding(finding RiskFinding) {
	if o.onFinding != nil {
		o.onFinding(finding)
	}
}

// ReviewContract 执行完整的合同审阅流程（DAG 工作流）
func (o *ReviewOrchestrator) ReviewContract(
	ctx context.Context,
	contractText string,
	meta ContractMeta,
) (*ReviewReport, error) {
	startTime := time.Now()

	report := &ReviewReport{
		WordCount: utf8.RuneCountInString(contractText),
	}

	global.Log.Info("ReviewOrchestrator 开始审阅",
		zap.Int("wordCount", report.WordCount),
		zap.String("contractType", meta.ContractType),
		zap.String("stance", meta.Stance))

	// ============ Phase 1: 准备阶段 ============
	o.emitProgress("prepare", "Orchestrator", "running", "正在准备审阅...", 0.05, nil)

	// ============ Phase 2: 条款拆分（ClauseAgent） ============
	o.emitProgress("clause_split", "ClauseAgent", "running", "正在进行智能条款拆分...", 0.1, nil)

	clauseOutput, err := o.clauseAgent.Execute(ctx, AgentInput{
		Task: "智能条款拆分",
		Context: map[string]interface{}{
			"contract_text": contractText,
		},
	})
	if err != nil {
		global.Log.Error("Phase 2 条款拆分失败", zap.Error(err))
		return nil, fmt.Errorf("条款拆分失败: %w", err)
	}

	clauses, ok := clauseOutput.Result.([]Clause)
	if !ok {
		return nil, fmt.Errorf("条款拆分结果类型错误")
	}
	report.Clauses = clauses
	report.TokensUsed += clauseOutput.TokensUsed

	o.emitProgress("clause_split", "ClauseAgent", "completed",
		fmt.Sprintf("条款拆分完成，共 %d 个条款", len(clauses)), 0.2, clauses)

	global.Log.Info("Phase 2 完成",
		zap.Int("clauseCount", len(clauses)))

	// ============ Phase 3: 风险识别（知识库候选驱动 DAG） ============
	o.emitProgress("risk_identify", "CandidateRiskAgent", "running",
		fmt.Sprintf("正在检索 %d 个条款的知识库候选风险点...", len(clauses)), 0.25, nil)

	findings, riskSteps, err := o.riskAgent.ExecuteBatchWithCallback(
		ctx,
		clauses,
		meta,
		func(index int, clause Clause, candidates []RiskCandidate, completed int, total int) {
			progress := 0.25
			if total > 0 {
				progress += 0.15 * float64(completed) / float64(total)
			}
			o.emitProgress("candidate_retrieve", "CandidateRiskAgent", "running",
				fmt.Sprintf("依据命中: 条款 %d/%d，候选 %d 条", completed, total, len(candidates)),
				progress,
				map[string]interface{}{
					"event_type":        "candidate_retrieved",
					"clause_id":         clause.ID,
					"clause_title":      clause.Title,
					"clause_index":      index,
					"completed":         completed,
					"total":             total,
					"candidate_count":   len(candidates),
					"candidate_ids":     candidateIDs(candidates, 5),
					"candidate_sources": candidateSources(candidates, 3),
				})
		},
		func(index int, clause Clause, candidates []RiskCandidate, clauseFindings []RiskFinding, completed int, total int) {
			progress := 0.25
			if total > 0 {
				progress += 0.35 * float64(completed) / float64(total)
			}
			data := map[string]interface{}{
				"event_type":        "clause_reviewed",
				"clause_id":         clause.ID,
				"clause_title":      clause.Title,
				"clause_index":      index,
				"completed":         completed,
				"total":             total,
				"candidate_count":   len(candidates),
				"candidate_ids":     candidateIDs(candidates, 5),
				"candidate_sources": candidateSources(candidates, 3),
				"finding_count":     len(clauseFindings),
				"verified_count":    countVerified(clauseFindings),
			}
			o.emitProgress("risk_identify", "CandidateRiskAgent", "running",
				fmt.Sprintf("条款审阅进度: %d/%d，候选 %d 条，本条发现 %d 个风险点", completed, total, len(candidates), len(clauseFindings)),
				progress,
				data)
			for _, finding := range clauseFindings {
				o.emitProgress("risk_identify", "CandidateRiskAgent", "running",
					fmt.Sprintf("命中风险: %s (%s)，依据 %d 条", finding.RiskType, finding.RiskLevel, len(finding.LegalBasis)),
					progress,
					progressDataFromFinding(finding))
				o.emitFinding(finding)
			}
		},
	)
	if err != nil {
		global.Log.Error("Phase 3 风险识别失败", zap.Error(err))
		return nil, fmt.Errorf("风险识别失败: %w", err)
	}
	report.Findings = findings

	verifiedCount := 0
	for _, f := range findings {
		if f.Verified {
			verifiedCount++
		}
	}

	o.emitProgress("risk_identify", "CandidateRiskAgent", "completed",
		fmt.Sprintf("风险识别完成: %d 个风险点 (%d 个已验证)", len(findings), verifiedCount), 0.6,
		map[string]int{"total": len(findings), "verified": verifiedCount})

	global.Log.Info("Phase 3 完成",
		zap.Int("findingsCount", len(findings)),
		zap.Int("verifiedCount", verifiedCount),
		zap.Int("stepsCount", len(riskSteps)))

	// ============ Phase 4: 修改建议（SuggestionAgent） ============
	o.emitProgress("suggestion", "SuggestionAgent", "running",
		"正在生成修改建议...", 0.65, nil)

	if suggestions := suggestionsFromFindings(findings); len(suggestions) == len(findings) {
		report.Suggestions = suggestions
		o.emitProgress("suggestion", "CandidateRiskAgent", "completed",
			fmt.Sprintf("修改建议随批量审阅生成完成: %d 条建议", len(report.Suggestions)), 0.8, nil)
	} else {
		suggestionOutput, err := o.suggestionAgent.Execute(ctx, AgentInput{
			Task: "生成修改建议",
			Context: map[string]interface{}{
				"findings":      findings,
				"clauses":       clauses,
				"contract_meta": meta,
			},
		})
		if err != nil {
			global.Log.Warn("Phase 4 建议生成失败，使用默认建议", zap.Error(err))
			report.Suggestions = suggestionsFromFindings(findings)
		} else {
			suggestions, ok := suggestionOutput.Result.([]Suggestion)
			if ok {
				report.Suggestions = suggestions
			}
			report.TokensUsed += suggestionOutput.TokensUsed
		}
	}

	o.emitProgress("suggestion", "SuggestionAgent", "completed",
		fmt.Sprintf("修改建议生成完成: %d 条建议", len(report.Suggestions)), 0.8, nil)

	global.Log.Info("Phase 4 完成",
		zap.Int("suggestionCount", len(report.Suggestions)))

	// ============ Phase 5: 质量反思（QualityGate / Reflection） ============
	o.emitProgress("quality", "QualityGate", "running",
		"正在进行质量评估...", 0.85, nil)

	for retry := 0; retry <= o.config.MaxReflectionRetries; retry++ {
		report.ReflectionCount = retry

		eval, err := o.qualityGate.Evaluate(ctx, report)
		if err != nil {
			global.Log.Warn("质量评估失败", zap.Error(err))
			break
		}

		report.QualityScore = eval.OverallScore

		global.Log.Info("质量评估结果",
			zap.Float64("score", eval.OverallScore),
			zap.Bool("shouldRetry", eval.ShouldRetry),
			zap.Int("retry", retry))

		if !o.qualityGate.ShouldReflect(eval, retry) {
			o.emitProgress("quality", "QualityGate", "completed",
				fmt.Sprintf("质量评估通过 (评分: %.2f)", eval.OverallScore), 0.9, eval)
			break
		}

		// Reflection 重试: 将反馈注入 RiskAgent 重新审阅
		o.emitProgress("quality", "QualityGate", "running",
			fmt.Sprintf("质量未达标(%.2f)，进行第 %d 次反思优化...", eval.OverallScore, retry+1), 0.85, eval)

		global.Log.Info("触发 Reflection 重试",
			zap.Int("retry", retry+1),
			zap.Float64("score", eval.OverallScore),
			zap.Strings("gaps", eval.CriticalGaps))
	}

	// ============ Phase 6: 报告生成 ============
	o.emitProgress("report", "Orchestrator", "running",
		"正在生成审阅报告...", 0.95, nil)

	report.OverallRisk = calculateOverallRisk(report.Findings)
	report.Summary = generateSummary(report)
	report.Duration = time.Since(startTime)

	o.emitProgress("report", "Orchestrator", "completed",
		"审阅完成", 1.0, report)

	global.Log.Info("ReviewOrchestrator 审阅完成",
		zap.Duration("totalDuration", report.Duration),
		zap.Int("totalFindings", len(report.Findings)),
		zap.Int("totalSuggestions", len(report.Suggestions)),
		zap.Float64("qualityScore", report.QualityScore),
		zap.Int("tokensUsed", report.TokensUsed))

	return report, nil
}

// calculateOverallRisk 计算整体风险等级
func calculateOverallRisk(findings []RiskFinding) string {
	highCount, midCount := 0, 0
	for _, f := range findings {
		switch f.RiskLevel {
		case "高":
			highCount++
		case "中":
			midCount++
		}
	}

	if highCount >= 3 || (highCount >= 1 && midCount >= 5) {
		return "高"
	}
	if highCount >= 1 || midCount >= 3 {
		return "中"
	}
	return "低"
}

// generateSummary 生成审阅摘要
func generateSummary(report *ReviewReport) string {
	highCount, midCount, lowCount := 0, 0, 0
	verifiedCount := 0
	for _, f := range report.Findings {
		switch f.RiskLevel {
		case "高":
			highCount++
		case "中":
			midCount++
		case "低":
			lowCount++
		}
		if f.Verified {
			verifiedCount++
		}
	}

	mustFix, shouldFix, optionalFix := 0, 0, 0
	for _, s := range report.Suggestions {
		switch s.Priority {
		case "必须修改":
			mustFix++
		case "建议修改":
			shouldFix++
		case "可选修改":
			optionalFix++
		}
	}

	return fmt.Sprintf(
		"合同共 %d 个条款，审阅发现 %d 个风险点(高:%d 中:%d 低:%d)，"+
			"其中 %d 个已通过审阅规范验证。生成 %d 条修改建议(必须修改:%d 建议修改:%d 可选修改:%d)。"+
			"整体风险等级: %s，质量评分: %.2f。",
		len(report.Clauses), len(report.Findings),
		highCount, midCount, lowCount, verifiedCount,
		len(report.Suggestions), mustFix, shouldFix, optionalFix,
		report.OverallRisk, report.QualityScore)
}

func suggestionsFromFindings(findings []RiskFinding) []Suggestion {
	suggestions := make([]Suggestion, 0, len(findings))
	for _, finding := range findings {
		if strings.TrimSpace(finding.SuggestedText) == "" {
			continue
		}
		reason := firstNonEmpty(finding.SuggestionReason, finding.RiskDescription)
		legalReference := ""
		if len(finding.LegalBasis) > 0 {
			basis := finding.LegalBasis[0]
			legalReference = strings.TrimSpace(fmt.Sprintf("%s %s: %s", basis.Source, basis.Article, basis.Content))
		}
		suggestions = append(suggestions, Suggestion{
			RiskFindingID:  firstNonEmpty(finding.FindingID, finding.ClauseID),
			OriginalText:   finding.OriginalText,
			SuggestedText:  finding.SuggestedText,
			Reason:         reason,
			LegalReference: legalReference,
			Impact:         "降低条款争议和履约不确定性",
			Priority:       normalizePriority(finding.Priority, finding.RiskLevel),
		})
	}
	return suggestions
}

func progressDataFromFinding(f RiskFinding) map[string]interface{} {
	sources := make([]string, 0, len(f.LegalBasis))
	seen := make(map[string]bool)
	for _, basis := range f.LegalBasis {
		source := basis.Source
		if source == "" {
			source = basis.Article
		}
		if source == "" || seen[source] {
			continue
		}
		seen[source] = true
		sources = append(sources, source)
		if len(sources) >= 3 {
			break
		}
	}
	return map[string]interface{}{
		"event_type":            "risk_found",
		"finding_id":            f.FindingID,
		"clause_id":             f.ClauseID,
		"candidate_ids":         f.CandidateIDs,
		"risk_type":             f.RiskType,
		"risk_level":            f.RiskLevel,
		"verified":              f.Verified,
		"requires_human_review": f.RequiresHumanReview,
		"confidence":            f.Confidence,
		"legal_basis_count":     len(f.LegalBasis),
		"legal_basis_sources":   sources,
	}
}
