package mapping

import (
	"bytes"
	jsoniter "github.com/json-iterator/go"
	"github.com/spf13/cast"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_string"
	"reflect"
	"strings"
)

// StringInterface2Struct
// @description 将map[string]interface{}转换为struct, 使用 json tag 进行映射, 如果未指定json tag, 将对字段名转成蛇形命名再进行匹配
func StringInterface2Struct(mapData map[string]interface{}, data interface{}) error {
	rt := reflect.TypeOf(data)
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}

	numField := rt.NumField()
	jsonBuf := bytes.Buffer{}
	jsonBuf.WriteByte('{')
	for i := 0; i < numField; i++ {
		tagName := rt.Field(i).Tag.Get("json")
		if tagName == "" {
			tagName = su_string.Camel2Case(rt.Field(i).Name)
		}
		val, exists := mapData[tagName]
		if !exists {
			continue
		}
		strVal := cast.ToString(val)
		switch rt.Field(i).Type.Kind() {
		case reflect.String:
			jsonBuf.WriteString(`"` + tagName + `":"` + strVal + `",`)
		case reflect.Bool:
			if strings.ToLower(strVal) == "true" || strVal == "1" {
				strVal = "true"
			} else {
				strVal = "false"
			}
			jsonBuf.WriteString(`"` + tagName + `":` + strVal + ",")
		case reflect.Int, reflect.Int32, reflect.Int64, reflect.Float32, reflect.Float64, reflect.Uint, reflect.Uint32, reflect.Uint64, reflect.Uint16, reflect.Uint8:
			jsonBuf.WriteString(`"` + tagName + `":` + strVal + ",")
		default:
			jsonBuf.WriteString(`"` + tagName + `":"` + strVal + `",`)
		}
	}

	byteData := jsonBuf.Bytes()
	if len(byteData) > 1 {
		byteData = byteData[0 : len(byteData)-1]
	}
	byteData = append(byteData, '}')
	err := jsoniter.Unmarshal(byteData, data)

	return err
}

// StringString2Struct
// @description 将map[string]string转换为struct, 使用 json tag 进行映射, 如果未指定json tag, 将对字段名转成蛇形命名再进行匹配
func StringString2Struct(mapData map[string]string, data interface{}) error {
	rt := reflect.TypeOf(data)
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}

	numField := rt.NumField()
	jsonBuf := bytes.Buffer{}
	jsonBuf.WriteByte('{')
	for i := 0; i < numField; i++ {
		tagName := rt.Field(i).Tag.Get("json")
		if tagName == "" {
			tagName = su_string.Camel2Case(rt.Field(i).Name)
		}
		strVal, exists := mapData[tagName]
		if !exists {
			continue
		}
		switch rt.Field(i).Type.Kind() {
		case reflect.String:
			jsonBuf.WriteString(`"` + tagName + `":"` + strVal + `",`)
		case reflect.Bool:
			if strings.ToLower(strVal) == "true" || strVal == "1" {
				strVal = "true"
			} else {
				strVal = "false"
			}
			jsonBuf.WriteString(`"` + tagName + `":` + strVal + ",")
		case reflect.Int, reflect.Int32, reflect.Int64, reflect.Float32, reflect.Float64, reflect.Uint, reflect.Uint32, reflect.Uint64, reflect.Uint16, reflect.Uint8:
			jsonBuf.WriteString(`"` + tagName + `":` + strVal + ",")
		default:
			jsonBuf.WriteString(`"` + tagName + `":"` + strVal + `",`)
		}
	}
	byteData := jsonBuf.Bytes()
	if len(byteData) > 1 {
		byteData = byteData[0 : len(byteData)-1]
	}
	byteData = append(byteData, '}')
	err := jsoniter.Unmarshal(byteData, data)

	return err
}
