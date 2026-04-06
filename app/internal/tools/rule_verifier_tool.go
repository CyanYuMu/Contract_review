package tools

import (
	"context"
	"contract_review/app/internal/agent"
	"contract_review/app/internal/global"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

// RuleVerifierTool 规则验证工具 — 验证 Agent 识别的风险是否在审阅规范中有据可依
type RuleVerifierTool struct {
	llmGenerate func(ctx context.Context, messages []*schema.Message) (*schema.Message, error)
}

// NewRuleVerifierTool 创建规则验证工具
func NewRuleVerifierTool(
	llmGenerate func(ctx context.Context, messages []*schema.Message) (*schema.Message, error),
) *RuleVerifierTool {
	return &RuleVerifierTool{llmGenerate: llmGenerate}
}

func (t *RuleVerifierTool) Name() string {
	return "rule_verifier"
}

func (t *RuleVerifierTool) Description() string {
	return "验证已识别的风险点是否在审阅规范或法律条例中有明确依据。" +
		"输入合同条款内容、初步识别的风险描述、以及 RAG 检索到的相关规范，" +
		"输出结构化的验证结果，包括是否验证通过、置信度、匹配的具体规则。" +
		"只有经过此工具验证的风险才会进入最终审阅报告。"
}

func (t *RuleVerifierTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"clause_text": map[string]interface{}{
				"type":        "string",
				"description": "待验证的合同条款原文",
			},
			"identified_risk": map[string]interface{}{
				"type":        "string",
				"description": "初步识别的风险描述",
			},
			"retrieved_rules": map[string]interface{}{
				"type":        "string",
				"description": "RAG 检索到的相关审阅规范内容（多条规范用换行分隔）",
			},
			"contract_type": map[string]interface{}{
				"type":        "string",
				"description": "合同类型",
			},
			"stance": map[string]interface{}{
				"type":        "string",
				"description": "审查立场（甲方/乙方）",
			},
		},
		"required": []string{"clause_text", "identified_risk", "retrieved_rules"},
	}
}

// VerificationOutput 验证输出结构
type VerificationOutput struct {
	IsVerified   bool          `json:"is_verified"`
	Confidence   float64       `json:"confidence"`
	RiskLevel    string        `json:"risk_level"`
	MatchedRules []MatchedRule `json:"matched_rules"`
	Reasoning    string        `json:"reasoning"`
}

// MatchedRule 匹配到的规则
type MatchedRule struct {
	RuleSource  string  `json:"rule_source"`
	RuleArticle string  `json:"rule_article"`
	RuleContent string  `json:"rule_content"`
	MatchScore  float64 `json:"match_score"`
	Explanation string  `json:"explanation"`
}

func (t *RuleVerifierTool) Execute(ctx context.Context, params map[string]interface{}) (agent.ToolResult, error) {
	clauseText, _ := params["clause_text"].(string)
	identifiedRisk, _ := params["identified_risk"].(string)
	retrievedRules, _ := params["retrieved_rules"].(string)
	contractType, _ := params["contract_type"].(string)
	stance, _ := params["stance"].(string)

	if clauseText == "" || identifiedRisk == "" {
		return agent.ToolResult{Success: false, Error: "条款内容和风险描述不能为空"}, nil
	}

	if retrievedRules == "" {
		return agent.ToolResult{
			Success: true,
			Data: VerificationOutput{
				IsVerified: false,
				Confidence: 0.1,
				Reasoning:  "无相关审阅规范可供验证，风险点缺乏法规依据",
			},
		}, nil
	}

	verifyPrompt := buildVerificationPrompt(clauseText, identifiedRisk, retrievedRules, contractType, stance)

	template := prompt.FromMessages(schema.GoTemplate,
		&schema.Message{
			Role:    schema.System,
			Content: ruleVerifierSystemPrompt,
		},
		&schema.Message{
			Role:    schema.User,
			Content: verifyPrompt,
		},
	)

	messages, err := template.Format(ctx, map[string]any{})
	if err != nil {
		return agent.ToolResult{Success: false, Error: "格式化验证提示词失败"}, nil
	}

	response, err := t.llmGenerate(ctx, messages)
	if err != nil {
		global.Log.Error("规则验证 LLM 调用失败", zap.Error(err))
		return agent.ToolResult{Success: false, Error: "验证服务调用失败"}, nil
	}

	output := parseVerificationOutput(response.Content)
	output = applyVerificationGuardrails(output, retrievedRules)

	return agent.ToolResult{
		Success: true,
		Data:    output,
	}, nil
}

