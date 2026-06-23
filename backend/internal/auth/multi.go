package auth

import (
	"context"
	"net/http"
)

// MultiAuthenticator validates Bearer tokens against multiple providers in order.
// It tries JWT validation first (cheap, local), then falls back to OIDC validation
// (network call). This allows a single Engram deployment to accept both
// Reliquary-issued JWTs and OIDC access tokens.
type MultiAuthenticator struct {
	jwtAuth  *JWTAuth
	oidcAuth *OIDCAuthenticator
}

// NewMultiAuthenticator creates a combined authenticator. Either provider may be
// nil; if both are nil the returned middleware rejects all requests.
func NewMultiAuthenticator(jwtAuth *JWTAuth, oidcAuth *OIDCAuthenticator) *MultiAuthenticator {
	return &MultiAuthenticator{
		jwtAuth:  jwtAuth,
		oidcAuth: oidcAuth,
	}
}

// Middleware returns an http.Handler that authenticates against JWT first, then
// OIDC. The request context receives username and role when authentication
// succeeds.
func (m *MultiAuthenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, role, ok := m.authenticate(r)
		if !ok {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), ctxUsername, username)
		ctx = context.WithValue(ctx, ctxRole, role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *MultiAuthenticator) authenticate(r *http.Request) (string, Role, bool) {
	if m.jwtAuth != nil {
		username, role, err := m.jwtAuth.AuthenticateRequest(r)
		if err == nil {
			return username, role, true
		}
	}

	if m.oidcAuth != nil {
		username, err := m.oidcAuth.AuthenticateRequest(r)
		if err == nil {
			return username, RoleUser, true
		}
	}

	return "", "", false
}
