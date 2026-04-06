package tools

import (
	"context"
	"contract_review/app/internal/agent"
	"fmt"
	"strings"
)

// ContractContextTool 合同上下文工具 — 让 Agent 能查询合同的元信息和完整条款
type ContractContextTool struct {
	meta    agent.ContractMeta
	clauses []agent.Clause
	fullText string
}

// NewContractContextTool 创建合同上下文工具
func NewContractContextTool(meta agent.ContractMeta, fullText string) *ContractContextTool {
	return &ContractContextTool{
		meta:     meta,
		fullText: fullText,
	}
}

// SetClauses 设置已拆分的条款（由 ClauseAgent 拆分后注入）
func (t *ContractContextTool) SetClauses(clauses []agent.Clause) {
	t.clauses = clauses
}

func (t *ContractContextTool) Name() string {
	return "contract_context"
}

func (t *ContractContextTool) Description() string {
	return "查询当前审阅合同的上下文信息。可查询合同元信息（甲乙方、合同类型、审查立场等）、" +
		"获取指定条款的完整内容、或查找合同中包含特定关键词的条款。"
}

func (t *ContractContextTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "查询类型: 'meta'查询合同元信息, 'clause'查询指定条款, 'search'搜索关键词",
				"enum":        []string{"meta", "clause", "search"},
			},
			"clause_id": map[string]interface{}{
				"type":        "string",
				"description": "条款ID（action=clause时使用）",
			},
			"keyword": map[string]interface{}{
				"type":        "string",
				"description": "搜索关键词（action=search时使用）",
			},
		},
		"required": []string{"action"},
	}
}

func (t *ContractContextTool) Execute(ctx context.Context, params map[string]interface{}) (agent.ToolResult, error) {
	action, _ := params["action"].(string)

	switch action {
	case "meta":
		return agent.ToolResult{
			Success: true,
			Data: map[string]string{
				"party_a":       t.meta.PartyA,
				"party_b":       t.meta.PartyB,
				"contract_type": t.meta.ContractType,
				"stance":        t.meta.Stance,
				"intensity":     t.meta.Intensity,
				"amount":        t.meta.Amount,
			},
		}, nil

	case "clause":
		clauseID, _ := params["clause_id"].(string)
		if clauseID == "" {
			return t.listClauses()
		}
		return t.getClause(clauseID)

	case "search":
		keyword, _ := params["keyword"].(string)
		if keyword == "" {
			return agent.ToolResult{Success: false, Error: "搜索关键词不能为空"}, nil
		}
		return t.searchClauses(keyword)

	default:
		return agent.ToolResult{Success: false, Error: fmt.Sprintf("未知查询类型: %s", action)}, nil
	}
}

func (t *ContractContextTool) listClauses() (agent.ToolResult, error) {
	if len(t.clauses) == 0 {
		return agent.ToolResult{
			Success: true,
			Data:    "合同条款尚未拆分",
		}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("合同共包含 %d 个条款：\n", len(t.clauses)))
	for _, c := range t.clauses {
		sb.WriteString(fmt.Sprintf("- [%s] %s (分类: %s)\n", c.ID, c.Title, c.Category))
	}

	return agent.ToolResult{
		Success: true,
		Data:    sb.String(),
	}, nil
}

func (t *ContractContextTool) getClause(clauseID string) (agent.ToolResult, error) {
	for _, c := range t.clauses {
		if c.ID == clauseID {
			return agent.ToolResult{
				Success: true,
				Data:    c,
			}, nil
		}
	}
	return agent.ToolResult{Success: false, Error: fmt.Sprintf("未找到条款: %s", clauseID)}, nil
}

func (t *ContractContextTool) searchClauses(keyword string) (agent.ToolResult, error) {
	var matched []agent.Clause
	keywordLower := strings.ToLower(keyword)

	for _, c := range t.clauses {
		contentLower := strings.ToLower(c.Content + c.Title)
		if strings.Contains(contentLower, keywordLower) {
			matched = append(matched, c)
		}
	}

	if len(matched) == 0 {
		if strings.Contains(strings.ToLower(t.fullText), keywordLower) {
			return agent.ToolResult{
				Success: true,
				Data:    fmt.Sprintf("在合同全文中找到关键词'%s'，但尚未拆分为具体条款。", keyword),
			}, nil
		}
		return agent.ToolResult{
			Success: true,
			Data:    fmt.Sprintf("未在合同中找到包含'%s'的条款。", keyword),
		}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("找到 %d 个包含'%s'的条款：\n\n", len(matched), keyword))
	for _, c := range matched {
		content := c.Content
		if len([]rune(content)) > 200 {
			content = string([]rune(content)[:200]) + "..."
		}
		sb.WriteString(fmt.Sprintf("[%s] %s\n%s\n\n", c.ID, c.Title, content))
	}

	return agent.ToolResult{
		Success: true,
		Data:    sb.String(),
	}, nil
}