func buildVerificationPrompt(clauseText, risk, rules, contractType, stance string) string {
	stanceInfo := ""
	if stance != "" {
		stanceInfo = fmt.Sprintf("审查立场：%s\n", stance)
	}
	typeInfo := ""
	if contractType != "" {
		typeInfo = fmt.Sprintf("合同类型：%s\n", contractType)
	}

	return fmt.Sprintf(`请验证以下合同条款中识别的风险是否在审阅规范中有明确依据。

## 合同条款原文
%s

## 初步识别的风险
%s

## 检索到的相关审阅规范
%s

%s%s

## 验证要求
1. 逐条对比合同条款与审阅规范，判断风险是否确实存在
2. 给出匹配的具体规范条目和匹配度
3. 如果风险在规范中无明确依据，如实标注为未验证
4. 给出综合置信度评分（0.0-1.0）

请以 JSON 格式输出验证结果：
{
  "is_verified": true/false,
  "confidence": 0.0-1.0,
  "risk_level": "高/中/低",
  "matched_rules": [
    {
      "rule_source": "规范来源",
      "rule_article": "条款编号",
      "rule_content": "规范内容摘要",
      "match_score": 0.0-1.0,
      "explanation": "匹配解释"
    }
  ],
  "reasoning": "验证推理过程"
}`, clauseText, risk, rules, stanceInfo, typeInfo)
}

func parseVerificationOutput(content string) VerificationOutput {
	var output VerificationOutput

	jsonStr := extractJSONFromText(content)
	if jsonStr != "" {
		if err := json.Unmarshal([]byte(jsonStr), &output); err == nil {
			return output
		}
	}

	return VerificationOutput{
		IsVerified: false,
		Confidence: 0.3,
		Reasoning:  "验证结果解析失败: " + truncateText(content, 200),
	}
}

// applyVerificationGuardrails 确定性护栏
func applyVerificationGuardrails(output VerificationOutput, retrievedRules string) VerificationOutput {
	// 护栏 1: 没有匹配规则但声称已验证 → 不可信
	if len(output.MatchedRules) == 0 && output.IsVerified {
		output.IsVerified = false
		output.Confidence = 0.2
		output.Reasoning += " [护栏修正: 无匹配规则，验证结果不可信]"
	}

	// 护栏 2: 匹配度最高的规则低于 0.4 → 降级
	if len(output.MatchedRules) > 0 {
		maxScore := 0.0
		for _, rule := range output.MatchedRules {
			if rule.MatchScore > maxScore {
				maxScore = rule.MatchScore
			}
		}
		if maxScore < 0.4 {
			output.Confidence *= 0.5
			if output.Confidence < 0.3 {
				output.IsVerified = false
			}
		}
	}

	// 护栏 3: 置信度上限不超过最高匹配度
	if len(output.MatchedRules) > 0 {
		maxScore := 0.0
		for _, rule := range output.MatchedRules {
			if rule.MatchScore > maxScore {
				maxScore = rule.MatchScore
			}
		}
		if output.Confidence > maxScore+0.1 {
			output.Confidence = maxScore + 0.1
		}
	}

	return output
}

func extractJSONFromText(text string) string {
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

func truncateText(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

const ruleVerifierSystemPrompt = `你是一个法律规范验证专家。你的任务是严格验证合同条款中的风险点是否在审阅规范和法律条例中有明确依据。

## 验证原则
1. 必须基于提供的审阅规范进行验证，不得凭空编造法规依据
2. 对每条匹配的规范给出客观的匹配度评分
3. 如果审阅规范中确实没有明确涉及该风险，如实标注为"未验证"
4. 区分"直接依据"和"间接推断"，给出不同的置信度

## 评分标准
- 0.9-1.0: 审阅规范中有直接、明确的对应条款
- 0.7-0.8: 审阅规范中有相关条款，需要一定推理
- 0.5-0.6: 审阅规范中有间接相关的内容
- 0.3-0.4: 审阅规范中有模糊的相关性
- 0.0-0.2: 审阅规范中未找到相关依据`
