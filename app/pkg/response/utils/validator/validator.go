package validator

import (
	"encoding/base64"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/validator/lib/go-playground/validator"
	"regexp"
)

// AppKey校验规则
// ZGV2ZWxvcDpDSmpCNnpEVWFhOjE2NDMzNDQ0MDI=
// develop:CJjB6zDUaa:1643344402
// base64decode
var appKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9]+:[a-zA-Z0-9]+:\d{10}$`)

// 订单ID校验规则
// 生成方式："XH_" + xid.New().String()
//示例：XH_c0vlnupfrd4ggl5t4aug
var xhOrderPattern = regexp.MustCompile(`XH_[a-zA-Z0-9]+`)

type SUValidator struct {
	*validator.Validate
}

// NewSUValidator
// @description new一个validator
// 基于 https://github.com/go-playground/validator 进行二次开发
func NewSUValidator() *SUValidator {
	v := validator.New()
	_ = v.RegisterValidation("appKey", func(fl validator.FieldLevel) bool {

		s, err := base64.StdEncoding.DecodeString(fl.Field().String())
		if err != nil {
			return false
		}
		if appKeyPattern.Match(s) {
			return true
		}
		return false
	})
	_ = v.RegisterValidation("order", func(fl validator.FieldLevel) bool {
		if xhOrderPattern.MatchString(fl.Field().String()) {
			return true
		}
		return false
	})

	return &SUValidator{v}
}
