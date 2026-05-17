package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"contract_review/app/internal/global"

	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

// RiskAgent 风险识别与验证 Agent
// 核心创新：使用 ReAct 循环 + RAG 检索 + RuleVerifier 验证
// 确保每个风险发现都有审阅规范依据
type RiskAgent struct {
	llmGenerate func(ctx context.Context, messages []*schema.Message) (*schema.Message, error)
	tools       []Tool
	reactConfig ReactConfig
}

// NewRiskAgent 创建风险识别 Agent
func NewRiskAgent(
	llmGenerate func(ctx context.Context, messages []*schema.Message) (*schema.Message, error),
	tools []Tool,
) *RiskAgent {
	return &RiskAgent{
		llmGenerate: llmGenerate,
		tools:       tools,
		reactConfig: ReactConfig{
			MaxIterations:     5,
			MinIterations:     2, // 至少调用 RAG + RuleVerifier 各一次
			ObservationWindow: 5,
		},
	}
}

func (ra *RiskAgent) Name() string { return "RiskAgent" }

func (ra *RiskAgent) AvailableTools() []Tool { return ra.tools }

// Execute 执行风险识别
// input.Context 应包含:
//   - "clause": Clause 对象
//   - "contract_meta": ContractMeta 对象
//   - "stance": 审查立场
//   - "intensity": 审查强度
func (ra *RiskAgent) Execute(ctx context.Context, input AgentInput) (AgentOutput, error) {
	clause, _ := input.Context["clause"].(Clause)
	meta, _ := input.Context["contract_meta"].(ContractMeta)

	if clause.Content == "" {
		return AgentOutput{}, fmt.Errorf("条款内容不能为空")
	}

	global.Log.Info("RiskAgent 开始审阅条款",
		zap.String("clauseID", clause.ID),
		zap.String("clauseTitle", clause.Title),
		zap.String("stance", meta.Stance))

	taskPrompt := buildRiskIdentificationPrompt(clause, meta)

	reactLoop := NewReactLoop(ra.reactConfig)
	output, err := reactLoop.Run(
		ctx,
		riskAgentSystemPrompt,
		taskPrompt,
		ra.tools,
		ra.llmGenerate,
	)

	if err != nil {
		global.Log.Error("RiskAgent 执行失败",
			zap.String("clauseID", clause.ID),
			zap.Error(err))
		return AgentOutput{}, err
	}

	findings := ra.extractFindings(output, clause.ID)

	global.Log.Info("RiskAgent 审阅完成",
		zap.String("clauseID", clause.ID),
		zap.Int("findingsCount", len(findings)),
		zap.Int("verifiedCount", countVerified(findings)))

	return AgentOutput{
		Result:     findings,
		Thinking:   output.Thinking,
		TokensUsed: output.TokensUsed,
		Duration:   output.Duration,
	}, nil
}

// extractFindings 从 Agent 输出中提取风险发现
func (ra *RiskAgent) extractFindings(output *AgentOutput, clauseID string) []RiskFinding {
	if output.Result == nil {
		return nil
	}

	var resultStr string
	if str, ok := output.Result.(string); ok {
		resultStr = str
	} else if output.Result != nil {
		if bytes, err := json.Marshal(output.Result); err == nil {
			resultStr = string(bytes)
		}
	}

	var findings []RiskFinding

	if resultStr != "" {
		findings = parseRiskFindingsFromJSON(resultStr, clauseID)
		if len(findings) == 0 {
			findings = parseRiskFindingsFromText(resultStr, clauseID)
		}
	}

	if len(findings) == 0 {
		for _, step := range output.Thinking {
			if step.Action == "tool_call" && strings.Contains(step.ActionInput, "rule_verifier") {
				if strings.Contains(step.Observation, "\"is_verified\":true") ||
					strings.Contains(step.Observation, "\"is_verified\": true") {
					findings = append(findings, RiskFinding{
						ClauseID:        clauseID,
						RiskLevel:       "中",
						RiskDescription: "规则验证确认存在风险，请结合审阅记录复核。",
						Verified:        true,
					})
				}
			}
		}
	}

	return findings
}

