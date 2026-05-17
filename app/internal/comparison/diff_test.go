package comparison

import "testing"

func TestDiffDocumentsIgnoresFormattingWhitespace(t *testing.T) {
	result := DiffDocuments(
		[]string{"第一条  甲方应按期付款。", "第二条\t乙方交付货物。"},
		[]string{"第一条甲方应按期付款。", "第二条 乙方交付货物。"},
	)

	if result.Summary.DifferenceCount != 0 {
		t.Fatalf("expected no content differences, got %d: %#v", result.Summary.DifferenceCount, result.Diffs)
	}
	if result.Similarity != 100 {
		t.Fatalf("expected similarity 100, got %v", result.Similarity)
	}
}

func TestDiffDocumentsIgnoresLineBreakOnlyChanges(t *testing.T) {
	result := DiffDocuments(
		[]string{"第一条甲方应按期付款。"},
		[]string{"第一条甲方", "应按期付款。"},
	)

	if result.Summary.DifferenceCount != 0 {
		t.Fatalf("expected line break only changes to be ignored, got %d: %#v", result.Summary.DifferenceCount, result.Diffs)
	}
}

func TestDiffDocumentsDetectsTextChange(t *testing.T) {
	result := DiffDocuments(
		[]string{"第一条甲方应在三日内付款。"},
		[]string{"第一条甲方应在五日内付款。"},
	)

	if result.Summary.DifferenceCount != 1 {
		t.Fatalf("expected one content difference, got %d: %#v", result.Summary.DifferenceCount, result.Diffs)
	}
	diff := result.Diffs[0]
	if diff.Operation != "replace" {
		t.Fatalf("expected replace diff, got %q", diff.Operation)
	}
	if len(diff.CharDiff) == 0 {
		t.Fatal("expected character-level difference")
	}

	found := false
	for _, charDiff := range diff.CharDiff {
		if charDiff.StdText == "三" && charDiff.CmpText == "五" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected char diff 三 -> 五, got %#v", diff.CharDiff)
	}
}
