package user

import (
	"context"
	"contract_review/app/internal/global"
	"contract_review/app/internal/middleware/redis"
	"contract_review/app/pkg/auth"
	"contract_review/app/pkg/consts/errno"
	"fmt"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"time"
)

type UserService struct {
	userrepo *UserRepo
	cache    *redis.RedisClient
}

func NewUserService(userrepo *UserRepo, cache *redis.RedisClient) *UserService {
	return &UserService{userrepo: userrepo, cache: cache}
}

// CreateUser 创建用户
func (us *UserService) CreateUser(ctx context.Context, user *User) error {
	exists, err := us.userrepo.ExistsAccount(ctx, user.Account)
	if err != nil {
		global.Log.Error("ExistsAccountCRUD error", zap.Error(err))
		return err
	}
	if exists {
		return errno.ErrAccountAlreadyExists
	}

	Hashpassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user.Password = string(Hashpassword)
	if err := us.userrepo.CreateUser(ctx, user); err != nil {
		return err
	}
	return nil
}

// Login 用户登录
func (us *UserService) Login(ctx context.Context, account string, password string) (accessToken string, refreshToken string, err error) {
	dbuser, err := us.userrepo.GetUserByAccount(ctx, account)
	if err != nil {
		global.Log.Error("GetUserByAccountCRUD error")
		return "", "", err
	}
	if dbuser.Password == "" {
		return "", "", err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(dbuser.Password), []byte(password)); err != nil {
		global.Log.Error("CompareHashAndPassword error")
		return "", "", err
	}
	//generate Token
	accessToken, err = auth.GenerateAccessToken(dbuser.Account, dbuser.Username)
	if err != nil {
		global.Log.Error("GenerateAccessToken error")
		return "", "", err
	}
	refreshToken, err = auth.GenerateRefreshToken(dbuser.Account)
	if err != nil {
		global.Log.Error("GenerateRefreshToken error")
		return "", "", err
	}
	ttl := time.Duration(global.Config.JWT.RefreshTokenExpireDays) * 24 * time.Hour
	err = us.cache.SetBytes(ctx, fmt.Sprintf("refreshtoken:%s", dbuser.Account), []byte(refreshToken), ttl)
	if err != nil {
		global.Log.Error("SetBytes error")
		return "", "", err
	}
	return accessToken, refreshToken, nil
}

// Logout 用户登出
func (us *UserService) Logout(ctx context.Context, account string) error {
	key := fmt.Sprintf("refreshtoken:%s", account)
	if err := us.cache.Del(ctx, key); err != nil {
		global.Log.Error("cache.Del error")
		return err
	}
	return nil
}

// RefreshAccessToken 使用刷新令牌获取新的访问令牌
func (us *UserService) RefreshAccessToken(ctx context.Context, refreshToken string) (newAccessToken string, err error) {
	claims, err := auth.ParseRefreshToken(refreshToken)
	if err != nil {
		global.Log.Error("ParseRefreshToken error", zap.Error(err))
		return "", err
	}

	// 验证 refresh token 是否在 Redis 中存在
	key := fmt.Sprintf("refreshtoken:%s", claims.Account)
	val, err := us.cache.GetString(ctx, key)
	if err != nil {
		global.Log.Error("cache.Get error", zap.Error(err))
		return "", err
	}
	if val != refreshToken {
		global.Log.Error("refresh token mismatch")
		return "", err
	}

	// 根据账号获取用户信息
	user, err := us.userrepo.GetUserByAccount(ctx, claims.Account)
	if err != nil {
		global.Log.Error("GetUserByAccount error", zap.Error(err))
		return "", err
	}

	// 生成新的 access token
	newAccessToken, err = auth.GenerateAccessToken(user.Account, user.Username)
	if err != nil {
		global.Log.Error("GenerateAccessToken error", zap.Error(err))
		return "", err
	}

	return newAccessToken, nil
}

// DeleteUser 删除用户
func (us *UserService) DeleteUser(ctx context.Context, userID uint) error {
	user, err := us.userrepo.GetUserByID(ctx, userID)
	if err != nil {
		global.Log.Error("GetUserByID error", zap.Error(err))
		return err
	}

	// 删除 Redis 中的刷新令牌
	key := fmt.Sprintf("refreshtoken:%s", user.Account)
	if err := us.cache.Del(ctx, key); err != nil {
		global.Log.Error("cache.Del error", zap.Error(err))
		// 不返回错误，继续删除用户
	}

	// 删除用户
	if err := us.userrepo.DeleteUser(ctx, userID); err != nil {
		return err
	}
	return nil
}

// UpdateUser 更新用户信息
func (us *UserService) UpdateUser(ctx context.Context, userID uint, updates *User) error {
	// 验证用户是否存在
	user, err := us.userrepo.GetUserByID(ctx, userID)
	if err != nil {
		global.Log.Error("GetUserByID error", zap.Error(err))
		return err
	}

	// 如果更新用户名，检查是否已被占用
	if updates.Username != "" && updates.Username != user.Username {
		existingUser, err := us.userrepo.GetUserByUsername(ctx, updates.Username)
		if err == nil && existingUser != nil && existingUser.ID != userID {
			return errno.ErrUsernameTaken
		}
	}

	// 更新用户信息
	updateData := map[string]interface{}{
		"username":   updates.Username,
		"department": updates.Department,
		"role":       updates.Role,
	}

	if err := us.userrepo.UpdateUserByID(ctx, userID, updateData); err != nil {
		return err
	}
	return nil
}

// GetUserInfo 获取用户信息
func (us *UserService) GetUserInfo(ctx context.Context, userID uint) (*User, error) {
	user, err := us.userrepo.GetUserByID(ctx, userID)
	if err != nil {
		global.Log.Error("GetUserByID error", zap.Error(err))
		return nil, err
	}
	return user, nil
}

func (us *UserService) GetUserInfoByAccount(ctx context.Context, account string) (*User, error) {
	user, err := us.userrepo.GetUserByAccount(ctx, account)
	if err != nil {
		global.Log.Error("GetUserByAccount error", zap.Error(err))
		return nil, err
	}
	return user, nil
}

// ChangePassword 修改密码
func (us *UserService) ChangePassword(ctx context.Context, userID uint, oldPassword string, newPassword string) error {
	user, err := us.userrepo.GetUserByID(ctx, userID)
	if err != nil {
		global.Log.Error("GetUserByID error", zap.Error(err))
		return err
	}

	// 验证旧密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(oldPassword)); err != nil {
		global.Log.Error("CompareHashAndPassword error")
		return err
	}

	// 生成新密码哈希
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		global.Log.Error("GenerateFromPassword error", zap.Error(err))
		return err
	}

	// 更新密码
	if err := us.userrepo.UpdatePassword(ctx, userID, string(hashedPassword)); err != nil {
		return err
	}
	return nil
}
