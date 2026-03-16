package main

import (
	"context"
	"log"
	"net/http"

	"github.com/chunhou/engram/internal/api"
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

	srv := api.NewServer(pool)
	addr := ":" + cfg.Port
	log.Printf("Engram backend listening on %s", addr)
	if err := http.ListenAndServe(addr, srv.Routes()); err != nil {
		log.Fatalf("http server: %v", err)
	}
}
