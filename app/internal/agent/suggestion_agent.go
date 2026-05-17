package agent

import (
	"context"
	"fmt"
	"strings"

	"contract_review/app/internal/global"

	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

// SuggestionAgent 修改建议 Agent
// 基于验证过的风险发现和审阅规范生成具体的修改建议
type SuggestionAgent struct {
	llmGenerate func(ctx context.Context, messages []*schema.Message) (*schema.Message, error)
	tools       []Tool
	reactConfig ReactConfig
}

// NewSuggestionAgent 创建修改建议 Agent
func NewSuggestionAgent(
	llmGenerate func(ctx context.Context, messages []*schema.Message) (*schema.Message, error),
	tools []Tool,
) *SuggestionAgent {
	return &SuggestionAgent{
		llmGenerate: llmGenerate,
		tools:       tools,
		reactConfig: ReactConfig{
			MaxIterations:     5,
			MinIterations:     1,
			ObservationWindow: 3,
		},
	}
}

func (sa *SuggestionAgent) Name() string { return "SuggestionAgent" }

func (sa *SuggestionAgent) AvailableTools() []Tool { return sa.tools }

// Execute 生成修改建议
// input.Context 应包含:
//   - "findings": []RiskFinding
//   - "clauses": []Clause
//   - "contract_meta": ContractMeta
func (sa *SuggestionAgent) Execute(ctx context.Context, input AgentInput) (AgentOutput, error) {
	findings, _ := input.Context["findings"].([]RiskFinding)
	meta, _ := input.Context["contract_meta"].(ContractMeta)

	if len(findings) == 0 {
		return AgentOutput{
			Result: []Suggestion{},
		}, nil
	}

	verifiedFindings := filterVerified(findings)
	unverifiedFindings := filterUnverified(findings)

	global.Log.Info("SuggestionAgent 开始生成修改建议",
		zap.Int("totalFindings", len(findings)),
		zap.Int("verifiedFindings", len(verifiedFindings)),
		zap.Int("unverifiedFindings", len(unverifiedFindings)))

	taskPrompt := buildSuggestionPrompt(verifiedFindings, unverifiedFindings, meta)

	reactLoop := NewReactLoop(sa.reactConfig)
	output, err := reactLoop.Run(
		ctx,
		suggestionAgentSystemPrompt,
		taskPrompt,
		sa.tools,
		sa.llmGenerate,
	)

	if err != nil {
		global.Log.Error("SuggestionAgent 执行失败", zap.Error(err))
		return AgentOutput{}, err
	}

	suggestions := sa.extractSuggestions(output, findings)

	global.Log.Info("SuggestionAgent 完成",
		zap.Int("suggestionCount", len(suggestions)))

	return AgentOutput{
		Result:     suggestions,
		Thinking:   output.Thinking,
		TokensUsed: output.TokensUsed,
		Duration:   output.Duration,
	}, nil
}

func filterVerified(findings []RiskFinding) []RiskFinding {
	var result []RiskFinding
	for _, f := range findings {
		if f.Verified {
			result = append(result, f)
		}
	}
	return result
}

func filterUnverified(findings []RiskFinding) []RiskFinding {
	var result []RiskFinding
	for _, f := range findings {
		if !f.Verified {
			result = append(result, f)
		}
	}
	return result
}

func (sa *SuggestionAgent) extractSuggestions(output *AgentOutput, findings []RiskFinding) []Suggestion {
	resultStr, ok := output.Result.(string)
	if !ok || resultStr == "" {
		return sa.buildDefaultSuggestions(findings)
	}

	suggestions := parseSuggestionsFromText(resultStr, findings)
	if len(suggestions) == 0 {
		return sa.buildDefaultSuggestions(findings)
	}

	return suggestions
}

func (sa *SuggestionAgent) buildDefaultSuggestions(findings []RiskFinding) []Suggestion {
	var suggestions []Suggestion
	for i, f := range findings {
		suggestions = append(suggestions, Suggestion{
			RiskFindingID: fmt.Sprintf("risk-%d", i+1),
			OriginalText:  f.OriginalText,
			Reason:        f.RiskDescription,
			Priority:      mapRiskLevelToPriority(f.RiskLevel),
		})
	}
	return suggestions
}

func parseSuggestionsFromText(text string, findings []RiskFinding) []Suggestion {
	var suggestions []Suggestion

	sections := strings.Split(text, "建议")
	for i, section := range sections {
		if i == 0 || strings.TrimSpace(section) == "" {
			continue
		}

		suggestion := Suggestion{}

		lines := strings.Split(section, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "原文") {
				suggestion.OriginalText = extractValue(line)
			} else if strings.Contains(line, "修改后") || strings.Contains(line, "建议修改") {
				suggestion.SuggestedText = extractValue(line)
			} else if strings.Contains(line, "理由") || strings.Contains(line, "原因") {
				suggestion.Reason = extractValue(line)
			} else if strings.Contains(line, "依据") || strings.Contains(line, "法律") {
				suggestion.LegalReference = extractValue(line)
			} else if strings.Contains(line, "优先级") || strings.Contains(line, "级别") {
				suggestion.Priority = extractValue(line)
			}
		}

		if suggestion.Priority == "" {
			suggestion.Priority = "建议修改"
		}

		if i-1 < len(findings) {
			suggestion.RiskFindingID = findings[i-1].ClauseID
		}

		if suggestion.SuggestedText != "" || suggestion.Reason != "" {
			suggestions = append(suggestions, suggestion)
		}
	}

	return suggestions
}

