package su_pool

import (
	"fmt"
	"testing"
)

func TestNew(t *testing.T) {
	p := NewLite(100)
	for i := 0; i < 1000; i++ {
		a := i
		p.AddTask(func() error {
			fmt.Println(a)
			return nil
		})
	}

	p.Wait()
}