func parseRiskFindingsFromJSON(text string, clauseID string) []RiskFinding {
	jsonStr := extractJSONArray(text)
	if jsonStr == "" {
		jsonStr = extractJSON(text)
	}
	if jsonStr == "" {
		return nil
	}

	type rawFinding struct {
		ClauseID        string       `json:"clause_id"`
		RiskType        string       `json:"risk_type"`
		RiskLevel       string       `json:"risk_level"`
		RiskDescription string       `json:"risk_description"`
		Description     string       `json:"description"`
		OriginalText    string       `json:"original_text"`
		LegalBasis      []LegalBasis `json:"legal_basis"`
		Verified        bool         `json:"verified"`
		Confidence      float64      `json:"confidence"`
	}

	var arr []rawFinding
	if strings.HasPrefix(strings.TrimSpace(jsonStr), "{") {
		var wrapper struct {
			Findings []rawFinding `json:"findings"`
			Risks    []rawFinding `json:"risks"`
		}
		if err := json.Unmarshal([]byte(jsonStr), &wrapper); err != nil {
			return nil
		}
		if len(wrapper.Findings) > 0 {
			arr = wrapper.Findings
		} else {
			arr = wrapper.Risks
		}
	} else if err := json.Unmarshal([]byte(jsonStr), &arr); err != nil {
		return nil
	}

	findings := make([]RiskFinding, 0, len(arr))
	for _, item := range arr {
		desc := item.RiskDescription
		if desc == "" {
			desc = item.Description
		}
		if strings.TrimSpace(desc) == "" && strings.TrimSpace(item.OriginalText) == "" {
			continue
		}
		id := item.ClauseID
		if id == "" {
			id = clauseID
		}
		findings = append(findings, RiskFinding{
			ClauseID:        id,
			RiskType:        item.RiskType,
			RiskLevel:       normalizeRiskLevel(item.RiskLevel),
			RiskDescription: desc,
			OriginalText:    item.OriginalText,
			LegalBasis:      item.LegalBasis,
			Verified:        item.Verified,
			Confidence:      item.Confidence,
		})
	}
	return findings
}

// parseRiskFindingsFromText 从文本中解析风险发现
func parseRiskFindingsFromText(text string, clauseID string) []RiskFinding {
	var findings []RiskFinding

	sections := strings.Split(text, "风险点")
	for i, section := range sections {
		if i == 0 || strings.TrimSpace(section) == "" {
			continue
		}

		finding := RiskFinding{
			ClauseID: clauseID,
		}

		if strings.Contains(section, "高") {
			finding.RiskLevel = "高"
		} else if strings.Contains(section, "中") {
			finding.RiskLevel = "中"
		} else {
			finding.RiskLevel = "低"
		}

		lines := strings.Split(section, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "类型") || strings.Contains(line, "风险类型") {
				finding.RiskType = extractValue(line)
			} else if strings.Contains(line, "描述") || strings.Contains(line, "分析") {
				finding.RiskDescription = extractValue(line)
			} else if strings.Contains(line, "原文") {
				finding.OriginalText = extractValue(line)
			}
		}

		if finding.RiskDescription != "" || finding.OriginalText != "" {
			findings = append(findings, finding)
		}
	}

	return findings
}

func normalizeRiskLevel(level string) string {
	if strings.Contains(level, "高") {
		return "高"
	}
	if strings.Contains(level, "中") {
		return "中"
	}
	if strings.Contains(level, "低") {
		return "低"
	}
	return "中"
}

func extractValue(line string) string {
	for _, sep := range []string{"：", ":", "】"} {
		if idx := strings.LastIndex(line, sep); idx != -1 {
			return strings.TrimSpace(line[idx+len(sep):])
		}
	}
	return strings.TrimSpace(line)
}

func countVerified(findings []RiskFinding) int {
	count := 0
	for _, f := range findings {
		if f.Verified {
			count++
		}
	}
	return count
}

