package auth

import (
	"contract_review/app/internal/global"
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"time"
)

type AccessClaims struct {
	Account  string   `json:"account"`
	Username string   `json:"username"`
	Roles    []string `json:"roles"`
	jwt.RegisteredClaims
}

type RefreshClaims struct {
	Account string `json:"account"`
	jwt.RegisteredClaims
}

func GenerateAccessToken(account string, username string) (string, error) {
	cfg := global.Config.JWT
	now := time.Now()

	claims := AccessClaims{
		Account:  account,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(
				now.Add(time.Duration(cfg.AccessTokenExpireMin) * time.Minute),
			),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(cfg.SecretKey))
}

func GenerateRefreshToken(account string) (string, error) {
	cfg := global.Config.JWT
	now := time.Now()

	claims := RefreshClaims{
		Account: account,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(
				now.Add(time.Duration(cfg.RefreshTokenExpireDays) * 24 * time.Hour),
			),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(cfg.RefreshSecretKey))
}

func ValidateAccessToken(tokenString string) error {
	_, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if token.Method == nil ||
			token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(global.Config.JWT.SecretKey), nil
	})
	return err
}

func ValidateRefreshToken(tokenString string) error {
	_, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if token.Method == nil ||
			token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(global.Config.JWT.RefreshSecretKey), nil
	})
	return err
}

func ParseAccessToken(tokenString string) (*AccessClaims, error) {
	token, err := jwt.ParseWithClaims(
		tokenString,
		&AccessClaims{},
		func(token *jwt.Token) (interface{}, error) {
			if token.Method == nil ||
				token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(global.Config.JWT.SecretKey), nil
		},
	)
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*AccessClaims)
	if !ok || !token.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}
	return claims, nil
}

func ParseRefreshToken(tokenString string) (*RefreshClaims, error) {
	token, err := jwt.ParseWithClaims(
		tokenString,
		&RefreshClaims{},
		func(token *jwt.Token) (interface{}, error) {
			if token.Method == nil ||
				token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(global.Config.JWT.RefreshSecretKey), nil
		},
	)
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*RefreshClaims)
	if !ok || !token.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}
	return claims, nil
}