func mapRiskLevelToPriority(riskLevel string) string {
	switch riskLevel {
	case "高":
		return "必须修改"
	case "中":
		return "建议修改"
	default:
		return "可选修改"
	}
}

func buildSuggestionPrompt(verified, unverified []RiskFinding, meta ContractMeta) string {
	var sb strings.Builder

	sb.WriteString("## 修改建议生成任务\n\n")
	sb.WriteString(fmt.Sprintf("合同类型: %s\n", meta.ContractType))
	sb.WriteString(fmt.Sprintf("审查立场: %s\n", meta.Stance))
	sb.WriteString(fmt.Sprintf("甲方: %s\n", meta.PartyA))
	sb.WriteString(fmt.Sprintf("乙方: %s\n\n", meta.PartyB))

	if len(verified) > 0 {
		sb.WriteString("## 已验证的风险点（需生成修改建议）\n\n")
		for i, f := range verified {
			sb.WriteString(fmt.Sprintf("### 风险点 %d\n", i+1))
			sb.WriteString(fmt.Sprintf("- 条款ID: %s\n", f.ClauseID))
			sb.WriteString(fmt.Sprintf("- 风险类型: %s\n", f.RiskType))
			sb.WriteString(fmt.Sprintf("- 风险等级: %s\n", f.RiskLevel))
			sb.WriteString(fmt.Sprintf("- 风险描述: %s\n", f.RiskDescription))
			sb.WriteString(fmt.Sprintf("- 原文: %s\n", f.OriginalText))
			sb.WriteString(fmt.Sprintf("- 置信度: %.2f\n", f.Confidence))
			if len(f.LegalBasis) > 0 {
				sb.WriteString("- 法律依据:\n")
				for _, lb := range f.LegalBasis {
					sb.WriteString(fmt.Sprintf("  - %s %s: %s\n", lb.Source, lb.Article, lb.Content))
				}
			}
			sb.WriteString("\n")
		}
	}

	if len(unverified) > 0 {
		sb.WriteString("## 待验证的风险点（也需生成建议，但必须标注待人工复核）\n\n")
		for i, f := range unverified {
			sb.WriteString(fmt.Sprintf("%d. [待验证] %s - %s\n", i+1, f.RiskType, f.RiskDescription))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(`## 工作步骤
1. 对每个风险点，调用 rag_search 工具检索该类条款的标准表述或示范文本；待验证风险的建议中必须写明"待人工复核"
2. 结合检索到的标准表述、合同上下文和法律依据，生成具体的修改建议
3. 评估每条修改建议的影响范围

## 输出格式
对每个风险点生成一条修改建议，包含：
- 原文摘录
- 修改后的具体条款文本
- 修改理由
- 法律依据引用
- 修改影响评估
- 优先级（必须修改/建议修改/可选修改）`)

	return sb.String()
}

const suggestionAgentSystemPrompt = `你是一名专业的法律顾问，擅长为合同风险点提供具体、可行的修改建议。

## 工作原则
1. **具体可行**：修改建议必须是可以直接替换原文的具体条款文本，不能是模糊的方向性建议
2. **法律合规**：修改后的条款必须符合相关法律法规
3. **利益平衡**：在保护审查方利益的同时，保持条款的公平性和可执行性
4. **参考规范**：优先使用 RAG 检索到的标准表述和示范文本

## 优先级判断标准
- 必须修改: 涉及合同效力、重大违法风险、核心利益保护
- 建议修改: 权利义务不对等、违约责任不合理、表述有争议
- 可选修改: 表述不够规范、条款可进一步完善`