func buildRiskIdentificationPrompt(clause Clause, meta ContractMeta) string {
	intensityDesc := map[string]string{
		"严格": "请进行严格审阅，覆盖全部审查维度，识别所有潜在风险。",
		"标准": "请进行标准审阅，重点关注核心风险领域。",
		"宽松": "请进行宽松审阅，仅指出重大法律风险。",
	}

	desc := intensityDesc[meta.Intensity]
	if desc == "" {
		desc = intensityDesc["标准"]
	}

	return fmt.Sprintf(`## 审阅任务
请审阅以下合同条款，识别对 %s 不利的风险点。

## 合同信息
- 甲方: %s
- 乙方: %s
- 合同类型: %s
- 审查立场: %s
- 审查强度: %s

## 条款信息
- 条款ID: %s
- 条款标题: %s
- 条款分类: %s

## 条款内容
%s

## 工作步骤（必须按顺序执行）
1. 仔细阅读条款内容，初步识别可能的风险点
2. 对每个风险点，调用 rag_search 工具检索相关审阅规范、法律条例和已配置风险点；调用时必须传入 contract_type="%s"
3. 将条款内容、风险描述和检索到的规范一起提交给 rule_verifier 工具验证
4. 输出已验证风险；如果知识库未命中但条款存在明显法律或履约风险，可以输出 verified=false 的"待人工确认风险"

%s

## 输出要求
请在完成工具检索和验证后，以 JSON 输出，格式如下：
{
  "findings": [
    {
      "clause_id": "%s",
      "risk_type": "风险类型（15字以内）",
      "risk_level": "高/中/低",
      "risk_description": "风险描述",
      "original_text": "原文摘录（100字以内）",
      "legal_basis": [{"source":"依据来源","article":"条款编号","content":"依据摘要","relevance":0.8}],
      "verified": true,
      "confidence": 0.0
    }
  ]
}
如无风险，输出 {"findings": []}。待人工确认风险必须写明为什么知识库未命中仍需人工复核。`,
		meta.Stance, meta.PartyA, meta.PartyB, meta.ContractType,
		meta.Stance, meta.Intensity,
		clause.ID, clause.Title, clause.Category,
		clause.Content, meta.ContractType, desc, clause.ID)
}

const riskAgentSystemPrompt = `你是一名资深合同审查律师，擅长识别合同条款中的法律风险。

## 工作原则
1. **有据可依**：你识别的每一个风险点，都必须通过 RAG 检索在审阅规范或法律条例中找到依据
2. **先检索后验证**：先用 rag_search 工具检索相关规范，再用 rule_verifier 工具验证风险
3. **客观专业**：基于法律专业知识和审阅规范进行分析，不做主观臆断
4. **分清轻重**：高风险（影响合同效力/重大利益损失）、中风险（可能产生争议）、低风险（表述不规范）

## 审阅维度
- 合同主体资格与权利能力
- 权利义务对等性
- 违约责任合理性
- 付款条件与时间约定
- 交付与验收标准
- 保密与知识产权
- 争议解决机制
- 合同解除与终止条件
- 不可抗力与免责条款
- 法律适用与管辖

## 重要
- 未经工具验证但确有必要提示的风险标记为"待人工确认"，verified=false
- 如果 RAG 检索无结果，如实说明"未找到对应审阅规范"
- 不要编造法律法规条文`

// ExecuteBatch 批量审阅多个条款（并发）
func (ra *RiskAgent) ExecuteBatch(ctx context.Context, clauses []Clause, meta ContractMeta, maxConcurrent int) ([]RiskFinding, []ThinkStep, error) {
	return ra.ExecuteBatchWithCallback(ctx, clauses, meta, maxConcurrent, nil)
}

// ExecuteBatchWithCallback 批量审阅多个条款，并在单个条款完成时回调局部结果。
func (ra *RiskAgent) ExecuteBatchWithCallback(
	ctx context.Context,
	clauses []Clause,
	meta ContractMeta,
	maxConcurrent int,
	onClauseResult func(index int, clause Clause, findings []RiskFinding, completed int, total int),
) ([]RiskFinding, []ThinkStep, error) {
	if maxConcurrent <= 0 {
		maxConcurrent = 3
	}

	type clauseResult struct {
		findings []RiskFinding
		steps    []ThinkStep
		err      error
		index    int
	}

	resultChan := make(chan clauseResult, len(clauses))
	semaphore := make(chan struct{}, maxConcurrent)

	for i, clause := range clauses {
		go func(idx int, c Clause) {
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			input := AgentInput{
				Task: fmt.Sprintf("审阅条款: %s", c.Title),
				Context: map[string]interface{}{
					"clause":        c,
					"contract_meta": meta,
				},
			}

			output, err := ra.Execute(ctx, input)
			findings, _ := output.Result.([]RiskFinding)

			resultChan <- clauseResult{
				findings: findings,
				steps:    output.Thinking,
				err:      err,
				index:    idx,
			}
		}(i, clause)
	}

	var allFindings []RiskFinding
	var allSteps []ThinkStep

	for i := 0; i < len(clauses); i++ {
		result := <-resultChan
		if result.err != nil {
			global.Log.Error("条款审阅失败",
				zap.Int("index", result.index),
				zap.Error(result.err))
			continue
		}
		completed := i + 1
		if onClauseResult != nil {
			onClauseResult(result.index, clauses[result.index], result.findings, completed, len(clauses))
		}
		allFindings = append(allFindings, result.findings...)
		allSteps = append(allSteps, result.steps...)
	}

	return allFindings, allSteps, nil
}
