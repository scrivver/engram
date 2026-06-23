package main

import (
	"context"
	"log"
	"net/http"

	"github.com/chunhou/engram/internal/api"
	"github.com/chunhou/engram/internal/auth"
	"github.com/chunhou/engram/internal/config"
	"github.com/chunhou/engram/internal/db"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if err := db.RunMigrations(cfg.PGMigrateURL()); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	pool, err := db.Open(context.Background(), cfg.PGDSN())
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer pool.Close()

	srv := api.NewServer(
		pool,
		api.WithConfig(cfg),
		api.WithPresignURLTemplate(cfg.PresignURLTemplate),
	)

	var authMW api.Middleware
	var authMode string

	var jwtAuth *auth.JWTAuth
	if cfg.JWTSecret != "" {
		var err error
		jwtAuth, err = auth.NewJWTAuth(cfg.JWTSecret)
		if err != nil {
			log.Fatalf("jwt auth: %v", err)
		}
	}

	var oidcAuth *auth.OIDCAuthenticator
	if cfg.OIDCIssuerURL != "" {
		var err error
		oidcAuth, err = auth.NewOIDCAuthenticator(context.Background(), cfg)
		if err != nil {
			log.Fatalf("oidc authenticator: %v", err)
		}
	}

	switch {
	case jwtAuth != nil && oidcAuth != nil:
		authMW = auth.NewMultiAuthenticator(jwtAuth, oidcAuth).Middleware
		authMode = "mixed"
	case jwtAuth != nil:
		authMW = jwtAuth.Middleware
		authMode = "jwt"
	case oidcAuth != nil:
		authMW = oidcAuth.Middleware
		authMode = "oidc"
	default:
		authMode = "none"
		// No auth — leave requests unauthenticated. Owner-scoped queries will
		// return only rows with NULL owner (effectively hidden).
	}

	addr := ":" + cfg.Port
	log.Printf("Engram backend listening on %s (auth=%s)", addr, authMode)
	if err := http.ListenAndServe(addr, srv.Routes(authMW)); err != nil {
		log.Fatalf("http server: %v", err)
	}
}
