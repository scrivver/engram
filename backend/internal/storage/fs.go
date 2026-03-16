package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type FSStore struct {
	root string
}

func NewFSStore(root string) (*FSStore, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create storage root: %w", err)
	}
	return &FSStore{root: root}, nil
}

func (s *FSStore) Put(_ context.Context, key string, r io.Reader, _ int64) error {
	path := filepath.Join(s.root, key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

func (s *FSStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	path := filepath.Join(s.root, key)
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	return f, nil
}

func (s *FSStore) PresignedURL(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", nil
}

func (s *FSStore) Delete(_ context.Context, key string) error {
	path := filepath.Join(s.root, key)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete file: %w", err)
	}
	return nil
}
