package su_struct

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"reflect"
	"strings"

	jsoniter "github.com/json-iterator/go"
)

//将src中的值拷贝给dst
//对象值拷贝

func DeepCopy(dst, src interface{}) error {
	if src == nil {
		return nil
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(src); err != nil {
		return err
	}
	return gob.NewDecoder(bytes.NewBuffer(buf.Bytes())).Decode(dst)
}

// JsonStrToMap json string to map
func JsonStrToMap(data []byte) (map[string]interface{}, error) {
	m := make(map[string]interface{})
	return m, jsoniter.Unmarshal(data, &m)
}

func MakeUpdateField(i interface{}) map[string]interface{} {
	t := reflect.TypeOf(i)
	v := reflect.ValueOf(i)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
		v = v.Elem()
	}
	field := make(map[string]interface{})
	for j := 0; j < t.NumField(); j++ {
		if !v.Field(j).IsZero() {
			field[snakeString(t.Field(j).Name)] = v.Field(j).Interface()
		}
	}
	return field
}

func snakeString(s string) string {
	char := make([]byte, 0, len(s)*2)
	num := len(s)
	for i := 0; i < num; i++ {
		if i > 0 {
			if !(isUp(s[i]) == isUp(s[i-1])) {
				if !isUp(s[i-1]) {
					char = append(char, '_')
				}
			} else if isUp(s[i]) && isUp(s[i-1]) && i < (num-2) {
				if !isUp(s[i+1]) {
					char = append(char, '_')
				}
			}
		}
		char = append(char, s[i])
	}
	return strings.ToLower(string(char))
}

func isUp(i byte) bool {
	return i >= 'A' && i <= 'Z'
}

// 判断结构体是否存在某个属性值
func IsExistField(i interface{}, field string) bool {
	t := reflect.TypeOf(i)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	for j := 0; j < t.NumField(); j++ {
		if t.Field(j).Name == field {
			return true
		}
	}
	return false
}

// StructToMap in转化成map
func StructToMap(in interface{}) (map[string]interface{}, error) {
	data, err := jsoniter.Marshal(in)
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{})
	return m, jsoniter.Unmarshal(data, &m)
}

// MapToStruct map in 转换成 struct out
func MapToStruct(in interface{}, out interface{}) error {
	data, err := jsoniter.Marshal(in)
	if err != nil {
		return err
	}
	return jsoniter.Unmarshal(data, out)
}

type MergeOption struct {
	// 覆盖非零值, default false
	OverwriteNonZero bool
	// 即使src的属性为零值，也进行覆盖, 否则只覆盖不为零值的属性, default false
	IncludeZeroSourceField bool
}

// 合并两个结构体实例，采用非零值
func Merge(dst, src interface{}, opt *MergeOption) error {
	if opt == nil {
		opt = &MergeOption{
			OverwriteNonZero:       false,
			IncludeZeroSourceField: false,
		}
	}
	// 检查传入参数是否为指针
	dstValue := reflect.ValueOf(dst)
	srcValue := reflect.ValueOf(src)

	if dstValue.Kind() != reflect.Ptr || srcValue.Kind() != reflect.Ptr {
		return fmt.Errorf("both dst and src must be pointers to structs")
	}

	// 检查 dst 是否为指针的指针并且是否为 nil
	if dstValue.Elem().Kind() == reflect.Ptr && dstValue.Elem().IsNil() {
		// 为 dst 分配内存并将 src 赋值给 dst
		dstValue.Elem().Set(reflect.New(srcValue.Elem().Type()))
		dstValue.Elem().Elem().Set(srcValue.Elem())
		return nil
	}

	// 获取指针指向的值
	dstElem := dstValue.Elem()
	srcElem := srcValue.Elem()

	// 检查指针指向的是否为结构体
	if dstElem.Kind() != reflect.Struct || srcElem.Kind() != reflect.Struct {
		return fmt.Errorf("both dst and src must be pointers to structs")
	}

	for i := 0; i < dstElem.NumField(); i++ {
		dstField := dstElem.Field(i)
		srcField := srcElem.Field(i)

		if opt.IncludeZeroSourceField {
			dstField.Set(srcField)
		} else if !IsZeroValue(srcField) {
			if opt.OverwriteNonZero || IsZeroValue(dstField) {
				dstField.Set(srcField)
			}
		}
	}
	return nil
}

// 判断值是否为零值
func IsZeroValue(v reflect.Value) bool {
	zero := reflect.Zero(v.Type()).Interface()
	return reflect.DeepEqual(v.Interface(), zero)
}

// CompareFieldValues 用于比较两个结构体中特定字段的值是否相同
func CompareFieldValues(struct1, struct2 interface{}, fieldNames []string) (bool, error) {
	val1 := reflect.ValueOf(struct1)
	val2 := reflect.ValueOf(struct2)

	if val1.Kind() != reflect.Struct || val2.Kind() != reflect.Struct {
		return false, fmt.Errorf("both arguments must be structs")
	}

	for _, fieldName := range fieldNames {
		field1 := val1.FieldByName(fieldName)
		field2 := val2.FieldByName(fieldName)

		if !field1.IsValid() || !field2.IsValid() {
			return false, fmt.Errorf("field %s not found in one of the structs", fieldName)
		}

		if !compareValues(field1, field2) {
			return false, nil
		}
	}

	return true, nil
}

// compareValues 比较两个反射值是否相同
func compareValues(val1, val2 reflect.Value) bool {
	if val1.Kind() == reflect.Ptr && val2.Kind() == reflect.Ptr {
		if val1.IsNil() || val2.IsNil() {
			return val1.IsNil() == val2.IsNil()
		}
		return compareValues(val1.Elem(), val2.Elem())
	}

	if val1.Kind() == reflect.Struct && val2.Kind() == reflect.Struct {
		for i := 0; i < val1.NumField(); i++ {
			if !compareValues(val1.Field(i), val2.Field(i)) {
				return false
			}
		}
		return true
	}

	return reflect.DeepEqual(val1.Interface(), val2.Interface())
}
