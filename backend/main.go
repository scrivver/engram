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

	var authMW api.Middleware
	switch cfg.AuthMode {
	case "oidc":
		authenticator, err := auth.NewOIDCAuthenticator(context.Background(), cfg)
		if err != nil {
			log.Fatalf("oidc authenticator: %v", err)
		}
		authMW = authenticator.Middleware
	case "", "none":
		// No auth — leave requests unauthenticated. Owner-scoped queries will
		// return only rows with NULL owner (effectively hidden).
	default:
		log.Fatalf("unknown AUTH_MODE: %q", cfg.AuthMode)
	}

	srv := api.NewServer(
		pool,
		api.WithConfig(cfg),
		api.WithPresignURLTemplate(cfg.PresignURLTemplate),
	)
	addr := ":" + cfg.Port
	log.Printf("Engram backend listening on %s (auth=%s)", addr, cfg.AuthMode)
	if err := http.ListenAndServe(addr, srv.Routes(authMW)); err != nil {
		log.Fatalf("http server: %v", err)
	}
}
