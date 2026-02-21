package user

import (
	"context"
	"contract_review/app/internal/global"
	"contract_review/app/pkg/consts/errno"

	"contract_review/app/pkg/response"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"go.uber.org/zap"
)

type UserHandler struct {
	userService *UserService
}

func NewUserHandler(userService *UserService) *UserHandler {
	return &UserHandler{userService: userService}
}

// Register 用户注册
func (h *UserHandler) Register(ctx context.Context, c *app.RequestContext) {
	var req UserRegisterRequest
	if err := c.BindAndValidate(&req); err != nil {
		global.Log.Error(errno.Msg[errno.ErrBindUserRequest], zap.Error(err))
		c.JSON(consts.StatusBadRequest, response.FailWithCode(errno.ErrBindUserRequest, errno.Msg[errno.ErrBindUserRequest]))
		return
	}
	if err := h.userService.CreateUser(ctx, &User{
		Account:  req.Account,
		Username: req.Username,
		Password: req.Password,
	}); err != nil {
		global.Log.Error("CreateUser error", zap.Error(err))
		c.JSON(consts.StatusInternalServerError, response.FailWithCode(errno.ErrInternal, errno.Msg[errno.ErrInternal]))
		return
	}
	resp := UserRegisterResponse{
		User: UserProfile{
			Account:  req.Account,
			Username: req.Username,
		},
	}
	c.JSON(consts.StatusOK, response.OK(resp))
}

// Login 用户登录
func (h *UserHandler) Login(ctx context.Context, c *app.RequestContext) {
	var req LoginUserRequest
	if err := c.BindAndValidate(&req); err != nil {
		global.Log.Error(errno.Msg[errno.ErrBindUserRequest], zap.Error(err))
		c.JSON(consts.StatusBadRequest, response.FailWithCode(errno.ErrBindUserRequest, errno.Msg[errno.ErrBindUserRequest]))
		return
	}

	accessToken, refreshToken, err := h.userService.Login(ctx, req.Account, req.Password)
	if err != nil {
		global.Log.Error("Login error", zap.Error(err))
		c.JSON(consts.StatusUnauthorized, response.FailWithCode(errno.ErrInvalidCredentials, "Invalid account or password"))
		return
	}

	// 获取用户信息
	user, err := h.userService.userrepo.GetUserByAccount(ctx, req.Account)
	if err != nil {
		global.Log.Error("GetUserByAccount error", zap.Error(err))
		c.JSON(consts.StatusInternalServerError, response.FailWithCode(errno.ErrInternal, errno.Msg[errno.ErrInternal]))
		return
	}

	resp := LoginUserResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(global.Config.JWT.AccessTokenExpireMin * 60),
		User: UserProfile{
			Id:        int64(user.ID),
			Account:   user.Account,
			Username:  user.Username,
			CreatedAt: user.CreatedAt.Unix(),
		},
	}
	c.JSON(consts.StatusOK, response.OK(resp))
}

// Logout 用户登出
func (h *UserHandler) Logout(ctx context.Context, c *app.RequestContext) {
	// 从 context 中获取用户信息
	userIDInterface, _ := c.Get("accountID")
	usernameInterface, _ := c.Get("username")

	_, ok := userIDInterface.(uint)
	if !ok {
		global.Log.Error("Invalid userID type")
		c.JSON(consts.StatusBadRequest, response.FailWithCode(errno.ErrInternal, "Invalid user info"))
		return
	}

	username, ok := usernameInterface.(string)
	if !ok {
		global.Log.Error("Invalid username type")
		c.JSON(consts.StatusBadRequest, response.FailWithCode(errno.ErrInternal, "Invalid user info"))
		return
	}

	err := h.userService.Logout(ctx, username)
	if err != nil {
		global.Log.Error("Logout error", zap.Error(err))
		c.JSON(consts.StatusInternalServerError, response.FailWithCode(errno.ErrInternal, errno.Msg[errno.ErrInternal]))
		return
	}

	c.JSON(consts.StatusOK, response.OK(map[string]string{"message": "Logout successful"}))
}

// RefreshToken 刷新访问令牌
func (h *UserHandler) RefreshToken(ctx context.Context, c *app.RequestContext) {
	var req RefreshTokenRequest
	if err := c.BindAndValidate(&req); err != nil {
		global.Log.Error(errno.Msg[errno.ErrBindUserRequest], zap.Error(err))
		c.JSON(consts.StatusBadRequest, response.FailWithCode(errno.ErrBindUserRequest, errno.Msg[errno.ErrBindUserRequest]))
		return
	}

	newAccessToken, err := h.userService.RefreshAccessToken(ctx, req.RefreshToken)
	if err != nil {
		global.Log.Error("RefreshAccessToken error", zap.Error(err))
		c.JSON(consts.StatusUnauthorized, response.FailWithCode(errno.ErrInvalidCredentials, "Invalid refresh token"))
		return
	}

	resp := RefreshTokenResponse{
		AccessToken: newAccessToken,
		ExpiresIn:   int64(global.Config.JWT.AccessTokenExpireMin * 60),
	}
	c.JSON(consts.StatusOK, response.OK(resp))
}

