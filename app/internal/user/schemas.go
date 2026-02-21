package user

type UserProfile struct {
	Id        int64  `json:"userId"`   //id为数据库中自增
	Account   string `json:"account"`  //账号用于登录
	Username  string `json:"username"` //用户名，即昵称
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

type UserRegisterRequest struct {
	Account  string `json:"account" binding:"required,min=3,max=32"`
	Username string `json:"username" binding:"required,min=1,max=64"`
	Password string `json:"password" binding:"required,min=6"`
}

type UserRegisterResponse struct {
	User UserProfile `json:"user"`
}

type LoginUserRequest struct {
	Account  string `json:"account" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginUserResponse struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	ExpiresIn    int64       `json:"expiresIn"`
	User         UserProfile `json:"user"`
}

type LogoutRequest struct {
	AccessToken string `json:"access_token" binding:"required"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type RefreshTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expiresIn"`
}

type UpdateUserRequest struct {
	Username   string `json:"username,omitempty"`
	Department string `json:"department,omitempty"`
	Role       string `json:"role,omitempty"`
}

type UpdateUserResponse struct {
	User UserProfile `json:"user"`
}

type DeleteUserRequest struct {
	UserID uint `json:"user_id" binding:"required"`
}

type GetUserRequest struct {
	UserID uint `json:"user_id" binding:"required"`
}

type GetUserResponse struct {
	User UserProfile `json:"user"`
}
