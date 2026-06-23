package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

type authConfigResponse struct {
	OIDC authOIDCConfig `json:"oidc"`
	None authNoneConfig `json:"none"`
}

type authOIDCConfig struct {
	Enabled       bool   `json:"enabled"`
	IssuerURL     string `json:"issuer_url,omitempty"`
	ClientID      string `json:"client_id,omitempty"`
	UsernameClaim string `json:"username_claim,omitempty"`
	RedirectURI   string `json:"redirect_uri,omitempty"`
}

type authNoneConfig struct {
	Enabled bool `json:"enabled"`
}

type oidcDiscoveryResponse struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	EndSessionEndpoint    string `json:"end_session_endpoint,omitempty"`
}

type oidcTokenRequest struct {
	GrantType    string `json:"grant_type"`
	Code         string `json:"code,omitempty"`
	RedirectURI  string `json:"redirect_uri,omitempty"`
	CodeVerifier string `json:"code_verifier,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

func (s *Server) handleAuthConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg
	oidcEnabled := cfg != nil && cfg.AuthMode == "oidc"
	noneEnabled := cfg == nil || cfg.AuthMode == "" || cfg.AuthMode == "none"

	response := authConfigResponse{
		OIDC: authOIDCConfig{Enabled: oidcEnabled},
		None: authNoneConfig{Enabled: noneEnabled},
	}
	if oidcEnabled {
		issuerURL := cfg.OIDCIssuerURL
		if cfg.OIDCPublicIssuerURL != "" {
			issuerURL = cfg.OIDCPublicIssuerURL
		}
		response.OIDC.IssuerURL = issuerURL
		response.OIDC.ClientID = cfg.OIDCClientID
		response.OIDC.UsernameClaim = cfg.OIDCUsernameClaim
		response.OIDC.RedirectURI = cfg.OIDCRedirectURI
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (s *Server) handleOIDCDiscovery(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg
	if cfg == nil || cfg.AuthMode != "oidc" {
		http.Error(w, "OIDC is not enabled", http.StatusBadRequest)
		return
	}

	discovery, err := fetchOIDCDiscovery(r.Context(), cfg.OIDCIssuerURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(discovery)
}

func (s *Server) handleOIDCToken(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg
	if cfg == nil || cfg.AuthMode != "oidc" {
		http.Error(w, "OIDC is not enabled", http.StatusBadRequest)
		return
	}

	var req oidcTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	discovery, err := fetchOIDCDiscovery(r.Context(), cfg.OIDCIssuerURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	form := url.Values{}
	form.Set("grant_type", req.GrantType)
	form.Set("client_id", cfg.OIDCClientID)

	switch req.GrantType {
	case "authorization_code":
		if req.Code == "" || req.RedirectURI == "" || req.CodeVerifier == "" {
			http.Error(w, "code, redirect_uri, and code_verifier are required", http.StatusBadRequest)
			return
		}
		form.Set("code", req.Code)
		form.Set("redirect_uri", req.RedirectURI)
		form.Set("code_verifier", req.CodeVerifier)
	case "refresh_token":
		if req.RefreshToken == "" {
			http.Error(w, "refresh_token is required", http.StatusBadRequest)
			return
		}
		form.Set("refresh_token", req.RefreshToken)
	default:
		http.Error(w, "unsupported grant_type", http.StatusBadRequest)
		return
	}

	tokenReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, discovery.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		http.Error(w, "failed to create token request", http.StatusInternalServerError)
		return
	}
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	tokenResp, err := http.DefaultClient.Do(tokenReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("token request failed: %v", err), http.StatusBadGateway)
		return
	}
	defer tokenResp.Body.Close()

	body, err := io.ReadAll(tokenResp.Body)
	if err != nil {
		http.Error(w, "failed to read token response", http.StatusBadGateway)
		return
	}
	if tokenResp.StatusCode != http.StatusOK {
		http.Error(w, string(body), tokenResp.StatusCode)
		return
	}

	var tokens map[string]any
	if err := json.Unmarshal(body, &tokens); err != nil {
		http.Error(w, "failed to parse token response", http.StatusBadGateway)
		return
	}

	if accessToken, ok := tokens["access_token"].(string); ok && accessToken != "" {
		username, err := fetchOIDCUsername(r.Context(), discovery.UserinfoEndpoint, accessToken, cfg.OIDCUsernameClaim)
		if err == nil {
			tokens["username"] = username
		} else {
			slog.Warn("failed to resolve oidc username after token exchange", "error", err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tokens)
}

func fetchOIDCDiscovery(ctx context.Context, issuer string) (oidcDiscoveryResponse, error) {
	if issuer == "" {
		return oidcDiscoveryResponse{}, fmt.Errorf("OIDC_ISSUER_URL is not configured")
	}
	discoveryURL := strings.TrimRight(issuer, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return oidcDiscoveryResponse{}, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return oidcDiscoveryResponse{}, fmt.Errorf("oidc discovery request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return oidcDiscoveryResponse{}, fmt.Errorf("oidc discovery returned %d: %s", resp.StatusCode, body)
	}

	var discovery oidcDiscoveryResponse
	if err := json.NewDecoder(resp.Body).Decode(&discovery); err != nil {
		return oidcDiscoveryResponse{}, fmt.Errorf("parse oidc discovery: %w", err)
	}
	return discovery, nil
}

func fetchOIDCUsername(ctx context.Context, userinfoEndpoint, accessToken, usernameClaim string) (string, error) {
	if userinfoEndpoint == "" {
		return "", fmt.Errorf("oidc provider has no userinfo_endpoint")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userinfoEndpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

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
		return "", fmt.Errorf("parse userinfo: %w", err)
	}
	username, _ := claims[usernameClaim].(string)
	if username == "" {
		return "", fmt.Errorf("userinfo missing %s claim", usernameClaim)
	}
	return username, nil
}
