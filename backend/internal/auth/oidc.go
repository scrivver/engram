package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/chunhou/engram/internal/config"
)

type contextKey string

const (
	ctxUsername contextKey = "username"
	ctxRole     contextKey = "role"
)

// OIDCAuthenticator validates Bearer tokens by calling the OIDC provider's
// userinfo endpoint. Port of the Reliquary implementation — kept intentionally
// similar so the two services behave identically.
type OIDCAuthenticator struct {
	userinfoEndpoint string
	usernameClaim    string
	cache            sync.Map // token → *cachedUser
}

type cachedUser struct {
	username  string
	expiresAt time.Time
}

const userinfoCacheTTL = 5 * time.Minute

func NewOIDCAuthenticator(ctx context.Context, cfg *config.Config) (*OIDCAuthenticator, error) {
	if cfg.OIDCIssuerURL == "" {
		return nil, fmt.Errorf("OIDC_ISSUER_URL is required when AUTH_MODE=oidc")
	}

	provider, err := oidc.NewProvider(ctx, cfg.OIDCIssuerURL)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}

	var claims struct {
		UserinfoEndpoint string `json:"userinfo_endpoint"`
	}
	if err := provider.Claims(&claims); err != nil {
		return nil, fmt.Errorf("oidc discovery claims: %w", err)
	}
	if claims.UserinfoEndpoint == "" {
		return nil, fmt.Errorf("oidc provider has no userinfo_endpoint")
	}

	slog.Info("oidc provider discovered",
		"issuer", cfg.OIDCIssuerURL,
		"userinfo_endpoint", claims.UserinfoEndpoint,
		"username_claim", cfg.OIDCUsernameClaim,
	)

	return &OIDCAuthenticator{
		userinfoEndpoint: claims.UserinfoEndpoint,
		usernameClaim:    cfg.OIDCUsernameClaim,
	}, nil
}

func (o *OIDCAuthenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, err := o.AuthenticateRequest(r)
		if err != nil {
			slog.Warn("oidc token validation failed", "error", err)
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), ctxUsername, username)
		ctx = context.WithValue(ctx, ctxRole, RoleUser)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// AuthenticateRequest validates the Bearer token via the OIDC userinfo endpoint
// and returns the username.
func (o *OIDCAuthenticator) AuthenticateRequest(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", fmt.Errorf("missing authorization header")
	}

	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenStr == authHeader {
		return "", fmt.Errorf("invalid authorization format")
	}

	return o.resolveUsername(r.Context(), tokenStr)
}

func (o *OIDCAuthenticator) resolveUsername(ctx context.Context, token string) (string, error) {
	if cached, ok := o.cache.Load(token); ok {
		entry := cached.(*cachedUser)
		if time.Now().Before(entry.expiresAt) {
			return entry.username, nil
		}
		o.cache.Delete(token)
	}

	username, err := o.fetchUserinfo(ctx, token)
	if err != nil {
		return "", err
	}

	o.cache.Store(token, &cachedUser{
		username:  username,
		expiresAt: time.Now().Add(userinfoCacheTTL),
	})

	return username, nil
}

func (o *OIDCAuthenticator) fetchUserinfo(ctx context.Context, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", o.userinfoEndpoint, nil)
	if err != nil {
		return "", fmt.Errorf("create userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("userinfo request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("userinfo returned %d: %s", resp.StatusCode, body)
	}

	var claims map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&claims); err != nil {
		return "", fmt.Errorf("parse userinfo response: %w", err)
	}

	username, ok := claims[o.usernameClaim].(string)
	if !ok || username == "" {
		return "", fmt.Errorf("userinfo missing claim %q", o.usernameClaim)
	}

	return username, nil
}

// UsernameFromContext returns the authenticated username, or "" if unauthenticated.
func UsernameFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxUsername).(string)
	return v
}

// RoleFromContext returns the authenticated user's role, or "" if unauthenticated.
func RoleFromContext(ctx context.Context) Role {
	v, _ := ctx.Value(ctxRole).(Role)
	return v
}
