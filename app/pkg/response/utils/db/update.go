package db

import (
	"reflect"
	"strings"
)

type ToUpdateOptions struct {
	// ? 自定义判断是否进行过滤
	Filter func(tagName string, v interface{}) (ignore bool)
	// ? 自定义判断是否进行过滤
	EmptyIgnore func(tagName string, v interface{}) (ignore bool)
	// ? 标签tag名称
	Tag string
	// 忽略更新时间字段, 默认自动填充
	IgnoreUpdateTimeField bool
}

func (o *ToUpdateOptions) Apply(opt *ToUpdateOptions) {
	if opt == nil {
		return
	}
	if opt.EmptyIgnore != nil {
		o.EmptyIgnore = opt.EmptyIgnore
	}
	if opt.Tag != "" {
		o.Tag = opt.Tag
	}

	if opt.IgnoreUpdateTimeField {
		o.IgnoreUpdateTimeField = opt.IgnoreUpdateTimeField
	}
}

func dftFirestoreToUpdateOptions() *UpdateOptions {
	return &UpdateOptions{
		EmptyIgnore: nil,
		Tag:         "firestore",
	}
}

func dftMongoToUpdateOptions() *UpdateOptions {
	return &UpdateOptions{
		EmptyIgnore: nil,
		Tag:         "bson",
	}
}

//func ToUpdateOne(data any, options *UpdateOptions) (upData UpdateOne) {
//	opt := dftToUpdateOptions()
//	opt.Apply(options)
//	v := reflect.ValueOf(data)
//	t := v.Type()
//	if t.Kind() == reflect.Ptr {
//		v = v.Elem()
//		t = v.Type()
//	}
//
//	if t.Kind() != reflect.Struct {
//		panic("data must be struct")
//	}
//
//	var containUpdateField bool
//	processStruct(v, t, opt, &upData, &containUpdateField)
//
//	if len(upData.Updates) > 0 {
//		if !containUpdateField && !opt.IgnoreUpdateTimeField {
//			upData.Updates = append(upData.Updates, Update{Field: UpdateField, Value: su_time.CurrentTimestampMilli()})
//		}
//	}
//
//	return upData
//}

func processStruct(v reflect.Value, t reflect.Type, opt *UpdateOptions, upData *UpdateOne, containUpdateField *bool) {
	nF := v.NumField()
	for i := 0; i < nF; i++ {
		field := t.Field(i)
		value := v.Field(i)

		key := field.Tag.Get(opt.Tag)
		if field.Name == "BaseModel" && value.Kind() == reflect.Struct {
			// firestore 处理struct
			// key = field.Tag.Get(opt.structTag)
			// if key == "" {
			// 	continue
			// }
			// if value.Kind() == reflect.Struct {
			// Recursively process inline struct
			processStruct(value, value.Type(), opt, upData, containUpdateField)
			continue
			// }
		}

		// Check for inline tag
		isInline := false
		if pos := strings.Index(key, ","); pos != -1 {
			isInline = strings.Contains(key[pos:], "inline")
			key = key[0:pos]
		}

		key = strings.TrimSpace(key)
		if !isInline && (key == "" || key == "-") {
			if field.Name == "Id" {
				upData.ID = value.String()
			}
			continue
		} else if key == "id" || key == "_id" {
			upData.ID = value.String()
			continue
		}

		if isInline && value.Kind() == reflect.Struct {
			// Recursively process inline struct
			processStruct(value, value.Type(), opt, upData, containUpdateField)
			continue
		}

		vi := value.Interface()
		if opt.Filter != nil && !opt.Filter(key, vi) {
			continue
		}

		isZero := reflect.DeepEqual(vi, reflect.Zero(value.Type()).Interface())
		if (isZero && opt.EmptyIgnore != nil && !opt.EmptyIgnore(key, vi)) || !isZero {
			upData.Updates = append(upData.Updates, Update{Field: key, Value: vi})
			if key == UpdateField {
				*containUpdateField = true
			}
		}
	}
}
