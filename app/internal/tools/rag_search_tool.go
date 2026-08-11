package tools

import (
	"context"
	"contract_review/app/internal/agent"
	"contract_review/app/internal/rag"
	"fmt"
	"strings"
)

// RAGSearchTool RAG 检索工具 — 供 Agent 调用检索审阅规范和法律条例
type RAGSearchTool struct {
	retriever *rag.RAGRetriever
}

// NewRAGSearchTool 创建 RAG 检索工具
func NewRAGSearchTool(retriever *rag.RAGRetriever) *RAGSearchTool {
	return &RAGSearchTool{retriever: retriever}
}

func (t *RAGSearchTool) Name() string {
	return "rag_search"
}

func (t *RAGSearchTool) Description() string {
	return "检索审阅规范和法律条例知识库。输入查询关键词，返回相关的审阅标准、法律法规条文、风险案例和示范文本。" +
		"当你需要验证某个风险点是否有法律依据、查找某类条款的审阅规范、寻找标准表述、或匹配后台配置的合同类型风险点时，请使用此工具。" +
		"审阅特定合同时应传入 contract_type，以优先检索该合同类型的风险点配置。"
}

func (t *RAGSearchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "检索查询内容，如'单方解约条款审阅规范'、'违约金比例标准'",
			},
			"contract_type": map[string]interface{}{
				"type":        "string",
				"description": "合同类型过滤，如'买卖合同'、'服务合同'、'劳动合同'、'租赁合同'、'借款合同'、'合作合同'、'知识产权合同'、'通用'",
			},
			"category": map[string]interface{}{
				"type":        "string",
				"description": "知识分类过滤: 规范/法规/案例/示范",
			},
		},
		"required": []string{"query"},
	}
}

func (t *RAGSearchTool) Execute(ctx context.Context, params map[string]interface{}) (agent.ToolResult, error) {
	query, _ := params["query"].(string)
	if query == "" {
		return agent.ToolResult{Success: false, Error: "查询内容不能为空"}, nil
	}

	contractType, _ := params["contract_type"].(string)
	category, _ := params["category"].(string)

	filters := make(map[string]string)
	if contractType != "" {
		filters["sub_category"] = contractType
	}
	if category != "" {
		filters["category"] = category
	}

	var results []rag.SearchResult
	var err error

	if contractType != "" {
		results, err = t.retriever.LayeredRetrieve(query, contractType)
	} else {
		results, err = t.retriever.Retrieve(query, filters)
	}

	if err != nil {
		return agent.ToolResult{Success: false, Error: fmt.Sprintf("检索失败: %s", err.Error())}, nil
	}

	if len(results) == 0 {
		return agent.ToolResult{
			Success: true,
			Data:    "未找到相关审阅规范或法律条例。",
		}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("共检索到 %d 条相关内容：\n\n", len(results)))
	for i, r := range results {
		source := r.Source
		if source == "" {
			source = r.Metadata["title"]
		}
		sb.WriteString(fmt.Sprintf("【结果%d】(来源: %s, 相关度: %.2f)\n%s\n\n",
			i+1, source, r.Score, r.Content))
	}

	return agent.ToolResult{
		Success: true,
		Data:    sb.String(),
	}, nil
}
