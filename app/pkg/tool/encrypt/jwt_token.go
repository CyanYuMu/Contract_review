package encrypt

import (
	"crypto/rsa"
	"errors"
	"github.com/golang-jwt/jwt/v4"
	"time"
)

type Claims struct {
	jwt.RegisteredClaims
	// Payload 载荷数据
	Payload string `json:"payload"`
}

var ErrSignMethodUnSupport = errors.New("signing method error")
var ErrTokenNotValid = errors.New("token not valid")

// SignByRSA512 基于rsa 512生成token
func SignByRSA512(key *rsa.PrivateKey, claims *Claims) (string, error) {
	if claims.IssuedAt == nil {
		claims.IssuedAt = jwt.NewNumericDate(time.Now())
	}
	if claims.ExpiresAt == nil {
		claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(time.Hour * 24))
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS512, claims)
	return token.SignedString(key)
}

func ParseByRSA512(key *rsa.PublicKey, tokenStr string) (*Claims, error) {
	var claims = &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		return key, nil
	})
	if err != nil {
		return claims, err
	}
	if token.Method != jwt.SigningMethodRS512 {
		return nil, ErrSignMethodUnSupport
	}
	if !token.Valid {
		return nil, ErrTokenNotValid
	}
	return claims, nil
}
