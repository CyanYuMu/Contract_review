package comparison

import (
	"contract_review/app/pkg/utils"
	"strings"
)

// DiffDocuments 比对两个文档，返回段落级和字符级差异
func DiffDocuments(stdLines, cmpLines []string) DiffResult {
	stdLines = filterEmptyLines(stdLines)
	cmpLines = filterEmptyLines(cmpLines)

	matcher := utils.NewSequenceMatcher(stdLines, cmpLines)
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
					StandardText: stdLines[offset],
				})
			}
		case "insert":
			for offset := op.J1; offset < op.J2; offset++ {
				idx := offset
				diffs = append(diffs, ComparisonParagraphDiff{
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

	stdFullText := strings.Join(stdLines, "\n")
	cmpFullText := strings.Join(cmpLines, "\n")
	similarity := utils.CalculateSimilarity(stdFullText, cmpFullText)

	summary := ComparisonSummary{
		StandardParagraphs:   len(stdLines),
		ComparisonParagraphs: len(cmpLines),
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
	stdRunes := []rune(stdText)
	cmpRunes := []rune(cmpText)

	charMatcher := utils.NewSequenceMatcher(
		utils.RunesToStrings(stdRunes),
		utils.RunesToStrings(cmpRunes),
	)
	opcodes := charMatcher.GetOpcodes()

	var charDiffs []ComparisonDiffDetail
	for _, op := range opcodes {
		if op.Tag == "equal" {
			continue
		}
		charDiffs = append(charDiffs, ComparisonDiffDetail{
			Operation: op.Tag,
			StdText:   string(stdRunes[op.I1:op.I2]),
			CmpText:   string(cmpRunes[op.J1:op.J2]),
			StdRange:  []int{op.I1, op.I2},
			CmpRange:  []int{op.J1, op.J2},
		})
	}
	return charDiffs
}

func filterEmptyLines(lines []string) []string {
	var result []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			result = append(result, line)
		}
	}
	return result
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
