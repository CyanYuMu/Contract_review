package error

import (
	"fmt"
	"strconv"
	"strings"
)

// SUError
// @Description:
// @Deprecate 调整到 su_error 包
type SUError struct {
	Code int32
	Msg  string
}

func (s *SUError) Error() string {
	return fmt.Sprintf("%d$%s", s.Code, s.Msg)
}

func NewSUError(code int32, msg string) *SUError {
	return &SUError{
		Code: code,
		Msg:  msg,
	}
}

func NewWithError(code int32, err error) *SUError {
	return &SUError{
		Code: code,
		Msg:  err.Error(),
	}
}

func Parse(err error) (code int32, msg string) {
	s := err.Error()
	tmp := strings.SplitN(s, "$", 2)
	if len(tmp) == 2 {
		c, _ := strconv.Atoi(tmp[0])
		return int32(c), tmp[1]
	} else {
		return 0, s
	}
}

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
// EmptyError
// @description 判断data是否为空, 如果为空则按照 err, msg 的优先级返回对应的错误
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
