package agent

import (
	"context"
	"contract_review/app/internal/global"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

// ReactLoop ReAct 循环引擎
// 参考 https://www.waylandz.com/ai-agent-book/ 第2章 ReAct 循环
// 核心：思考(Reason) → 行动(Act) → 观察(Observe) → 循环
type ReactLoop struct {
	config ReactConfig
}

// NewReactLoop 创建 ReAct 循环引擎
func NewReactLoop(config ReactConfig) *ReactLoop {
	return &ReactLoop{config: config}
}

// ReactState ReAct 循环状态
type ReactState struct {
	Iteration    int
	Observations []string
	Steps        []ThinkStep
	TokensUsed   int
	StartTime    time.Time
	Completed    bool
	FinalResult  interface{}
}

// ToolCallRequest LLM 输出的工具调用请求
type ToolCallRequest struct {
	ToolName string                 `json:"tool_name"`
	Params   map[string]interface{} `json:"params"`
	Thought  string                 `json:"thought"`
}

// ReactDecision LLM 单步决策结果
type ReactDecision struct {
	Thought   string           `json:"thought"`
	Action    string           `json:"action"`    // "tool_call" 或 "finish"
	ToolCall  *ToolCallRequest `json:"tool_call,omitempty"`
	FinalAnswer interface{}    `json:"final_answer,omitempty"`
}

// Run 执行 ReAct 循环
// systemPrompt: Agent 角色描述
// taskPrompt: 任务描述
// tools: 可用工具列表
// llmGenerate: LLM 调用函数
func (rl *ReactLoop) Run(
	ctx context.Context,
	systemPrompt string,
	taskPrompt string,
	tools []Tool,
	llmGenerate func(ctx context.Context, messages []*schema.Message) (*schema.Message, error),
) (*AgentOutput, error) {

	state := &ReactState{
		StartTime: time.Now(),
	}

	toolDescriptions := buildToolDescriptions(tools)
	toolMap := buildToolMap(tools)

	var conversationHistory []*schema.Message

	sysMsg := &schema.Message{
		Role:    schema.System,
		Content: buildReactSystemPrompt(systemPrompt, toolDescriptions),
	}
	conversationHistory = append(conversationHistory, sysMsg)

	userMsg := &schema.Message{
		Role:    schema.User,
		Content: taskPrompt,
	}
	conversationHistory = append(conversationHistory, userMsg)

	for state.Iteration < rl.config.MaxIterations {
		state.Iteration++

		select {
		case <-ctx.Done():
			return buildOutput(state), ctx.Err()
		default:
		}

		global.Log.Info("ReAct 循环",
			zap.Int("iteration", state.Iteration),
			zap.Int("maxIterations", rl.config.MaxIterations))

		response, err := llmGenerate(ctx, conversationHistory)
		if err != nil {
			global.Log.Error("ReAct LLM 调用失败",
				zap.Int("iteration", state.Iteration),
				zap.Error(err))
			if state.Iteration >= rl.config.MinIterations && state.FinalResult != nil {
				return buildOutput(state), nil
			}
			return buildOutput(state), fmt.Errorf("LLM 调用失败(第%d轮): %w", state.Iteration, err)
		}

		decision, err := parseReactDecision(response.Content)
		if err != nil {
			global.Log.Warn("解析 ReAct 决策失败，尝试作为最终答案处理",
				zap.Int("iteration", state.Iteration),
				zap.Error(err))
			if state.Iteration >= rl.config.MinIterations {
				state.FinalResult = response.Content
				state.Completed = true
				state.Steps = append(state.Steps, ThinkStep{
					Iteration: state.Iteration,
					Thought:   "输出最终结果",
					Action:    "finish",
					Observation: response.Content,
					Timestamp: time.Now(),
				})
				return buildOutput(state), nil
			}
			continue
		}

		step := ThinkStep{
			Iteration: state.Iteration,
			Thought:   decision.Thought,
			Action:    decision.Action,
			Timestamp: time.Now(),
		}

		conversationHistory = append(conversationHistory, &schema.Message{
			Role:    schema.Assistant,
			Content: response.Content,
		})

		if decision.Action == "finish" || decision.FinalAnswer != nil {
			if state.Iteration < rl.config.MinIterations {
				global.Log.Warn("未达最小轮数，强制继续",
					zap.Int("iteration", state.Iteration),
					zap.Int("minIterations", rl.config.MinIterations))

				conversationHistory = append(conversationHistory, &schema.Message{
					Role:    schema.User,
					Content: "你还没有使用任何工具获取信息，请先调用工具验证你的判断，不要直接给出答案。",
				})
				continue
			}

			state.FinalResult = decision.FinalAnswer
			state.Completed = true
			step.Observation = "任务完成"
			state.Steps = append(state.Steps, step)

			global.Log.Info("ReAct 循环完成",
				zap.Int("totalIterations", state.Iteration))
			return buildOutput(state), nil
		}

		if decision.Action == "tool_call" && decision.ToolCall != nil {
			step.ActionInput = fmt.Sprintf("%s(%v)", decision.ToolCall.ToolName, decision.ToolCall.Params)

			tool, exists := toolMap[decision.ToolCall.ToolName]
			if !exists {
				observation := fmt.Sprintf("错误：工具 '%s' 不存在。可用工具: %s",
					decision.ToolCall.ToolName, strings.Join(getToolNames(tools), ", "))
				step.Observation = observation
				state.Steps = append(state.Steps, step)
				state.Observations = append(state.Observations, observation)

				conversationHistory = append(conversationHistory, &schema.Message{
					Role:    schema.User,
					Content: fmt.Sprintf("观察结果：%s", observation),
				})
				continue
			}

			toolResult, err := tool.Execute(ctx, decision.ToolCall.Params)
			var observation string
			if err != nil {
				observation = fmt.Sprintf("工具执行出错: %s", err.Error())
			} else if !toolResult.Success {
				observation = fmt.Sprintf("工具执行失败: %s", toolResult.Error)
			} else {
				obsBytes, _ := json.Marshal(toolResult.Data)
				observation = string(obsBytes)
				if len(observation) > 4000 {
					observation = observation[:4000] + "...(内容被截断)"
				}
			}

			step.Observation = observation
			state.Steps = append(state.Steps, step)
			state.Observations = append(state.Observations, observation)

			if len(state.Observations) > rl.config.ObservationWindow {
				state.Observations = state.Observations[len(state.Observations)-rl.config.ObservationWindow:]
			}

			conversationHistory = append(conversationHistory, &schema.Message{
				Role:    schema.User,
				Content: fmt.Sprintf("观察结果：%s", observation),
			})

			if shouldStopEarly(state) {
				global.Log.Info("ReAct 循环提前终止：结果收敛")
				state.Completed = true
				return buildOutput(state), nil
			}
		}
	}

	global.Log.Warn("ReAct 循环达到最大轮数",
		zap.Int("maxIterations", rl.config.MaxIterations))
	state.Completed = true
	return buildOutput(state), nil
}

// shouldStopEarly 检测结果是否收敛（连续观察结果高度相似）
func shouldStopEarly(state *ReactState) bool {
	if len(state.Observations) < 2 {
		return false
	}
	last := state.Observations[len(state.Observations)-1]
	prev := state.Observations[len(state.Observations)-2]
	return areSimilar(last, prev)
}

// areSimilar 简单的相似度检测
func areSimilar(a, b string) bool {
	if a == b {
		return true
	}
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	shorter, longer := a, b
	if len(a) > len(b) {
		shorter, longer = b, a
	}
	if float64(len(shorter))/float64(len(longer)) > 0.9 {
		matchCount := 0
		shorterRunes := []rune(shorter)
		longerRunes := []rune(longer)
		minLen := len(shorterRunes)
		if len(longerRunes) < minLen {
			minLen = len(longerRunes)
		}
		for i := 0; i < minLen; i++ {
			if shorterRunes[i] == longerRunes[i] {
				matchCount++
			}
		}
		return float64(matchCount)/float64(minLen) > 0.85
	}
	return false
}

// parseReactDecision 解析 LLM 输出为 ReAct 决策
func parseReactDecision(content string) (*ReactDecision, error) {
	var decision ReactDecision
	if err := json.Unmarshal([]byte(extractJSON(content)), &decision); err == nil {
		return &decision, nil
	}

	decision = ReactDecision{}

	if idx := strings.Index(content, "思考："); idx != -1 {
		end := strings.Index(content[idx+len("思考："):], "\n")
		if end == -1 {
			decision.Thought = strings.TrimSpace(content[idx+len("思考："):])
		} else {
			decision.Thought = strings.TrimSpace(content[idx+len("思考：") : idx+len("思考：")+end])
		}
	} else if idx := strings.Index(content, "Thought:"); idx != -1 {
		end := strings.Index(content[idx+len("Thought:"):], "\n")
		if end == -1 {
			decision.Thought = strings.TrimSpace(content[idx+len("Thought:"):])
		} else {
			decision.Thought = strings.TrimSpace(content[idx+len("Thought:") : idx+len("Thought:")+end])
		}
	}

	if strings.Contains(content, "最终答案") || strings.Contains(content, "Final Answer") ||
		strings.Contains(content, "任务完成") {
		decision.Action = "finish"
		decision.FinalAnswer = content
		return &decision, nil
	}

	if strings.Contains(content, "调用工具") || strings.Contains(content, "tool_call") ||
		strings.Contains(content, "Action:") || strings.Contains(content, "行动：") {
		decision.Action = "tool_call"

		jsonStr := extractJSON(content)
		if jsonStr != "" {
			var tc ToolCallRequest
			if err := json.Unmarshal([]byte(jsonStr), &tc); err == nil {
				decision.ToolCall = &tc
				return &decision, nil
			}
		}
	}

	if decision.Thought == "" && decision.Action == "" {
		return nil, fmt.Errorf("无法解析 ReAct 决策: %s", truncate(content, 200))
	}

	if decision.Action == "" {
		decision.Action = "finish"
		decision.FinalAnswer = content
	}

	return &decision, nil
}

// extractJSON 从文本中提取 JSON 块
func extractJSON(text string) string {
	start := strings.Index(text, "{")
	if start == -1 {
		return ""
	}
	depth := 0
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return text[start : i+1]
			}
		}
	}
	return ""
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// buildReactSystemPrompt 构建 ReAct 系统提示词
func buildReactSystemPrompt(rolePrompt string, toolDescriptions string) string {
	return fmt.Sprintf(`%s

## 工作模式
你使用 ReAct（Reasoning + Acting）模式工作：每一步先思考，再决定行动，观察结果后继续思考。

## 可用工具
%s

## 输出格式要求
每一步请严格按以下 JSON 格式输出：

当需要调用工具时：
{
  "thought": "你的思考过程",
  "action": "tool_call",
  "tool_call": {
    "tool_name": "工具名称",
    "params": {"参数名": "参数值"}
  }
}

当任务完成时：
{
  "thought": "总结思考",
  "action": "finish",
  "final_answer": "最终结果（可以是字符串或JSON对象）"
}

## 重要规则
1. 必须先使用工具获取信息，不要凭空回答
2. 每次只调用一个工具
3. 仔细阅读观察结果后再决定下一步
4. 如果工具执行失败，换个方式重试
5. 任务完成后明确输出 "finish"`, rolePrompt, toolDescriptions)
}