// DeleteUser 删除用户
func (h *UserHandler) DeleteUser(ctx context.Context, c *app.RequestContext) {
	userIDInterface, _ := c.Get("accountID")
	userID, ok := userIDInterface.(uint)
	if !ok {
		global.Log.Error("Invalid userID type")
		c.JSON(consts.StatusBadRequest, response.FailWithCode(errno.ErrInternal, "Invalid user info"))
		return
	}

	err := h.userService.DeleteUser(ctx, userID)
	if err != nil {
		global.Log.Error("DeleteUser error", zap.Error(err))
		c.JSON(consts.StatusInternalServerError, response.FailWithCode(errno.ErrInternal, errno.Msg[errno.ErrInternal]))
		return
	}

	c.JSON(consts.StatusOK, response.OK(map[string]string{"message": "User deleted successfully"}))
}

// UpdateUser 更新用户信息
func (h *UserHandler) UpdateUser(ctx context.Context, c *app.RequestContext) {
	userIDInterface, _ := c.Get("accountID")
	userID, ok := userIDInterface.(uint)
	if !ok {
		global.Log.Error("Invalid userID type")
		c.JSON(consts.StatusBadRequest, response.FailWithCode(errno.ErrInternal, "Invalid user info"))
		return
	}

	var req UpdateUserRequest
	if err := c.BindAndValidate(&req); err != nil {
		global.Log.Error(errno.Msg[errno.ErrBindUserRequest], zap.Error(err))
		c.JSON(consts.StatusBadRequest, response.FailWithCode(errno.ErrBindUserRequest, errno.Msg[errno.ErrBindUserRequest]))
		return
	}

	updateUser := &User{
		Username:   req.Username,
		Department: req.Department,
		Role:       req.Role,
	}

	err := h.userService.UpdateUser(ctx, userID, updateUser)
	if err != nil {
		global.Log.Error("UpdateUser error", zap.Error(err))
		c.JSON(consts.StatusInternalServerError, response.FailWithCode(errno.ErrInternal, errno.Msg[errno.ErrInternal]))
		return
	}

	// 获取更新后的用户信息
	user, err := h.userService.GetUserInfo(ctx, userID)
	if err != nil {
		global.Log.Error("GetUserInfo error", zap.Error(err))
		c.JSON(consts.StatusInternalServerError, response.FailWithCode(errno.ErrInternal, errno.Msg[errno.ErrInternal]))
		return
	}

	resp := UpdateUserResponse{
		User: UserProfile{
			Id:        int64(user.ID),
			Account:   user.Account,
			Username:  user.Username,
			CreatedAt: user.CreatedAt.Unix(),
		},
	}
	c.JSON(consts.StatusOK, response.OK(resp))
}

// GetUserInfo 获取用户信息
func (h *UserHandler) GetUserInfo(ctx context.Context, c *app.RequestContext) {
	userIDInterface, _ := c.Get("accountID")
	userID, ok := userIDInterface.(uint)
	if !ok {
		global.Log.Error("Invalid userID type")
		c.JSON(consts.StatusBadRequest, response.FailWithCode(errno.ErrInternal, "Invalid user info"))
		return
	}

	user, err := h.userService.GetUserInfo(ctx, userID)
	if err != nil {
		global.Log.Error("GetUserInfo error", zap.Error(err))
		c.JSON(consts.StatusInternalServerError, response.FailWithCode(errno.ErrInternal, errno.Msg[errno.ErrInternal]))
		return
	}

	resp := GetUserResponse{
		User: UserProfile{
			Id:        int64(user.ID),
			Account:   user.Account,
			Username:  user.Username,
			CreatedAt: user.CreatedAt.Unix(),
		},
	}
	c.JSON(consts.StatusOK, response.OK(resp))
}

// ChangePassword 修改密码
func (h *UserHandler) ChangePassword(ctx context.Context, c *app.RequestContext) {
	userIDInterface, _ := c.Get("accountID")
	userID, ok := userIDInterface.(uint)
	if !ok {
		global.Log.Error("Invalid userID type")
		c.JSON(consts.StatusBadRequest, response.FailWithCode(errno.ErrInternal, "Invalid user info"))
		return
	}

	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=6"`
	}
	if err := c.BindAndValidate(&req); err != nil {
		global.Log.Error(errno.Msg[errno.ErrBindUserRequest], zap.Error(err))
		c.JSON(consts.StatusBadRequest, response.FailWithCode(errno.ErrBindUserRequest, errno.Msg[errno.ErrBindUserRequest]))
		return
	}

	err := h.userService.ChangePassword(ctx, userID, req.OldPassword, req.NewPassword)
	if err != nil {
		global.Log.Error("ChangePassword error", zap.Error(err))
		c.JSON(consts.StatusInternalServerError, response.FailWithCode(errno.ErrInternal, errno.Msg[errno.ErrInternal]))
		return
	}

	c.JSON(consts.StatusOK, response.OK(map[string]string{"message": "Password changed successfully"}))
}
