package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestMultiAuthenticatorJWTOnly(t *testing.T) {
	secret := "test-secret"
	jwtAuth, err := NewJWTAuth(secret)
	if err != nil {
		t.Fatalf("new jwt auth: %v", err)
	}

	multi := NewMultiAuthenticator(jwtAuth, nil)

	validToken, err := makeTestJWT(secret, "alice", "admin")
	if err != nil {
		t.Fatalf("make jwt: %v", err)
	}

	t.Run("valid jwt", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/files", nil)
		req.Header.Set("Authorization", "Bearer "+validToken)
		rec := httptest.NewRecorder()

		var gotUsername string
		var gotRole Role
		handler := multi.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotUsername = UsernameFromContext(r.Context())
			gotRole = RoleFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		}))
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if gotUsername != "alice" {
			t.Fatalf("expected username alice, got %s", gotUsername)
		}
		if gotRole != RoleAdmin {
			t.Fatalf("expected role admin, got %s", gotRole)
		}
	})

	t.Run("invalid jwt", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/files", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		rec := httptest.NewRecorder()

		handler := multi.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("handler should not be called")
		}))
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})
}

func TestMultiAuthenticatorOIDCFallback(t *testing.T) {
	secret := "test-secret"
	jwtAuth, err := NewJWTAuth(secret)
	if err != nil {
		t.Fatalf("new jwt auth: %v", err)
	}

	oidcAuth := newTestOIDCAuthenticator(t, "bob")
	multi := NewMultiAuthenticator(jwtAuth, oidcAuth)

	oidcToken := "oidc-access-token"

	t.Run("valid jwt takes precedence over oidc", func(t *testing.T) {
		validToken, err := makeTestJWT(secret, "alice", "admin")
		if err != nil {
			t.Fatalf("make jwt: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/api/files", nil)
		req.Header.Set("Authorization", "Bearer "+validToken)
		rec := httptest.NewRecorder()

		var gotUsername string
		handler := multi.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotUsername = UsernameFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		}))
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if gotUsername != "alice" {
			t.Fatalf("expected jwt username alice, got %s", gotUsername)
		}
	})

	t.Run("invalid jwt falls back to oidc", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/files", nil)
		req.Header.Set("Authorization", "Bearer "+oidcToken)
		rec := httptest.NewRecorder()

		var gotUsername string
		var gotRole Role
		handler := multi.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotUsername = UsernameFromContext(r.Context())
			gotRole = RoleFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		}))
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if gotUsername != "bob" {
			t.Fatalf("expected oidc username bob, got %s", gotUsername)
		}
		if gotRole != RoleUser {
			t.Fatalf("expected role user for oidc, got %s", gotRole)
		}
	})

	t.Run("neither provider accepts token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/files", nil)
		req.Header.Set("Authorization", "Bearer bad-token")
		rec := httptest.NewRecorder()

		handler := multi.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("handler should not be called")
		}))
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})
}

func TestMultiAuthenticatorNoProviders(t *testing.T) {
	multi := NewMultiAuthenticator(nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/files", nil)
	req.Header.Set("Authorization", "Bearer any-token")
	rec := httptest.NewRecorder()

	handler := multi.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func makeTestJWT(secret, username, role string) (string, error) {
	claims := &Claims{
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func newTestOIDCAuthenticator(t *testing.T, username string) *OIDCAuthenticator {
	t.Helper()

	validToken := "oidc-access-token"

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"userinfo_endpoint": server.URL + "/userinfo",
		})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+validToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"preferred_username": username,
		})
	})

	// Discover the provider to populate the authenticator.
	// We bypass NewOIDCAuthenticator to avoid the heavy go-oidc discovery
	// and construct the struct directly with the test endpoints.
	return &OIDCAuthenticator{
		userinfoEndpoint: server.URL + "/userinfo",
		usernameClaim:    "preferred_username",
	}
}
