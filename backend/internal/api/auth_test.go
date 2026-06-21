package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chunhou/engram/internal/config"
)

func TestAuthConfigOIDC(t *testing.T) {
	cfg := &config.Config{
		AuthMode:          "oidc",
		OIDCIssuerURL:     "https://idp.example.test/application/o/mind-palace/",
		OIDCClientID:      "mind-palace",
		OIDCUsernameClaim: "preferred_username",
		OIDCRedirectURI:   "http://localhost:2080/callback",
	}
	srv := NewServer(nil, WithConfig(cfg))

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/config", nil)
	srv.Routes(nil).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.Code)
	}
	var got authConfigResponse
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !got.OIDC.Enabled || got.OIDC.IssuerURL != cfg.OIDCIssuerURL || got.OIDC.ClientID != cfg.OIDCClientID {
		t.Fatalf("unexpected oidc config: %+v", got.OIDC)
	}
	if got.None.Enabled {
		t.Fatalf("none auth should be disabled when oidc is enabled")
	}
}

func TestAuthConfigNone(t *testing.T) {
	srv := NewServer(nil, WithConfig(&config.Config{AuthMode: "none"}))

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/config", nil)
	srv.Routes(nil).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.Code)
	}
	var got authConfigResponse
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !got.None.Enabled || got.OIDC.Enabled {
		t.Fatalf("unexpected auth config: %+v", got)
	}
}

func TestOIDCHelpersRequireOIDC(t *testing.T) {
	srv := NewServer(nil, WithConfig(&config.Config{AuthMode: "none"}))

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/auth/oidc/discovery"},
		{method: http.MethodPost, path: "/api/auth/oidc/token", body: `{}`},
	} {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
		srv.Routes(nil).ServeHTTP(res, req)
		if res.Code != http.StatusBadRequest {
			t.Fatalf("%s %s status = %d, want 400", tc.method, tc.path, res.Code)
		}
	}
}
