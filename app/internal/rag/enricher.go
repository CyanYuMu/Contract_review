package rag

import (
	"fmt"
	"strings"
)

// buildEnrichedPassage 构建增强 Passage（用于 Rerank 模型输入）
//
// 设计理念（参考 WeKnora chat_pipeline/rerank.go:664-713）:
// Rerank 模型需要的输入不止是 chunk 原文，还需要携带元数据上下文
// 以帮助 Cross-Encoder 更准确地判断相关性
//
// 输出格式:
//
//	[标题]
//	[来源: 分类]
//	正文内容
func buildEnrichedPassage(result SearchResult) string {
	var sb strings.Builder

	// 标题（如果有）
	title := ""
	if result.Metadata != nil {
		if t, ok := result.Metadata["title"]; ok && strings.TrimSpace(t) != "" {
			title = strings.TrimSpace(t)
		}
	}
	if title == "" && result.Source != "" {
		title = result.Source
	}

	if title != "" {
		sb.WriteString(fmt.Sprintf("标题: %s\n", title))
	}

	// 分类信息（帮 Rerank 模型理解领域上下文）
	category := ""
	if result.Metadata != nil {
		if cat, ok := result.Metadata["category"]; ok && strings.TrimSpace(cat) != "" {
			category = strings.TrimSpace(cat)
		}
	}
	subCategory := ""
	if result.Metadata != nil {
		if sub, ok := result.Metadata["sub_category"]; ok && strings.TrimSpace(sub) != "" {
			subCategory = strings.TrimSpace(sub)
		}
	}

	if category != "" || subCategory != "" {
		parts := make([]string, 0, 2)
		if category != "" {
			parts = append(parts, category)
		}
		if subCategory != "" {
			parts = append(parts, subCategory)
		}
		sb.WriteString(fmt.Sprintf("分类: %s\n", strings.Join(parts, " · ")))
	}

	// 分隔
	if sb.Len() > 0 {
		sb.WriteString("\n")
	}

	// 正文内容
	sb.WriteString(strings.TrimSpace(result.Content))

	return sb.String()
}

// buildCandidateQuery 构建检索查询文本（增强版）
//
// 从条款 + 合同元信息中提取最有效的检索信号:
//  1. 合同类型 → 精确匹配 sub_category
//  2. 条款类别 → 缩小检索范围
//  3. 条款标题 → 关键词信号
//  4. 条款关键词 → 提取关键法律术语
func buildEnhancedCandidateQuery(clauseContent string, clauseCategory string, clauseTitle string, contractType string, stance string) string {
	parts := make([]string, 0, 5)

	// 合同类型作为首要信号
	if strings.TrimSpace(contractType) != "" {
		parts = append(parts, "合同类型: "+strings.TrimSpace(contractType))
	}

	// 审查立场
	if strings.TrimSpace(stance) != "" {
		parts = append(parts, "审查立场: "+strings.TrimSpace(stance))
	}

	// 条款类别
	if strings.TrimSpace(clauseCategory) != "" && clauseCategory != "其他" {
		parts = append(parts, "条款类别: "+strings.TrimSpace(clauseCategory))
	}

	// 条款标题
	if strings.TrimSpace(clauseTitle) != "" {
		parts = append(parts, "条款: "+strings.TrimSpace(clauseTitle))
	}

	// 提取关键法律术语（从条款内容中）
	keywords := extractLegalKeywords(clauseContent)
	if len(keywords) > 0 {
		parts = append(parts, "关键术语: "+strings.Join(keywords, " "))
	}

	// 截断过长的内容部分
	content := truncateRunes(clauseContent, 800)

	return strings.Join(append(parts, content), "\n")
}

// extractLegalKeywords 从条款文本中提取法律相关关键词
func extractLegalKeywords(text string) []string {
	legalPatterns := []string{
		"违约责任", "赔偿", "损失", "违约金", "解除",
		"终止", "知识产权", "保密", "争议", "仲裁",
		"诉讼", "管辖", "付款", "支付", "验收",
		"交付", "质量", "期限", "发票", "逾期",
		"甲方", "乙方", "单方", "免责", "不可抗力",
		"合同变更", "转让", "分包", "连带责任", "保证",
		"抵押", "留置", "违约金", "滞纳金",
	}

	textLower := strings.ToLower(text)
	var found []string
	seen := make(map[string]bool)

	for _, kw := range legalPatterns {
		if strings.Contains(textLower, strings.ToLower(kw)) {
			if !seen[kw] {
				seen[kw] = true
				found = append(found, kw)
			}
		}
		if len(found) >= 8 {
			break
		}
	}

	return found
}

// truncateRunes 按 rune 截断文本
func truncateRunes(text string, maxRunes int) string {
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes])
}
