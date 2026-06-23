package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// Role mirrors the Reliquary role model for JWT claim compatibility.
type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

// JWTAuth validates Bearer tokens signed with a shared HMAC secret.
// It is intended for deployments where Reliquary issues the JWT and Engram
// consumes it.
type JWTAuth struct {
	secret []byte
}

// Claims matches the Reliquary JWT shape.
type Claims struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// NewJWTAuth creates a JWT validator using the given shared secret.
func NewJWTAuth(secret string) (*JWTAuth, error) {
	if secret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}
	return &JWTAuth{secret: []byte(secret)}, nil
}

// Middleware validates the JWT and injects username/role into the context.
func (j *JWTAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, role, err := j.AuthenticateRequest(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), ctxUsername, username)
		ctx = context.WithValue(ctx, ctxRole, role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// AuthenticateRequest validates the Bearer token and returns the username and role.
func (j *JWTAuth) AuthenticateRequest(r *http.Request) (string, Role, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", "", fmt.Errorf("missing authorization header")
	}
	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenStr == authHeader {
		return "", "", fmt.Errorf("invalid authorization format")
	}
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		return j.secret, nil
	})
	if err != nil || !token.Valid {
		return "", "", fmt.Errorf("invalid token")
	}
	return claims.Username, Role(claims.Role), nil
}
