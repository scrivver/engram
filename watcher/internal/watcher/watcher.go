package watcher

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/chunhou/engram-watcher/internal/publisher"
	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	fsw        *fsnotify.Watcher
	pub        *publisher.Publisher
	deviceName string
	debounce   map[string]*time.Timer
	mu         sync.Mutex
}

func New(pub *publisher.Publisher, deviceName string) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}

	return &Watcher{
		fsw:        fsw,
		pub:        pub,
		deviceName: deviceName,
		debounce:   make(map[string]*time.Timer),
	}, nil
}

func (w *Watcher) AddRecursive(dir string) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if err := w.fsw.Add(path); err != nil {
				return fmt.Errorf("watch %s: %w", path, err)
			}
		}
		return nil
	})
}

func (w *Watcher) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-w.fsw.Events:
			if !ok {
				return nil
			}
			if event.Has(fsnotify.Create) || event.Has(fsnotify.Write) {
				w.scheduleProcess(ctx, event.Name)
			}
			// Watch new directories
			if event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					w.AddRecursive(event.Name)
				}
			}
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return nil
			}
			log.Printf("fsnotify error: %v", err)
		}
	}
}

func (w *Watcher) scheduleProcess(ctx context.Context, path string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Debounce: wait 1s after last write before processing
	if t, ok := w.debounce[path]; ok {
		t.Stop()
	}
	w.debounce[path] = time.AfterFunc(1*time.Second, func() {
		w.mu.Lock()
		delete(w.debounce, path)
		w.mu.Unlock()

		w.processFile(ctx, path)
	})
}

func (w *Watcher) processFile(ctx context.Context, path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	if info.IsDir() {
		return
	}

	hash, err := hashFile(path)
	if err != nil {
		log.Printf("hash %s: %v", path, err)
		return
	}

	event := publisher.FileEvent{
		Event:       "create",
		FilePath:    path,
		Filename:    filepath.Base(path),
		Size:        info.Size(),
		Hash:        fmt.Sprintf("sha256:%s", hash),
		Mtime:       info.ModTime().UTC().Format(time.RFC3339),
		DeviceName:  w.deviceName,
		StorageType: "fs",
	}

	if err := w.pub.Publish(ctx, event); err != nil {
		log.Printf("publish %s: %v", path, err)
		return
	}

	log.Printf("published: %s (%d bytes)", path, info.Size())
}

func (w *Watcher) Close() error {
	return w.fsw.Close()
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
