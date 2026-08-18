package agent

import (
	"sort"
	"strings"
)

// MergeFindings 跨条款去重合并：同一风险在多条款命中时归并为一条，保留所有 ClauseIDs。
// 合并键优先级：候选风险点 ID 集合（同一知识库风险点）> 风险类型 + 首要法律依据（无候选的待人工确认风险）。
// 返回顺序与输入首次出现顺序一致（主条为最早条款）。
func MergeFindings(findings []RiskFinding) []RiskFinding {
	if len(findings) <= 1 {
		return findings
	}

	order := make([]string, 0, len(findings))
	groups := make(map[string][]RiskFinding)
	for _, f := range findings {
		key := findingMergeKey(f)
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], f)
	}

	out := make([]RiskFinding, 0, len(order))
	for _, key := range order {
		out = append(out, mergeFindingGroup(groups[key]))
	}
	return out
}

// findingMergeKey 计算风险点的归并键。
func findingMergeKey(f RiskFinding) string {
	if len(f.CandidateIDs) > 0 {
		ids := append([]string(nil), f.CandidateIDs...)
		sort.Strings(ids)
		return "cand:" + strings.Join(ids, "\x00")
	}
	basis := ""
	if len(f.LegalBasis) > 0 {
		b := f.LegalBasis[0]
		basis = strings.TrimSpace(b.Source + "|" + b.Article)
	}
	return "risk:" + f.RiskType + "|" + basis
}

// mergeFindingGroup 合并同一归并键的一组 finding。
// 主条取最早条款（group[0]），并聚合：ClauseIDs 并集、CandidateIDs 并集、
// 最严重风险等级、最高置信度、已验证/待人工复核按"任一命中"取或。
func mergeFindingGroup(group []RiskFinding) RiskFinding {
	if len(group) == 1 {
		return group[0]
	}

	merged := group[0]
	seenClause := make(map[string]bool, len(group))
	seenCand := make(map[string]bool, len(merged.CandidateIDs))
	clauses := make([]string, 0, len(group))
	candidates := make([]string, 0, len(merged.CandidateIDs))

	for _, f := range group {
		if f.ClauseID != "" && !seenClause[f.ClauseID] {
			seenClause[f.ClauseID] = true
			clauses = append(clauses, f.ClauseID)
		}
		for _, c := range f.CandidateIDs {
			if c != "" && !seenCand[c] {
				seenCand[c] = true
				candidates = append(candidates, c)
			}
		}
	}
	merged.ClauseIDs = clauses
	merged.CandidateIDs = candidates

	riskRank := map[string]int{"高": 3, "中": 2, "低": 1}
	for _, f := range group[1:] {
		if riskRank[f.RiskLevel] > riskRank[merged.RiskLevel] {
			merged.RiskLevel = f.RiskLevel
		}
		if f.Confidence > merged.Confidence {
			merged.Confidence = f.Confidence
		}
		if f.Verified && !merged.Verified {
			merged.Verified = true
		}
		if f.RequiresHumanReview && !merged.RequiresHumanReview {
			merged.RequiresHumanReview = true
		}
		// 主条缺依据/建议时，从后续命中补齐（保持最终报告完整）。
		if len(merged.LegalBasis) == 0 && len(f.LegalBasis) > 0 {
			merged.LegalBasis = f.LegalBasis
		}
		if strings.TrimSpace(merged.SuggestedText) == "" && strings.TrimSpace(f.SuggestedText) != "" {
			merged.SuggestedText = f.SuggestedText
			merged.SuggestionReason = f.SuggestionReason
			merged.Priority = f.Priority
		}
	}
	return merged
}
