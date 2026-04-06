package stack

import (
	"fmt"
	"testing"
)

func TestGet(t *testing.T) {
	type args struct {
		levels   int
		maxBytes int
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			"a",
			args{
				levels:   6,
				maxBytes: 256,
			},
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Get(&Option{
				Levels:    0,
				Size:      0,
				Separator: " --> ",
			})
			fmt.Println(got)
		})
	}
}
