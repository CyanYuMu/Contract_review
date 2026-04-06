package su_error

import (
	"fmt"
	"google.golang.org/grpc/status"
)

type SUError struct {
	Code int32
	Msg  string
}

func (s *SUError) Error() string {
	return fmt.Sprintf("code = %d desc = %s", s.Code, s.Msg)
}

func New(code int32, msg string) error {
	return &SUError{
		Code: code,
		Msg:  msg,
	}
}

func NewF(code int32, format string, args ...interface{}) error {
	return New(code, fmt.Sprintf(format, args...))
}

func FromError(code int32, err error) error {
	return New(code, err.Error())
}

func Parse(err error) (c int32, msg string) {
	if e, ok := err.(*SUError); ok {
		return e.Code, e.Msg
	} else {
		s, ok := status.FromError(err)
		if !ok {
			return 0, err.Error()
		}
		c = int32(s.Code())
		msg = s.String()
		return c, msg
	}
}

//
//func IgnoreNoRecord(err error) error {
//	if err == gorm.ErrRecordNotFound {
//		return nil
//	} else if err == redis.Nil {
//		return nil
//	}
//
//	return err
//}
//
//// EmptyError
//// @description 判断data是否为空, 如果为空则按照 err, msg 的优先级返回对应的错误
//func EmptyError(data interface{}, err error, module string, msg string, ignoreNoRecord bool) error {
//	if ignoreNoRecord {
//		err = IgnoreNoRecord(err)
//	}
//
//	if err != nil {
//		if module != "" {
//			return fmt.Errorf("module:%s err:%s", module, err.Error())
//		} else {
//			return err
//		}
//	}
//
//	if validator.IsNilInterface(data) {
//		if module != "" {
//			return fmt.Errorf("moduel:%s err:%s", module, msg)
//		} else {
//			return errors.New(msg)
//		}
//	}
//
//	return nil
//}
//
//type Entry struct {
//	Key   string
//	Value interface{}
//}
//
//func Sprintf(err error, args ...interface{}) error {
//	return fmt.Errorf(err.Error(), args...)
//}
//
//func Wrap(err error, entries ...*Entry) error {
//	if len(entries) == 0 {
//		return err
//	}
//	msg := strings.Builder{}
//	msg.WriteString("[param] ")
//	for i, _ := range entries {
//		if i > 0 {
//			msg.WriteString("&")
//		}
//		if entries[i].Key != "" {
//			msg.WriteString(entries[i].Key)
//			msg.WriteString("=")
//		}
//
//		// 常见数据类型推断处理
//		if entries[i].Value == nil {
//			msg.WriteString("nil")
//			continue
//		} else if s, ok := entries[i].Value.(string); ok {
//			msg.WriteString(s)
//			continue
//		} else if b, ok := entries[i].Value.([]byte); ok {
//			msg.Write(b)
//			continue
//		} else if bl, ok := entries[i].Value.(bool); ok {
//			if bl {
//				msg.WriteString("true")
//			} else {
//				msg.WriteString("false")
//			}
//		} else {
//			tpe := reflect.TypeOf(entries[i].Value)
//			if tpe.Kind() == reflect.Ptr {
//				tpe = tpe.Elem()
//			}
//			if tpe.Kind() == reflect.Ptr {
//				tpe = tpe.Elem()
//			}
//			switch tpe.Kind() {
//			case reflect.Struct, reflect.Map, reflect.Slice:
//				curV, _ := jsoniter.MarshalToString(entries[i].Value)
//				msg.WriteString(curV)
//			default:
//				msg.WriteString(fmt.Sprintf("%+v", entries[i].Value))
//			}
//		}
//	}
//
//	return errors.New(err.Error() + " " + msg.String())
//}
