package utils

import "strings"

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

		matchLen := 0
		for aIndex+matchLen < len(sm.a) && bIndex+matchLen < len(sm.b) &&
			sm.a[aIndex+matchLen] == sm.b[bIndex+matchLen] {
			matchLen++
		}

		if aIndex > i {
			if bIndex > j {
				opcodes = append(opcodes, OpCode{Tag: "replace", I1: i, I2: aIndex, J1: j, J2: bIndex})
			} else {
				opcodes = append(opcodes, OpCode{Tag: "delete", I1: i, I2: aIndex, J1: j, J2: j})
			}
		} else if bIndex > j {
			opcodes = append(opcodes, OpCode{Tag: "insert", I1: i, I2: i, J1: j, J2: bIndex})
		}

		if matchLen > 0 {
			opcodes = append(opcodes, OpCode{Tag: "equal", I1: aIndex, I2: aIndex + matchLen, J1: bIndex, J2: bIndex + matchLen})
			i = aIndex + matchLen
			j = bIndex + matchLen
		}
	}

	if i < len(sm.a) || j < len(sm.b) {
		if i < len(sm.a) && j < len(sm.b) {
			opcodes = append(opcodes, OpCode{Tag: "replace", I1: i, I2: len(sm.a), J1: j, J2: len(sm.b)})
		} else if i < len(sm.a) {
			opcodes = append(opcodes, OpCode{Tag: "delete", I1: i, I2: len(sm.a), J1: j, J2: j})
		} else if j < len(sm.b) {
			opcodes = append(opcodes, OpCode{Tag: "insert", I1: i, I2: i, J1: j, J2: len(sm.b)})
		}
	}

	return opcodes
}

func (sm *SequenceMatcher) findLongestMatches() {
	sm.matching = nil

	m, n := len(sm.a), len(sm.b)
	if m == 0 || n == 0 {
		return
	}

	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

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

	for maxLen > 0 {
		sm.matching = append([][2]int{{endA - maxLen, endB - maxLen}}, sm.matching...)
		endA -= maxLen
		endB -= maxLen
		maxLen = 0

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

	matches := len(sm.matching)
	total := len(sm.a) + len(sm.b)
	if total == 0 {
		return 1.0
	}

	return 2.0 * float64(matches) / float64(total)
}

// RunesToStrings 将rune切片转换为string切片
func RunesToStrings(runes []rune) []string {
	result := make([]string, len(runes))
	for i, r := range runes {
		result[i] = string(r)
	}
	return result
}

// CalculateSimilarity 计算文本相似度（返回 0-100 百分比）
func CalculateSimilarity(text1, text2 string) float64 {
	if text1 == "" && text2 == "" {
		return 100.0
	}
	if text1 == "" || text2 == "" {
		return 0.0
	}

	runes1 := []rune(text1)
	runes2 := []rune(text2)

	m, n := len(runes1), len(runes2)

	prev := make([]int, n+1)
	curr := make([]int, n+1)

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if runes1[i-1] == runes2[j-1] {
				curr[j] = prev[j-1] + 1
			} else {
				curr[j] = maxInt(prev[j], curr[j-1])
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

// ExtractParagraphsFromDOCX extracts paragraphs from text (split by newlines)
func ExtractParagraphsFromText(text string) []string {
	lines := strings.Split(text, "\n")
	var result []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			result = append(result, line)
		}
	}
	return result
}

func roundToTwoDecimals(val float64) float64 {
	return float64(int(val*100+0.5)) / 100
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
