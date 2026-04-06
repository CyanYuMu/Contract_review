package su_math

import (
	"golang.org/x/exp/constraints"
)

func GetMax[T constraints.Ordered](i ...T) T {
	var max = i[0]
	for j := range i {
		if i[j] > max {
			max = i[j]
		}
	}

	return max
}

func GetMin[T constraints.Ordered](i ...T) T {
	var min = i[0]
	for j := range i {
		if i[j] < min {
			min = i[j]
		}
	}

	return min
}

type Number interface {
	constraints.Float | constraints.Integer
}

func GetWithDefault[T Number](i T, dft T) T {
	if i == 0 {
		return dft
	}

	return i
}

func Ternary[T constraints.Ordered](ok bool, trueA, falseB T) T {
	if ok {
		return trueA
	}
	return falseB
}