// buildToolDescriptions 构建工具描述
func buildToolDescriptions(tools []Tool) string {
	if len(tools) == 0 {
		return "无可用工具"
	}
	var sb strings.Builder
	for i, t := range tools {
		sb.WriteString(fmt.Sprintf("%d. **%s**: %s\n", i+1, t.Name(), t.Description()))
		params := t.Parameters()
		if len(params) > 0 {
			paramsJSON, _ := json.MarshalIndent(params, "   ", "  ")
			sb.WriteString(fmt.Sprintf("   参数: %s\n", string(paramsJSON)))
		}
	}
	return sb.String()
}

func buildToolMap(tools []Tool) map[string]Tool {
	m := make(map[string]Tool, len(tools))
	for _, t := range tools {
		m[t.Name()] = t
	}
	return m
}

func getToolNames(tools []Tool) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name()
	}
	return names
}

func buildOutput(state *ReactState) *AgentOutput {
	return &AgentOutput{
		Result:     state.FinalResult,
		Thinking:   state.Steps,
		TokensUsed: state.TokensUsed,
		Duration:   time.Since(state.StartTime),
	}
}

// RunSimple 简化版 ReAct — 无工具调用，直接 LLM 推理
func RunSimple(
	ctx context.Context,
	systemPrompt string,
	userPrompt string,
	llmGenerate func(ctx context.Context, messages []*schema.Message) (*schema.Message, error),
) (string, error) {
	template := prompt.FromMessages(schema.GoTemplate,
		&schema.Message{Role: schema.System, Content: systemPrompt},
		&schema.Message{Role: schema.User, Content: userPrompt},
	)
	messages, err := template.Format(ctx, map[string]any{})
	if err != nil {
		return "", fmt.Errorf("格式化提示词失败: %w", err)
	}
	response, err := llmGenerate(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("LLM 调用失败: %w", err)
	}
	return response.Content, nil
}
