package rag

import "math"

// compositeScore 计算复合分数
//
// 权重分配:
//
//	60% — Rerank 模型分数 (Cross-Encoder 精排)
//	30% — 原始检索分数 (RRF 融合后的分数)
//	10% — 来源权重 (知识库来源 > 外部来源)
//
// 位置先验: 段落内位置靠前的内容相关性稍高 (+5% bias)
//
// 分数范围: [0, 1]
func compositeScore(result *SearchResult, rerankScore, baseScore float64, isWebSource bool) float64 {
	// 来源权重: 内部知识库 1.0, 外部(web_search等) 0.95
	sourceWeight := 1.0
	if isWebSource {
		sourceWeight = 0.95
	}

	// 位置先验: ±5% 微调 (越靠前的内容通常与文档主题更相关)
	positionPrior := 1.0
	if result.ChunkID != "" {
		// 基于 chunk_id 的简单启发式: 较小编号的 chunk 通常在文档前面
		// 这里只做微调，不影响主要排序
		positionPrior = 1.0 + clampFloat(0.05, -0.05, 0.05)
	}

	// 如果没有有效的 base score，使用 rerank score
	if baseScore <= 0 {
		baseScore = rerankScore
	}

	// 复合公式
	composite := 0.6*rerankScore + 0.3*baseScore + 0.1*sourceWeight
	composite *= positionPrior

	return clampFloat(composite, 0, 1)
}

// clampFloat 将值限制在 [min, max] 范围内
func clampFloat(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// clampInt 将 int 值限制在 [min, max] 范围内
func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// computeEffectiveRRFWeights 计算有效的 RRF 权重
// 如果多种检索方式中某一种未启用，重新分配权重
func computeEffectiveRRFWeights(
	vectorResults, bm25Results, keywordResults []SearchResult,
	baseVectorWeight, baseBM25Weight, baseKeywordWeight float64,
) (float64, float64, float64) {
	var activeWeights []float64
	var activeIndices []int

	if len(vectorResults) > 0 {
		activeWeights = append(activeWeights, baseVectorWeight)
		activeIndices = append(activeIndices, 0)
	}
	if len(bm25Results) > 0 {
		activeWeights = append(activeWeights, baseBM25Weight)
		activeIndices = append(activeIndices, 1)
	}
	if len(keywordResults) > 0 {
		activeWeights = append(activeWeights, baseKeywordWeight)
		activeIndices = append(activeIndices, 2)
	}

	// 正常化权重
	totalWeight := 0.0
	for _, w := range activeWeights {
		totalWeight += w
	}

	weights := []float64{0, 0, 0} // [vector, bm25, keyword]
	if totalWeight > 0 {
		for i, w := range activeWeights {
			weights[activeIndices[i]] = w / totalWeight
		}
	} else if len(activeWeights) > 0 {
		// 平均分配
		avg := 1.0 / float64(len(activeWeights))
		for _, idx := range activeIndices {
			weights[idx] = avg
		}
	}

	return weights[0], weights[1], weights[2]
}

// dedupByChunkID 按 ChunkID 去重，保留最高分
func dedupByChunkID(results []SearchResult) []SearchResult {
	if len(results) <= 1 {
		return results
	}

	seen := make(map[string]SearchResult)
	for _, r := range results {
		if existing, ok := seen[r.ChunkID]; ok {
			if r.Score > existing.Score {
				seen[r.ChunkID] = r
			}
		} else {
			seen[r.ChunkID] = r
		}
	}

	deduped := make([]SearchResult, 0, len(seen))
	for _, r := range seen {
		deduped = append(deduped, r)
	}
	return deduped
}

// truncateResults 截断到 TopK
func (c *RetrieverConfig) effectiveFinalTopK() int {
	if c.FinalTopK <= 0 {
		return 5
	}
	return c.FinalTopK
}

// safeMin 安全地返回 float64 最小值
func safeMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ensureValidThreshold 确保 Rerank 阈值在合理范围
func ensureValidThreshold(threshold float64) float64 {
	return clampFloat(threshold, 0.0, 1.0)
}

// logitToScore 将 logit 值转为 0-1 分数（用于 NVIDIA 等原始 logit 输出）
func logitToScore(logit float64) float64 {
	return 1.0 / (1.0 + math.Exp(-logit))
}
