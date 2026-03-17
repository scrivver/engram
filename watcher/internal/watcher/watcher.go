package watcher

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chunhou/engram-watcher/internal/publisher"
	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	fsw        *fsnotify.Watcher
	pub        *publisher.Publisher
	deviceName string
	ignore     []string
	debounce   map[string]*time.Timer
	mu         sync.Mutex
	// Track renames: fsnotify sends Rename on the old path, then Create on the new path.
	// We buffer the old path briefly to pair them.
	pendingRename   string
	pendingRenameAt time.Time
}

func New(pub *publisher.Publisher, deviceName string, ignore []string) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}

	return &Watcher{
		fsw:        fsw,
		pub:        pub,
		deviceName: deviceName,
		ignore:     ignore,
		debounce:   make(map[string]*time.Timer),
	}, nil
}

func (w *Watcher) AddRecursive(dir string) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if w.shouldIgnore(path) {
				return filepath.SkipDir
			}
			if err := w.fsw.Add(path); err != nil {
				return fmt.Errorf("watch %s: %w", path, err)
			}
		}
		return nil
	})
}

func (w *Watcher) shouldIgnore(path string) bool {
	name := filepath.Base(path)
	// Skip hidden files/dirs (dotfiles)
	if strings.HasPrefix(name, ".") && name != "." {
		return true
	}
	for _, pattern := range w.ignore {
		if name == pattern {
			return true
		}
	}
	return false
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
			if w.shouldIgnore(event.Name) {
				continue
			}
			w.handleEvent(ctx, event)
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return nil
			}
			log.Printf("fsnotify error: %v", err)
		}
	}
}

func (w *Watcher) handleEvent(ctx context.Context, event fsnotify.Event) {
	if event.Has(fsnotify.Remove) {
		w.publishDelete(ctx, event.Name)
		return
	}

	if event.Has(fsnotify.Rename) {
		// Buffer the old path — a Create event for the new path may follow
		w.mu.Lock()
		w.pendingRename = event.Name
		w.pendingRenameAt = time.Now()
		w.mu.Unlock()

		// If no Create follows within 500ms, treat as delete
		time.AfterFunc(500*time.Millisecond, func() {
			w.mu.Lock()
			if w.pendingRename == event.Name {
				w.pendingRename = ""
				w.mu.Unlock()
				w.publishDelete(ctx, event.Name)
			} else {
				w.mu.Unlock()
			}
		})
		return
	}

	if event.Has(fsnotify.Create) {
		// Check if this is the new path of a rename
		w.mu.Lock()
		oldPath := w.pendingRename
		isRename := oldPath != "" && time.Since(w.pendingRenameAt) < 500*time.Millisecond
		if isRename {
			w.pendingRename = ""
		}
		w.mu.Unlock()

		if isRename {
			w.publishRename(ctx, oldPath, event.Name)
		} else {
			// Watch new directories
			if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
				w.AddRecursive(event.Name)
			}
			w.scheduleProcess(ctx, event.Name, "create")
		}
		return
	}

	if event.Has(fsnotify.Write) {
		w.scheduleProcess(ctx, event.Name, "create")
	}
}

func (w *Watcher) scheduleProcess(ctx context.Context, path string, eventType string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if t, ok := w.debounce[path]; ok {
		t.Stop()
	}
	w.debounce[path] = time.AfterFunc(1*time.Second, func() {
		w.mu.Lock()
		delete(w.debounce, path)
		w.mu.Unlock()

		w.processFile(ctx, path, eventType)
	})
}

func (w *Watcher) processFile(ctx context.Context, path string, eventType string) {
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
		Event:       eventType,
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

	log.Printf("published %s: %s (%d bytes)", eventType, path, info.Size())
}

func (w *Watcher) publishDelete(ctx context.Context, path string) {
	event := publisher.FileEvent{
		Event:       "delete",
		FilePath:    path,
		Filename:    filepath.Base(path),
		DeviceName:  w.deviceName,
		StorageType: "fs",
	}

	if err := w.pub.Publish(ctx, event); err != nil {
		log.Printf("publish delete %s: %v", path, err)
		return
	}

	log.Printf("published delete: %s", path)
}

func (w *Watcher) publishRename(ctx context.Context, oldPath, newPath string) {
	info, err := os.Stat(newPath)
	if err != nil {
		// New path doesn't exist — treat as delete of old path
		w.publishDelete(ctx, oldPath)
		return
	}
	if info.IsDir() {
		return
	}

	hash, err := hashFile(newPath)
	if err != nil {
		log.Printf("hash %s: %v", newPath, err)
		return
	}

	event := publisher.FileEvent{
		Event:       "rename",
		FilePath:    newPath,
		Filename:    filepath.Base(newPath),
		Size:        info.Size(),
		Hash:        fmt.Sprintf("sha256:%s", hash),
		Mtime:       info.ModTime().UTC().Format(time.RFC3339),
		DeviceName:  w.deviceName,
		StorageType: "fs",
		OldFilePath: oldPath,
	}

	if err := w.pub.Publish(ctx, event); err != nil {
		log.Printf("publish rename %s -> %s: %v", oldPath, newPath, err)
		return
	}

	log.Printf("published rename: %s -> %s", oldPath, newPath)
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
