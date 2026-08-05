package rag

import (
	"sort"
	"strings"
	"sync"
)

// maximalMarginalRelevance 最大边际相关性算法
// 平衡检索结果的相关性（relevance）与多样性（diversity penalty）
//
// lambda: 相关性与多样性权衡因子
//
//	lambda=1.0 → 纯相关性排序（等价于普通分数排序）
//	lambda=0.7 → 默认值，70% 相关性 + 30% 多样性惩罚（与已选结果相似的被降权）
//
// 冗余度: 使用 Jaccard 相似度（token 集合交集/并集）
// 选择策略: 贪心选择 — 每次选 mmr 分数最高的未选结果
func maximalMarginalRelevance(
	results []SearchResult,
	lambda float64,
	topK int,
) []SearchResult {
	if len(results) <= 1 || topK <= 0 || topK >= len(results) {
		return results
	}

	if lambda < 0 {
		lambda = 0
	}
	if lambda > 1 {
		lambda = 1
	}

	// Step 1: 预计算每个文档的 token 集合（并发构建）
	type tokenSet struct {
		tokens map[string]struct{}
	}
	sets := make([]tokenSet, len(results))
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sets[idx] = tokenSet{tokens: tokenizeForMMR(results[idx].Content)}
		}(i)
	}
	wg.Wait()

	// Step 2: 贪心 MMR 选择
	selected := make([]SearchResult, 0, topK)
	candidates := make([]int, len(results))
	for i := range candidates {
		candidates[i] = i
	}

	for len(selected) < topK && len(candidates) > 0 {
		bestIdx := candidates[0]
		bestMMR := -1.0

		for _, idx := range candidates {
			relevance := results[idx].Score
			redundancy := 0.0

			if len(selected) > 0 && lambda < 1.0 {
				// 计算与已选结果的最大 Jaccard 相似度
				for _, sel := range selected {
					selIdx := indexOf(results, sel)
					if selIdx >= 0 {
						sim := jaccardSimilarity(sets[idx].tokens, sets[selIdx].tokens)
						if sim > redundancy {
							redundancy = sim
						}
					}
				}
			}

			mmr := lambda*relevance - (1.0-lambda)*redundancy
			if mmr > bestMMR {
				bestMMR = mmr
				bestIdx = idx
			}
		}

		selected = append(selected, results[bestIdx])
		candidates = removeIndex(candidates, bestIdx)
	}

	// 如果 MMR 选择不足 topK，补充剩余的最高分结果
	if len(selected) < topK {
		selectedSet := make(map[string]bool)
		for _, s := range selected {
			selectedSet[s.ChunkID] = true
		}
		for _, r := range results {
			if !selectedSet[r.ChunkID] {
				selected = append(selected, r)
				selectedSet[r.ChunkID] = true
				if len(selected) >= topK {
					break
				}
			}
		}
	}

	return selected
}

// jaccardSimilarity 计算 Jaccard 相似度: |A ∩ B| / |A ∪ B|
func jaccardSimilarity(a, b map[string]struct{}) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}

	intersection := 0
	// 遍历较小的集合
	if len(a) > len(b) {
		a, b = b, a
	}
	for token := range a {
		if _, ok := b[token]; ok {
			intersection++
		}
	}

	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}

	return float64(intersection) / float64(union)
}

// tokenizeForMMR 将文本分词为 token 集合（用于 Jaccard 相似度计算）
// 使用简单的中英文混合分词策略
func tokenizeForMMR(text string) map[string]struct{} {
	tokens := make(map[string]struct{})

	// 统一小写
	text = strings.ToLower(strings.TrimSpace(text))

	// 英文单词 + 中文双字 bigram
	// Step 1: 按空白分割得到英文单词
	parts := strings.Fields(text)
	for _, part := range parts {
		// 过滤过短的 token
		if len(part) < 2 {
			continue
		}
		// 判断是否包含中文
		runes := []rune(part)
		hasCJK := false
		for _, r := range runes {
			if (r >= '一' && r <= '鿿') || (r >= '㐀' && r <= '䶿') {
				hasCJK = true
				break
			}
		}

		if hasCJK {
			// CJK bigram 分词
			for i := 0; i < len(runes)-1; i++ {
				bigram := string(runes[i : i+2])
				tokens[bigram] = struct{}{}
			}
			// 同时也保留单独的 rune
			for _, r := range runes {
				if r >= '一' && r <= '鿿' {
					tokens[string(r)] = struct{}{}
				}
			}
		} else {
			// 英文单词直接作为 token
			if len(part) >= 2 {
				tokens[part] = struct{}{}
			}
		}
	}

	return tokens
}

// indexOf 查找 SearchResult 在 slice 中的索引
func indexOf(results []SearchResult, target SearchResult) int {
	for i, r := range results {
		if r.ChunkID == target.ChunkID {
			return i
		}
	}
	return -1
}

// removeIndex 从 int slice 中移除指定值
func removeIndex(slice []int, value int) []int {
	for i, v := range slice {
		if v == value {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

// sortByCompositeScore 按复合分数降序排序
func sortByCompositeScore(results []SearchResult) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
}
