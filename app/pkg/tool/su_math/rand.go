package su_math

import (
	"math/rand"
	"time"
)

func RandInt(min, max int) int {
	src := rand.NewSource(time.Now().UnixNano())
	return rand.New(src).Intn(max-min+1) + min
}

func RandInt64(min, max int) int64 {
	src := rand.NewSource(time.Now().UnixNano())
	return int64(rand.New(src).Intn(max-min+1) + min)
}

// RandFloat32
func RandFloat32(min, max float32) float32 {
	src := rand.NewSource(time.Now().UnixNano())
	return rand.New(src).Float32()*(max-min) + min
}

// RandFloat64
func RandFloat64(min, max float64) float64 {
	src := rand.NewSource(time.Now().UnixNano())
	return rand.New(src).Float64()*(max-min) + min
}
