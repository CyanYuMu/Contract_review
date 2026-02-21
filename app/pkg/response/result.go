package response

import "encoding/json"

type Result struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data,omitempty"`
}

type ErrResult struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func (r *Result) Error() string {
	b, _ := json.Marshal(r)
	return string(b)
}

// OK 成功响应（带数据）
func OK(data interface{}) *Result {
	return &Result{Code: 200, Msg: "Success", Data: data}
}

// Ok 成功响应（无数据）
func Ok() *Result {
	return &Result{Code: 200, Msg: "Success"}
}

// OkWithData 成功响应（带数据）
func OkWithData(data interface{}) *Result {
	return &Result{Code: 200, Msg: "Success", Data: data}
}

// OkWithMsg 成功响应（带消息）
func OkWithMsg(msg string) *Result {
	return &Result{Code: 200, Msg: msg}
}

// Fail 失败响应（默认）
func Fail() *ErrResult {
	return &ErrResult{Code: 400, Msg: "请求失败"}
}

// FailWithCode 失败响应（带错误码和消息）
func FailWithCode(code int, msg string) *ErrResult {
	return &ErrResult{Code: code, Msg: msg}
}

// FailWithMsg 失败响应（带消息）
func FailWithMsg(msg string) *ErrResult {
	return &ErrResult{Code: 400, Msg: msg}
}

// ServerError 服务器错误响应
func ServerError() *ErrResult {
	return &ErrResult{Code: 500, Msg: "服务器内部错误"}
}

// Unauthorized 未授权响应
func Unauthorized() *ErrResult {
	return &ErrResult{Code: 401, Msg: "未授权"}
}

// NotFound 未找到响应
func NotFound() *ErrResult {
	return &ErrResult{Code: 404, Msg: "资源不存在"}
}
