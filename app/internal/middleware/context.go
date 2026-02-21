package middleware

import "github.com/cloudwego/hertz/pkg/app"

func GetCurrentUserID(c *app.RequestContext) string {
	v, ok := c.Get("account")
	if !ok {
		return ""
	}
	account, ok := v.(string)
	if ok {
		return account
	}
	return ""
}
