package jwt_auth

import (
	"context"
	"contract_review/app/internal/global"
	"contract_review/app/pkg/auth"
	"errors"
	"fmt"
	"github.com/cloudwego/hertz/pkg/app"
	"net/http"
	"strings"
	"time"
)

func AccessJWTAuth() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		authHeader := string(c.GetHeader("Authorization"))
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, map[string]string{
				"error": "missing authorization header",
			})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(401, map[string]string{
				"error": "invalid authorization header",
			})
			return
		}

		accessToken := parts[1]
		//校验JWT本身（签名+exp）
		claims, err := auth.ParseAccessToken(accessToken)
		if err != nil {
			c.AbortWithStatusJSON(401, map[string]string{
				"error": "invalid or expired access token",
			})
			return
		}
		//检验当前是否有效的AccessToken
		if err := checkAccessToken(ctx, claims, accessToken); err != nil {
			c.AbortWithStatusJSON(401, map[string]string{
				"error": "invalid or expired access token",
			})
			return
		}
		c.Set("account", claims.Account)
		c.Set("accountID", claims.Account)
		c.Set("username", claims.Username)
		c.Set("roles", claims.Roles)

		c.Next(ctx)
	}
}

func checkAccessToken(
	ctx context.Context,
	claims *auth.AccessClaims,
	accessToken string,
) error {

	uid := claims.Account
	redisKey := fmt.Sprintf("access:%s", uid)

	//Redis 优先
	if global.Redis != nil {
		rctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		defer cancel()

		val, err := global.Redis.GetString(rctx, redisKey)
		if err == nil {
			if val != accessToken {
				return errors.New("access token has been revoked")
			}
			return nil
		}
	}

	// Token 本身验证已在 ParseAccessToken 中进行，直接返回成功
	return nil
}

func checkRefreshToken(
	ctx context.Context,
	account string,
	refreshToken string,
) error {
	redisKey := fmt.Sprintf("refreshtoken:%s", account)

	//Redis 优先
	if global.Redis != nil {
		rctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		defer cancel()

		val, err := global.Redis.GetString(rctx, redisKey)
		if err == nil {
			if val != refreshToken {
				return errors.New("refresh token has been revoked")
			}
			return nil
		}
	}

	return nil
}

func RefreshTokenHandler(ctx context.Context, c *app.RequestContext) {

	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, map[string]string{"error": "invalid request"})
		return
	}

	claims, err := auth.ParseRefreshToken(req.RefreshToken)
	if err != nil {
		c.JSON(401, map[string]string{"error": "invalid refresh token"})
		return
	}

	// 校验 Redis / DB refresh token
	if err := checkRefreshToken(ctx, claims.Account, req.RefreshToken); err != nil {
		c.JSON(401, map[string]string{"error": err.Error()})
		return
	}

	// 生成新 Access Token（可选：Rotation）
	newAccess, _ := auth.GenerateAccessToken(
		"",
		claims.Account,
	)

	c.JSON(200, map[string]string{
		"access_token": newAccess,
	})
}
