package cache

import (
	"errors"
)

type Base struct {
	prefix string
}

func (b *Base) GetKey(k string) string {
	if b.prefix != "" {
		k = b.prefix + ":" + k
	}

	//if len(k) > 64 {
	//	t := sha1.New()
	//	_, _ = io.WriteString(t, k)
	//	k = fmt.Sprintf("%x", t.Sum(nil))
	//}

	return k
}

type ZSorts []Z

type Z struct {
	Score  float64
	Member interface{}
}

func (z ZSorts) GetScore(mem interface{}) float64 {
	for _, v := range z {
		if v.Member == mem {
			return v.Score
		}
	}

	return 0
}

var ErrUnlockValNotMatch = errors.New("unlock failed value not match")

const EmptyString = "_empty_"
