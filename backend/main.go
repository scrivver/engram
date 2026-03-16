package main

import (
	"context"
	"log"
	"net/http"

	"github.com/chunhou/engram/internal/api"
	"github.com/chunhou/engram/internal/config"
	"github.com/chunhou/engram/internal/db"
	"github.com/chunhou/engram/internal/queue"
	"github.com/chunhou/engram/internal/storage"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// Run migrations
	if err := db.RunMigrations(cfg.PGMigrateURL()); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	// Open DB pool
	pool, err := db.Open(context.Background(), cfg.PGDSN())
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer pool.Close()

	// Initialize storage
	var store storage.Store
	switch cfg.StorageBackend {
	case "fs":
		store, err = storage.NewFSStore(cfg.StorageFSRoot)
	case "s3":
		store, err = storage.NewS3Store(
			cfg.StorageS3Endpoint,
			cfg.StorageS3AccessKey,
			cfg.StorageS3SecretKey,
			cfg.StorageS3Bucket,
		)
	}
	if err != nil {
		log.Fatalf("init storage (%s): %v", cfg.StorageBackend, err)
	}
	log.Printf("Storage backend: %s", cfg.StorageBackend)

	// Connect to RabbitMQ
	pub, err := queue.NewPublisher(cfg.RabbitMQURL())
	if err != nil {
		log.Fatalf("rabbitmq: %v", err)
	}
	defer pub.Close()

	// Determine bucket name for file records
	bucket := cfg.StorageBackend
	if cfg.StorageBackend == "s3" {
		bucket = cfg.StorageS3Bucket
	}

	// Start HTTP server
	srv := api.NewServer(pool, store, pub, bucket)
	addr := ":" + cfg.Port
	log.Printf("Engram backend listening on %s", addr)
	if err := http.ListenAndServe(addr, srv.Routes()); err != nil {
		log.Fatalf("http server: %v", err)
	}
}
