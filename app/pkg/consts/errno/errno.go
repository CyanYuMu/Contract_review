package errno

import "errors"

// Error Variables
var ErrAccountAlreadyExists = errors.New("account already exists")
var ErrUsernameTaken = errors.New("username taken")

// Error Codes
const (
	Success = 200
)

const (
	ErrUsernameTakenCode = 50000 + iota
	ErrNewUsernameRequired
	ErrBindUserRequest
	ErrInternal
	ErrUnauthorized
	ErrForbidden
	ErrNotFound
	ErrUserNotFound
	ErrInvalidToken
	ErrTokenExpired
	ErrInvalidRefreshToken
	ErrInvalidCredentials
)

// Error Messages Mapping
var Msg = map[int]string{
	Success:                "success",
	ErrUsernameTakenCode:   "username taken",
	ErrNewUsernameRequired: "new username required",
	ErrBindUserRequest:     "bind user request err",
	ErrInternal:            "internal service error",
	ErrUnauthorized:        "unauthorized",
	ErrForbidden:           "forbidden",
	ErrNotFound:            "not found",
	ErrUserNotFound:        "user not found",
	ErrInvalidToken:        "invalid token",
	ErrTokenExpired:        "token expired",
	ErrInvalidRefreshToken: "invalid refresh token",
	ErrInvalidCredentials:  "invalid credentials",
}

// GetMsg 获取错误信息
func GetMsg(code int) string {
	if msg, ok := Msg[code]; ok {
		return msg
	}
	return "unknown error"
}
