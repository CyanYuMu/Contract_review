package utils

import (
	"contract_review/app/internal/comparison"
	"strings"
)

// SequenceMatcher 实现类似Python difflib.SequenceMatcher的序列匹配器
type SequenceMatcher struct {
	a        []string
	b        []string
	matching [][2]int
}

// NewSequenceMatcher 创建序列匹配器
func NewSequenceMatcher(a, b []string) *SequenceMatcher {
	return &SequenceMatcher{
		a: a,
		b: b,
	}
}

// OpCode 表示一个操作
type OpCode struct {
	Tag string
	I1  int
	I2  int
	J1  int
	J2  int
}

// GetOpcodes 获取操作序列
func (sm *SequenceMatcher) GetOpcodes() []OpCode {
	sm.findLongestMatches()

	var opcodes []OpCode
	i, j := 0, 0

	for _, m := range sm.matching {
		aIndex, bIndex := m[0], m[1]
		aLen, bLen := m[0], m[1]

		// 找到匹配块的长度
		matchLen := 0
		for aIndex+matchLen < len(sm.a) && bIndex+matchLen < len(sm.b) &&
			sm.a[aIndex+matchLen] == sm.b[bIndex+matchLen] {
			matchLen++
		}

		// 处理差异部分
		if aIndex > i {
			if bIndex > j {
				opcodes = append(opcodes, OpCode{
					Tag: "replace",
					I1:  i,
					I2:  aIndex,
					J1:  j,
					J2:  bIndex,
				})
			} else {
				opcodes = append(opcodes, OpCode{
					Tag: "delete",
					I1:  i,
					I2:  aIndex,
					J1:  j,
					J2:  j,
				})
			}
		} else if bIndex > j {
			opcodes = append(opcodes, OpCode{
				Tag: "insert",
				I1:  i,
				I2:  i,
				J1:  j,
				J2:  bIndex,
			})
		}

		// 添加相等部分
		if matchLen > 0 {
			opcodes = append(opcodes, OpCode{
				Tag: "equal",
				I1:  aIndex,
				I2:  aIndex + matchLen,
				J1:  bIndex,
				J2:  bIndex + matchLen,
			})
			i = aIndex + matchLen
			j = bIndex + matchLen
		}
	}

	// 处理末尾的差异
	if i < len(sm.a) || j < len(sm.b) {
		if i < len(sm.a) && j < len(sm.b) {
			opcodes = append(opcodes, OpCode{
				Tag: "replace",
				I1:  i,
				I2:  len(sm.a),
				J1:  j,
				J2:  len(sm.b),
			})
		} else if i < len(sm.a) {
			opcodes = append(opcodes, OpCode{
				Tag: "delete",
				I1:  i,
				I2:  len(sm.a),
				J1:  j,
				J2:  j,
			})
		} else if j < len(sm.b) {
			opcodes = append(opcodes, OpCode{
				Tag: "insert",
				I1:  i,
				I2:  i,
				J1:  j,
				J2:  len(sm.b),
			})
		}
	}

	return opcodes
}

// findLongestMatches 找到最长的匹配块
func (sm *SequenceMatcher) findLongestMatches() {
	sm.matching = nil

	// 使用简单的最长公共子序列算法
	// 构建2D表格
	m, n := len(sm.a), len(sm.b)
	if m == 0 || n == 0 {
		return
	}

	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

	// 找到最长公共子序列长度
	maxLen := 0
	endA := 0
	endB := 0

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if sm.a[i-1] == sm.b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
				if dp[i][j] > maxLen {
					maxLen = dp[i][j]
					endA = i
					endB = j
				}
			}
		}
	}

	// 记录匹配位置
	for maxLen > 0 {
		sm.matching = append([][2]int{{endA - maxLen, endB - maxLen}}, sm.matching...)
		endA -= maxLen
		endB -= maxLen
		maxLen = 0

		// 找下一个匹配块
		for i := endA; i >= 1; i-- {
			for j := endB; j >= 1; j-- {
				if dp[i][j] > maxLen {
					maxLen = dp[i][j]
					endA = i
					endB = j
				}
			}
		}
		if maxLen > 0 {
			sm.matching = append([][2]int{{endA - maxLen, endB - maxLen}}, sm.matching...)
		}
	}
}

// Ratio 计算相似度比例
func (sm *SequenceMatcher) Ratio() float64 {
	if len(sm.a) == 0 && len(sm.b) == 0 {
		return 1.0
	}

	matches := 0
	for _, m := range sm.matching {
		matches++
	}

	total := len(sm.a) + len(sm.b)
	if total == 0 {
		return 1.0
	}

	return 2.0 * float64(matches) / float64(total)
}

