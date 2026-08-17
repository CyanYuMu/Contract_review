package middleware

import (
	"context"
	"net/http"
	"strings"

	"contract_review/app/internal/global"
	"contract_review/app/pkg/response"

	"github.com/cloudwego/hertz/pkg/app"
)

const (
	SystemRoleMember = "member"
	SystemRoleAdmin  = "admin"
	SystemRoleOwner  = "owner"
)

// RequireSystemRole performs an authoritative database lookup on every
// privileged request. JWT roles are only UI hints: querying the database here
// makes a role demotion take effect immediately.
func RequireSystemRole(allowedRoles ...string) app.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedRoles))
	for _, role := range allowedRoles {
		role = strings.ToLower(strings.TrimSpace(role))
		if role != "" {
			allowed[role] = struct{}{}
		}
	}

	return func(ctx context.Context, c *app.RequestContext) {
		account := GetCurrentUserID(c)
		if account == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, response.Unauthorized())
			return
		}
		if global.DB == nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, response.FailWithMsg("权限服务暂不可用"))
			return
		}

		var userRole struct {
			SystemRole string `gorm:"column:system_role"`
		}
		err := global.DB.WithContext(ctx).
			Table("users").
			Select("system_role").
			Where("account = ?", account).
			First(&userRole).Error
		if err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, response.FailWithMsg("无权执行该操作"))
			return
		}

		role := strings.ToLower(strings.TrimSpace(userRole.SystemRole))
		if _, ok := allowed[role]; !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, response.FailWithMsg("无权执行该操作"))
			return
		}

		c.Set("systemRole", role)
		c.Next(ctx)
	}
}
