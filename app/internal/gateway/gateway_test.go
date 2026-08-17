package gateway

import "testing"

func TestSemanticAnswerCacheAllowed(t *testing.T) {
	tests := []struct {
		feature string
		want    bool
	}{
		{feature: FeatureReview, want: false},
		{feature: FeatureQA, want: false},
		{feature: FeatureChat, want: true},
		{feature: FeatureCompare, want: true},
		{feature: FeatureDefault, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.feature, func(t *testing.T) {
			if got := semanticAnswerCacheAllowed(tt.feature); got != tt.want {
				t.Fatalf("semanticAnswerCacheAllowed(%q) = %v, want %v", tt.feature, got, tt.want)
			}
		})
	}
}
