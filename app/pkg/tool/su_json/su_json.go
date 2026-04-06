package su_json

import (
	"github.com/bytedance/sonic"
)

func MustMarshalToString(i interface{}) string {
	toString, err := sonic.MarshalString(i)
	if err != nil {
		panic(err)
	}

	return toString
}

func MustMarshal(i interface{}) []byte {
	toString, err := sonic.Marshal(i)
	if err != nil {
		panic(err)
	}

	return toString
}

func MustUnmarshalFromString(i string, data interface{}) {
	if i == "" {
		return
	}
	err := sonic.UnmarshalString(i, data)
	if err != nil {
		panic(err)
	}
}

func MustUnmarshal(i []byte, data interface{}) {
	if len(i) == 0 {
		return
	}
	err := sonic.Unmarshal(i, data)
	if err != nil {
		panic(err)
	}
}

func Marshal(i any) ([]byte, error) {
	return sonic.Marshal(i)
}

func Unmarshal(i []byte, data any) error {
	return sonic.Unmarshal(i, data)
}

func MarshalToString(i any) (string, error) {
	return sonic.MarshalString(i)
}

func UnmarshalFromString(i string, data any) error {
	return sonic.UnmarshalString(i, data)
}
