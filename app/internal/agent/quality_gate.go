package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"contract_review/app/internal/global"

	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

// QualityGate 质量评估门 — Reflection 模式实现
// 参考 https://www.waylandz.com/ai-agent-book/ 第11章 Reflection 模式
// 核心：评估输出 → 生成反馈 → 带反馈重试
type QualityGate struct {
	llmGenerate func(ctx context.Context, messages []*schema.Message) (*schema.Message, error)
	config      ReflectionConfig
}

// NewQualityGate 创建质量评估门
func NewQualityGate(
	llmGenerate func(ctx context.Context, messages []*schema.Message) (*schema.Message, error),
	config ReflectionConfig,
) *QualityGate {
	return &QualityGate{
		llmGenerate: llmGenerate,
		config:      config,
	}
}

// Evaluate 评估审阅报告质量
func (qg *QualityGate) Evaluate(ctx context.Context, report *ReviewReport) (*QualityEvaluation, error) {
	if !qg.config.Enabled {
		return &QualityEvaluation{
			OverallScore: 0.75,
			ShouldRetry:  false,
		}, nil
	}

	evalPrompt := buildEvaluationPrompt(report, qg.config.Criteria)

	result, err := RunSimple(ctx, qualityEvalSystemPrompt, evalPrompt, qg.llmGenerate)
	if err != nil {
		global.Log.Warn("质量评估 LLM 调用失败，使用默认评分", zap.Error(err))
		return &QualityEvaluation{
			OverallScore: 0.6,
			Feedback:     "质量评估服务不可用",
			ShouldRetry:  false,
		}, nil
	}

	eval := parseEvaluation(result)

	eval = qg.applyGuardrails(eval, report)

	global.Log.Info("质量评估完成",
		zap.Float64("overallScore", eval.OverallScore),
		zap.Bool("shouldRetry", eval.ShouldRetry),
		zap.Int("criticalGaps", len(eval.CriticalGaps)))

	return eval, nil
}

// ShouldReflect 判断是否需要触发 Reflection
func (qg *QualityGate) ShouldReflect(eval *QualityEvaluation, currentRetry int) bool {
	if !qg.config.Enabled {
		return false
	}
	if currentRetry >= qg.config.MaxRetries {
		return false
	}
	if eval.OverallScore >= qg.config.ConfidenceThreshold {
		return false
	}
	return eval.ShouldRetry
}

// BuildReflectionFeedback 构建反馈信息供重试使用
func (qg *QualityGate) BuildReflectionFeedback(eval *QualityEvaluation) map[string]interface{} {
	return map[string]interface{}{
		"reflection_feedback": eval.Feedback,
		"critical_gaps":       eval.CriticalGaps,
		"overall_score":       eval.OverallScore,
		"criteria_scores":     eval.CriteriaScores,
		"improvement_needed":  true,
	}
}

// applyGuardrails 确定性护栏 — 覆盖 LLM 评估中的不合理判断
func (qg *QualityGate) applyGuardrails(eval *QualityEvaluation, report *ReviewReport) *QualityEvaluation {
	// 护栏 1: 没有任何高风险发现但声称覆盖完整 → 降低置信度
	highRiskCount := 0
	for _, f := range report.Findings {
		if f.RiskLevel == "高" {
			highRiskCount++
		}
	}
	if highRiskCount == 0 && eval.OverallScore > 0.9 && len(report.Findings) > 0 {
		eval.OverallScore = 0.65
		eval.Feedback += " 未发现高风险点但评分过高，建议复查关键条款。"
		eval.ShouldRetry = true
	}

	// 护栏 2: 存在未验证的风险发现 → 标记需要重试
	unverifiedCount := 0
	for _, f := range report.Findings {
		if !f.Verified {
			unverifiedCount++
		}
	}
	if unverifiedCount > 0 {
		eval.CriticalGaps = append(eval.CriticalGaps,
			fmt.Sprintf("%d 个风险点未经 RAG 验证", unverifiedCount))
		if report.ReflectionCount < qg.config.MaxRetries {
			eval.ShouldRetry = true
		}
	}

	// 护栏 3: 审阅结果密度过低 → 可能遗漏
	if report.WordCount > 0 {
		riskDensity := float64(len(report.Findings)) / (float64(report.WordCount) / 10000.0)
		if riskDensity < 1.0 && eval.OverallScore > 0.8 {
			eval.OverallScore = 0.55
			eval.Feedback += fmt.Sprintf(" 风险点密度偏低(%.1f个/万字)，可能存在遗漏。", riskDensity)
			eval.ShouldRetry = true
		}
	}

	// 护栏 4: 修改建议数量与风险发现数量严重不匹配
	if len(report.Suggestions) < len(report.Findings)/2 && len(report.Findings) > 2 {
		eval.CriticalGaps = append(eval.CriticalGaps,
			fmt.Sprintf("修改建议(%d)远少于风险发现(%d)", len(report.Suggestions), len(report.Findings)))
		eval.ShouldRetry = true
	}

	// 护栏 5: 达到最大重试次数 → 强制停止
	if report.ReflectionCount >= qg.config.MaxRetries {
		eval.ShouldRetry = false
	}

	return eval
}

