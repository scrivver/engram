package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/chunhou/engram-watcher/internal/config"
	"github.com/chunhou/engram-watcher/internal/publisher"
	"github.com/chunhou/engram-watcher/internal/watcher"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	pub, err := publisher.New(cfg.RabbitMQURL())
	if err != nil {
		log.Fatalf("rabbitmq: %v", err)
	}
	defer pub.Close()

	w, err := watcher.New(pub, cfg.DeviceName)
	if err != nil {
		log.Fatalf("watcher: %v", err)
	}
	defer w.Close()

	for _, dir := range cfg.WatchDirs {
		log.Printf("Watching: %s", dir)
		if err := w.AddRecursive(dir); err != nil {
			log.Fatalf("watch %s: %v", dir, err)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("Engram watcher running (device=%s)", cfg.DeviceName)
	if err := w.Run(ctx); err != nil {
		log.Fatalf("watcher: %v", err)
	}
}
