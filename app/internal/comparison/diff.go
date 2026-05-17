package comparison

import (
	"contract_review/app/pkg/utils"
	"strings"
	"unicode"
)

type preparedLine struct {
	text       string
	compareKey string
}

type diffUnit struct {
	text  string
	start int
	end   int
}

// DiffDocuments 比对两个文档，返回段落级和字符级差异
func DiffDocuments(stdLines, cmpLines []string) DiffResult {
	stdPrepared := prepareLines(stdLines)
	cmpPrepared := prepareLines(cmpLines)

	stdCompareLines := extractCompareLines(stdPrepared)
	cmpCompareLines := extractCompareLines(cmpPrepared)
	stdCompareText := strings.Join(stdCompareLines, "")
	cmpCompareText := strings.Join(cmpCompareLines, "")

	if stdCompareText == cmpCompareText {
		return DiffResult{
			Summary: ComparisonSummary{
				StandardParagraphs:   len(stdPrepared),
				ComparisonParagraphs: len(cmpPrepared),
				DifferenceCount:      0,
				Similarity:           100,
			},
			Diffs:      nil,
			Similarity: 100,
		}
	}

	matcher := utils.NewSequenceMatcher(stdCompareLines, cmpCompareLines)
	opcodes := matcher.GetOpcodes()

	var diffs []ComparisonParagraphDiff

	for _, op := range opcodes {
		switch op.Tag {
		case "delete":
			for offset := op.I1; offset < op.I2; offset++ {
				idx := offset
				diffs = append(diffs, ComparisonParagraphDiff{
					Operation:    "delete",
					StdIndex:     &idx,
					CmpIndex:     nil,
					StandardText: stdPrepared[offset].text,
				})
			}
		case "insert":
			for offset := op.J1; offset < op.J2; offset++ {
				idx := offset
				diffs = append(diffs, ComparisonParagraphDiff{
					Operation:      "insert",
					StdIndex:       nil,
					CmpIndex:       &idx,
					ComparisonText: cmpPrepared[offset].text,
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
					stdText = stdPrepared[stdIdx].text
					stdIdxPtr = &stdIdx
				}
				if cmpIdx < op.J2 {
					cmpText = cmpPrepared[cmpIdx].text
					cmpIdxPtr = &cmpIdx
				}

				charDiff := buildCharDiff(stdText, cmpText)
				if len(charDiff) == 0 {
					continue
				}

				diffs = append(diffs, ComparisonParagraphDiff{
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

	similarity := utils.CalculateSimilarity(stdCompareText, cmpCompareText)

	summary := ComparisonSummary{
		StandardParagraphs:   len(stdPrepared),
		ComparisonParagraphs: len(cmpPrepared),
		DifferenceCount:      len(diffs),
		Similarity:           similarity,
	}

	return DiffResult{
		Summary:    summary,
		Diffs:      diffs,
		Similarity: similarity,
	}
}

func buildCharDiff(stdText, cmpText string) []ComparisonDiffDetail {
	stdUnits := buildDiffUnits(stdText)
	cmpUnits := buildDiffUnits(cmpText)

	charMatcher := utils.NewSequenceMatcher(
		unitsToStrings(stdUnits),
		unitsToStrings(cmpUnits),
	)
	opcodes := charMatcher.GetOpcodes()

	var charDiffs []ComparisonDiffDetail
	for _, op := range opcodes {
		if op.Tag == "equal" {
			continue
		}
		stdSnippet, stdRange := unitsSnippet(stdUnits, op.I1, op.I2)
		cmpSnippet, cmpRange := unitsSnippet(cmpUnits, op.J1, op.J2)
		charDiffs = append(charDiffs, ComparisonDiffDetail{
			Operation: op.Tag,
			StdText:   stdSnippet,
			CmpText:   cmpSnippet,
			StdRange:  stdRange,
			CmpRange:  cmpRange,
		})
	}
	return charDiffs
}

func prepareLines(lines []string) []preparedLine {
	var result []preparedLine
	for _, line := range lines {
		text := strings.TrimSpace(normalizeDisplayWhitespace(line))
		compareKey := normalizeForCompare(text)
		if compareKey != "" {
			result = append(result, preparedLine{text: text, compareKey: compareKey})
		}
	}
	return result
}

func extractCompareLines(lines []preparedLine) []string {
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		result = append(result, line.compareKey)
	}
	return result
}

func normalizeDisplayWhitespace(text string) string {
	replacer := strings.NewReplacer(
		"\u00a0", " ",
		"\u2007", " ",
		"\u202f", " ",
		"\t", " ",
	)
	return replacer.Replace(text)
}

func normalizeForCompare(text string) string {
	var builder strings.Builder
	for _, r := range normalizeDisplayWhitespace(text) {
		if isFormatNeutralRune(r) {
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

func buildDiffUnits(text string) []diffUnit {
	var units []diffUnit
	runeIndex := 0
	for _, r := range normalizeDisplayWhitespace(text) {
		start := runeIndex
		runeIndex++
		if isFormatNeutralRune(r) {
			continue
		}
		units = append(units, diffUnit{
			text:  string(r),
			start: start,
			end:   runeIndex,
		})
	}
	return units
}

func unitsToStrings(units []diffUnit) []string {
	result := make([]string, len(units))
	for i, unit := range units {
		result[i] = unit.text
	}
	return result
}

func unitsSnippet(units []diffUnit, start, end int) (string, []int) {
	if start >= end || start < 0 || end > len(units) {
		return "", nil
	}
	var builder strings.Builder
	for _, unit := range units[start:end] {
		builder.WriteString(unit.text)
	}
	return builder.String(), []int{units[start].start, units[end-1].end}
}

func isFormatNeutralRune(r rune) bool {
	return unicode.IsSpace(r) || r == '\u200b' || r == '\u200c' || r == '\u200d' || r == '\ufeff'
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
