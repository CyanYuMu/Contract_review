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

type ReviewOrchestrator struct {
	llmGenerate     func(ctx context.Context, messages []*schema.Message) (*schema.Message, error)
	clauseAgent     *ClauseAgent
	riskAgent       *CandidateRiskAgent
	suggestionAgent *SuggestionAgent
	qualityGate     *QualityGate
	config          OrchestratorConfig
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

// ReviewCallbacks belongs to one review invocation. Keeping callbacks in the
// call scope makes a shared ReviewOrchestrator safe to reuse across concurrent
// review runs without one request overwriting another request's SSE sink.
type ReviewCallbacks struct {
	OnProgress func(event ProgressEvent)
	OnFinding  func(finding RiskFinding)
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

func (c ReviewCallbacks) emitProgress(phase, agent, status, message string, progress float64, data interface{}) {
	if c.OnProgress != nil {
		c.OnProgress(ProgressEvent{
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

func (c ReviewCallbacks) emitFinding(finding RiskFinding) {
	if c.OnFinding != nil {
		c.OnFinding(finding)
	}
}

// ReviewContract 执行完整的合同审阅流程（DAG 工作流）
func (o *ReviewOrchestrator) ReviewContract(
	ctx context.Context,
	contractText string,
	meta ContractMeta,
	callbacks ReviewCallbacks,
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
	callbacks.emitProgress("prepare", "Orchestrator", "running", "正在准备审阅...", 0.05, nil)

	// ============ Phase 2: 条款拆分（ClauseAgent） ============
	callbacks.emitProgress("clause_split", "ClauseAgent", "running", "正在进行智能条款拆分...", 0.1, nil)

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

	// 条款级路由：跳过首部/签署页/送达等 boilerplate 条款，不检索不审阅。
	reviewClauses, skipped := filterReviewableClauses(clauses)

	callbacks.emitProgress("clause_split", "ClauseAgent", "completed",
		fmt.Sprintf("条款拆分完成，共 %d 个条款", len(clauses)), 0.2, clauses)

	global.Log.Info("Phase 2 完成",
		zap.Int("clauseCount", len(clauses)),
		zap.Int("reviewClauseCount", len(reviewClauses)),
		zap.Int("skippedBoilerplate", skipped))

	// ============ Phase 3: 风险识别（知识库候选驱动 DAG） ============
	callbacks.emitProgress("risk_identify", "CandidateRiskAgent", "running",
		fmt.Sprintf("正在检索 %d 个条款的知识库候选风险点...", len(reviewClauses)), 0.25, nil)

	findings, riskSteps, err := o.riskAgent.ExecuteBatchWithCallback(
		ctx,
		reviewClauses,
		meta,
		func(index int, clause Clause, candidates []RiskCandidate, completed int, total int) {
			progress := 0.25
			if total > 0 {
				progress += 0.15 * float64(completed) / float64(total)
			}
			callbacks.emitProgress("candidate_retrieve", "CandidateRiskAgent", "running",
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
			callbacks.emitProgress("risk_identify", "CandidateRiskAgent", "running",
				fmt.Sprintf("条款审阅进度: %d/%d，候选 %d 条，本条发现 %d 个风险点", completed, total, len(candidates), len(clauseFindings)),
				progress,
				data)
			for _, finding := range clauseFindings {
				callbacks.emitProgress("risk_identify", "CandidateRiskAgent", "running",
					fmt.Sprintf("命中风险: %s (%s)，依据 %d 条", finding.RiskType, finding.RiskLevel, len(finding.LegalBasis)),
					progress,
					progressDataFromFinding(finding))
				callbacks.emitFinding(finding)
			}
		},
		nil, // 正常审阅无反思反馈
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

	callbacks.emitProgress("risk_identify", "CandidateRiskAgent", "completed",
		fmt.Sprintf("风险识别完成: %d 个风险点 (%d 个已验证)", len(findings), verifiedCount), 0.6,
		map[string]int{"total": len(findings), "verified": verifiedCount})

	global.Log.Info("Phase 3 完成",
		zap.Int("findingsCount", len(findings)),
		zap.Int("verifiedCount", verifiedCount),
		zap.Int("stepsCount", len(riskSteps)))

	// ============ Phase 4: 修改建议（SuggestionAgent） ============
	callbacks.emitProgress("suggestion", "SuggestionAgent", "running",
		"正在生成修改建议...", 0.65, nil)

	if suggestions := suggestionsFromFindings(findings); len(suggestions) == len(findings) {
		report.Suggestions = suggestions
		callbacks.emitProgress("suggestion", "CandidateRiskAgent", "completed",
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

	callbacks.emitProgress("suggestion", "SuggestionAgent", "completed",
		fmt.Sprintf("修改建议生成完成: %d 条建议", len(report.Suggestions)), 0.8, nil)

	global.Log.Info("Phase 4 完成",
		zap.Int("suggestionCount", len(report.Suggestions)))

	// ============ Phase 5: 质量评估 + Reflection 反思重审 ============
	// 真 Reflection：质量门评估产出评分与缺口后，若存在"无风险发现的缺口条款"且质量信号要求重审，
	// 则把缺口作为反思反馈注入 CandidateRiskAgent，对缺口条款定向重审（不做全量重审），
	// 合并新发现后重新评估，直至无缺口 / 质量达标 / 达到最大反思轮数。
	callbacks.emitProgress("quality", "QualityGate", "running",
		"正在进行质量评估...", 0.85, nil)

	report.ReflectionCount = 0
	for {
		eval, err := o.qualityGate.Evaluate(ctx, report)
		if err != nil {
			global.Log.Warn("质量评估失败", zap.Error(err))
			break
		}
		report.QualityScore = eval.OverallScore
		global.Log.Info("质量评估结果",
			zap.Float64("score", eval.OverallScore),
			zap.Int("reflection", report.ReflectionCount),
			zap.Strings("gaps", eval.CriticalGaps))

		gapClauses := findGapClauses(reviewClauses, report.Findings)
		qualitySignal := eval.ShouldRetry || eval.OverallScore < o.qualityGate.config.ConfidenceThreshold
		canReflect := o.qualityGate.config.Enabled &&
			report.ReflectionCount < o.qualityGate.config.MaxRetries &&
			len(gapClauses) > 0 &&
			qualitySignal

		if !canReflect {
			callbacks.emitProgress("quality", "QualityGate", "completed",
				fmt.Sprintf("质量评估完成 (评分: %.2f)", eval.OverallScore), 0.9, eval)
			break
		}

		report.ReflectionCount++
		callbacks.emitProgress("reflection", "QualityGate", "running",
			fmt.Sprintf("反思第 %d 轮：定向重审 %d 个缺口条款", report.ReflectionCount, len(gapClauses)),
			0.86,
			map[string]interface{}{
				"reflection":  report.ReflectionCount,
				"gap_clauses": len(gapClauses),
			})

		hints := buildReflectionHints(eval)
		newFindings, _, rErr := o.riskAgent.ExecuteBatchWithCallback(
			ctx,
			gapClauses,
			meta,
			func(index int, clause Clause, candidates []RiskCandidate, completed int, total int) {
				callbacks.emitProgress("reflection", "CandidateRiskAgent", "running",
					fmt.Sprintf("反思重审候选检索 %d/%d", completed, total), 0.87, nil)
			},
			func(index int, clause Clause, candidates []RiskCandidate, clauseFindings []RiskFinding, completed int, total int) {
				for _, finding := range clauseFindings {
					callbacks.emitFinding(finding)
				}
			},
			hints,
		)
		if rErr != nil {
			global.Log.Warn("反思重审失败", zap.Int("reflection", report.ReflectionCount), zap.Error(rErr))
			break
		}

		report.Findings = MergeFindings(append(report.Findings, newFindings...))
		global.Log.Info("反思重审完成",
			zap.Int("reflection", report.ReflectionCount),
			zap.Int("newFindings", len(newFindings)),
			zap.Int("totalFindings", len(report.Findings)))
		// 继续下一轮质量评估
	}

	// ============ Phase 6: 报告生成 ============
	callbacks.emitProgress("report", "Orchestrator", "running",
		"正在生成审阅报告...", 0.95, nil)

	report.OverallRisk = calculateOverallRisk(report.Findings)
	report.Summary = generateSummary(report)
	report.Duration = time.Since(startTime)

	callbacks.emitProgress("report", "Orchestrator", "completed",
		"审阅完成", 1.0, report)

	global.Log.Info("ReviewOrchestrator 审阅完成",
		zap.Duration("totalDuration", report.Duration),
		zap.Int("totalFindings", len(report.Findings)),
		zap.Int("totalSuggestions", len(report.Suggestions)),
		zap.Float64("qualityScore", report.QualityScore),
		zap.Int("tokensUsed", report.TokensUsed))

	return report, nil
}

// boilerplateTitleKeywords 命中标题即视为 boilerplate 条款（首部/签署页/送达等），跳过审阅。
var boilerplateTitleKeywords = []string{
	"序言", "首部", "签署页", "签章页", "落款", "签章", "签字盖章", "盖章", "送达",
}

// isBoilerplateClause 判断条款是否为 boilerplate（无审阅价值），确定性规则、零 LLM。
func isBoilerplateClause(clause Clause) bool {
	if clause.ID == "preamble" {
		return true
	}
	title := clause.Title
	for _, kw := range boilerplateTitleKeywords {
		if strings.Contains(title, kw) {
			return true
		}
	}
	return false
}

// filterReviewableClauses 过滤掉 boilerplate 条款，返回可审阅条款与跳过数量。
func filterReviewableClauses(clauses []Clause) (reviewable []Clause, skipped int) {
	reviewable = make([]Clause, 0, len(clauses))
	for _, c := range clauses {
		if isBoilerplateClause(c) {
			skipped++
			continue
		}
		reviewable = append(reviewable, c)
	}
	return reviewable, skipped
}

// findGapClauses 返回没有任何风险发现（含 ClauseIDs）覆盖的条款，作为反思重审的"缺口条款"。
func findGapClauses(clauses []Clause, findings []RiskFinding) []Clause {
	covered := make(map[string]bool, len(findings))
	for _, f := range findings {
		if f.ClauseID != "" {
			covered[f.ClauseID] = true
		}
		for _, cid := range f.ClauseIDs {
			if cid != "" {
				covered[cid] = true
			}
		}
	}
	var gaps []Clause
	for _, c := range clauses {
		if !covered[c.ID] {
			gaps = append(gaps, c)
		}
	}
	return gaps
}

// buildReflectionHints 由质量评估结果构建反思反馈，注入下一轮重审提示词。
func buildReflectionHints(eval *QualityEvaluation) []string {
	if eval == nil {
		return nil
	}
	hints := make([]string, 0, len(eval.CriticalGaps)+1)
	hints = append(hints, eval.CriticalGaps...)
	if strings.TrimSpace(eval.Feedback) != "" {
		hints = append(hints, "改进建议："+strings.TrimSpace(eval.Feedback))
	}
	return hints
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