func buildEvaluationPrompt(report *ReviewReport, criteria []string) string {
	return fmt.Sprintf(`请评估以下合同审阅报告的质量。

## 审阅统计
- 条款总数: %d
- 风险发现总数: %d
- 已验证风险: %d / %d
- 修改建议数: %d
- 合同字数: %d
- 当前反思次数: %d

## 风险发现摘要
%s

## 修改建议摘要
%s

## 评估维度
%s

请对每个维度打分 0.0-1.0，然后给出加权平均分。
如果总分低于 0.7，给出具体改进建议。

输出 JSON 格式:
{
  "overall_score": 0.0-1.0,
  "criteria_scores": {"维度名": 分数},
  "critical_gaps": ["关键缺口描述"],
  "feedback": "具体改进建议",
  "should_retry": true/false
}`,
		len(report.Clauses),
		len(report.Findings),
		countVerifiedInReport(report), len(report.Findings),
		len(report.Suggestions),
		report.WordCount,
		report.ReflectionCount,
		summarizeFindings(report.Findings),
		summarizeSuggestions(report.Suggestions),
		formatCriteria(criteria))
}

func countVerifiedInReport(report *ReviewReport) int {
	count := 0
	for _, f := range report.Findings {
		if f.Verified {
			count++
		}
	}
	return count
}

func summarizeFindings(findings []RiskFinding) string {
	if len(findings) == 0 {
		return "无风险发现"
	}
	result := ""
	for i, f := range findings {
		if i >= 5 {
			result += fmt.Sprintf("...及其他 %d 个风险点\n", len(findings)-5)
			break
		}
		verified := "待验证"
		if f.Verified {
			verified = "已验证"
		}
		result += fmt.Sprintf("- [%s][%s] %s: %s (置信度: %.2f)\n",
			f.RiskLevel, verified, f.RiskType, truncateStr(f.RiskDescription, 50), f.Confidence)
	}
	return result
}

func summarizeSuggestions(suggestions []Suggestion) string {
	if len(suggestions) == 0 {
		return "无修改建议"
	}
	result := ""
	for i, s := range suggestions {
		if i >= 5 {
			result += fmt.Sprintf("...及其他 %d 条建议\n", len(suggestions)-5)
			break
		}
		result += fmt.Sprintf("- [%s] %s\n", s.Priority, truncateStr(s.Reason, 50))
	}
	return result
}

func formatCriteria(criteria []string) string {
	criteriaDesc := map[string]string{
		"completeness":       "完整性 - 是否覆盖所有条款类别的审阅",
		"legal_accuracy":     "法律准确性 - 法律依据是否准确、是否经过RAG验证",
		"risk_coverage":      "风险覆盖度 - 高中低风险覆盖是否合理",
		"suggestion_quality": "建议质量 - 修改建议是否具体可行",
		"consistency":        "一致性 - 审阅风格和术语是否一致",
	}

	result := ""
	for _, c := range criteria {
		if desc, ok := criteriaDesc[c]; ok {
			result += fmt.Sprintf("- %s\n", desc)
		} else {
			result += fmt.Sprintf("- %s\n", c)
		}
	}
	return result
}

func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

func parseEvaluation(content string) *QualityEvaluation {
	eval := &QualityEvaluation{
		OverallScore:   0.6,
		CriteriaScores: make(map[string]float64),
	}

	jsonStr := extractJSONBlock(content)
	if jsonStr != "" {
		if err := json.Unmarshal([]byte(jsonStr), eval); err == nil {
			return eval
		}
	}

	eval.Feedback = "评估结果解析失败: " + truncateStr(content, 200)
	return eval
}

func extractJSONBlock(text string) string {
	start := -1
	depth := 0
	for i, c := range text {
		if c == '{' {
			if depth == 0 {
				start = i
			}
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 && start != -1 {
				return text[start : i+1]
			}
		}
	}
	return ""
}

const qualityEvalSystemPrompt = `你是一个合同审阅质量评估专家。你的任务是客观评估审阅报告的质量。

## 评估原则
1. 基于客观指标评估，不做主观偏好判断
2. 关注审阅的完整性、准确性和实用性
3. 给出具体的改进方向，而不是模糊的评价
4. 评分要校准：0.5 为及格线，0.7 为良好，0.9 为优秀

## 关键检查点
- 是否覆盖了合同的主要条款类别？
- 风险发现是否有审阅规范依据？
- 修改建议是否具体可行？
- 高/中/低风险的分布是否合理？
- 是否存在明显遗漏的重要条款？`