// DiffDocuments 比对两个文档
func DiffDocuments(stdLines, cmpLines []string) comparison.DiffResult {
	stdLines = filterEmptyLines(stdLines)
	cmpLines = filterEmptyLines(cmpLines)

	matcher := NewSequenceMatcher(stdLines, cmpLines)
	opcodes := matcher.GetOpcodes()

	var diffs []comparison.ComparisonParagraphDiff

	for _, op := range opcodes {
		switch op.Tag {
		case "delete":
			for offset := op.I1; offset < op.I2; offset++ {
				idx := offset
				diffs = append(diffs, comparison.ComparisonParagraphDiff{
					Operation:    "delete",
					StdIndex:     &idx,
					CmpIndex:     nil,
					StandardText: stdLines[offset],
				})
			}
		case "insert":
			for offset := op.J1; offset < op.J2; offset++ {
				idx := offset
				diffs = append(diffs, comparison.ComparisonParagraphDiff{
					Operation:      "insert",
					StdIndex:       nil,
					CmpIndex:       &idx,
					ComparisonText: cmpLines[offset],
				})
			}
		case "replace":
			maxLen := max(op.I2-op.I1, op.J2-op.J1)
			for k := 0; k < maxLen; k++ {
				stdIdx := op.I1 + k
				cmpIdx := op.J1 + k

				var stdText, cmpText string
				var stdIdxPtr, cmpIdxPtr *int

				if stdIdx < op.I2 {
					stdText = stdLines[stdIdx]
					stdIdxPtr = &stdIdx
				}
				if cmpIdx < op.J2 {
					cmpText = cmpLines[cmpIdx]
					cmpIdxPtr = &cmpIdx
				}

				charDiff := buildCharDiff(stdText, cmpText)

				diffs = append(diffs, comparison.ComparisonParagraphDiff{
					Operation:      "replace",
					StdIndex:       stdIdxPtr,
					CmpIndex:       cmpIdxPtr,
					StandardText:   stdText,
					ComparisonText: cmpText,
					CharDiff:       charDiff,
				})
			}
		}
	}

	stdFullText := strings.Join(stdLines, "\n")
	cmpFullText := strings.Join(cmpLines, "\n")
	similarity := calculateSimilarity(stdFullText, cmpFullText)

	summary := comparison.ComparisonSummary{
		StandardParagraphs:   len(stdLines),
		ComparisonParagraphs: len(cmpLines),
		DifferenceCount:      len(diffs),
		Similarity:           similarity,
	}

	return comparison.DiffResult{
		Summary:    summary,
		Diffs:      diffs,
		Similarity: similarity,
	}
}

// buildCharDiff 构建字符级差异
func buildCharDiff(stdText, cmpText string) []comparison.ComparisonDiffDetail {
	stdRunes := []rune(stdText)
	cmpRunes := []rune(cmpText)

	charMatcher := NewSequenceMatcher(
		runesToStrings(stdRunes),
		runesToStrings(cmpRunes),
	)
	opcodes := charMatcher.GetOpcodes()

	var charDiffs []comparison.ComparisonDiffDetail

	for _, op := range opcodes {
		if op.Tag == "equal" {
			continue
		}

		charDiffs = append(charDiffs, comparison.ComparisonDiffDetail{
			Operation: op.Tag,
			StdText:   string(stdRunes[op.I1:op.I2]),
			CmpText:   string(cmpRunes[op.J1:op.J2]),
			StdRange:  []int{op.I1, op.I2},
			CmpRange:  []int{op.J1, op.J2},
		})
	}

	return charDiffs
}

// runesToStrings 将rune切片转换为string切片
func runesToStrings(runes []rune) []string {
	result := make([]string, len(runes))
	for i, r := range runes {
		result[i] = string(r)
	}
	return result
}

// filterEmptyLines 过滤空行
func filterEmptyLines(lines []string) []string {
	var result []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			result = append(result, line)
		}
	}
	return result
}

// calculateSimilarity 计算文本相似度
func calculateSimilarity(text1, text2 string) float64 {
	if text1 == "" && text2 == "" {
		return 100.0
	}
	if text1 == "" || text2 == "" {
		return 0.0
	}

	runes1 := []rune(text1)
	runes2 := []rune(text2)

	m, n := len(runes1), len(runes2)

	// 使用优化的空间复杂度
	// 只保留两行
	prev := make([]int, n+1)
	curr := make([]int, n+1)

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if runes1[i-1] == runes2[j-1] {
				curr[j] = prev[j-1] + 1
			} else {
				curr[j] = max(prev[j], curr[j-1])
			}
		}
		prev, curr = curr, prev
	}

	lcsLen := prev[n]
	totalLen := m + n

	if totalLen == 0 {
		return 100.0
	}

	return roundToTwoDecimals(2.0 * float64(lcsLen) / float64(totalLen) * 100)
}

// roundToTwoDecimals 保留两位小数
func roundToTwoDecimals(val float64) float64 {
	return float64(int(val*100+0.5)) / 100
}

// max 返回较大的整数
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
