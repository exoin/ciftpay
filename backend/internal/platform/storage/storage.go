// Package storage is the file store behind merchant uploads (the signed
// Safaricom authorization letters, ADR-0008). Keys are relative paths such as
// authorizations/<org>/<shortcode>.jpg; the database stores the key, never an
// absolute path or URL, so the local-disk implementation can be swapped for an
// object store without touching handlers.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ErrNotFound is returned by Open for an unknown key.
var ErrNotFound = errors.New("storage: not found")

// Store puts and gets opaque files by key.
type Store interface {
	Put(ctx context.Context, key string, r io.Reader) error
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

// Local keeps files under a root directory (UPLOAD_DIR).
type Local struct{ root string }

// NewLocal creates the root directory if needed.
func NewLocal(root string) (*Local, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("storage: %w", err)
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, fmt.Errorf("storage: mkdir %s: %w", abs, err)
	}
	return &Local{root: abs}, nil
}

// Root is the absolute directory files live in.
func (l *Local) Root() string { return l.root }

func (l *Local) resolve(key string) (string, error) {
	clean := path.Clean("/" + strings.ReplaceAll(key, `\`, "/"))
	if clean == "/" || strings.Contains(key, "..") {
		return "", fmt.Errorf("storage: invalid key %q", key)
	}
	return filepath.Join(l.root, filepath.FromSlash(clean)), nil
}

// Put writes r to key atomically (temp file + rename), replacing any file.
func (l *Local) Put(_ context.Context, key string, r io.Reader) error {
	dst, err := l.resolve(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return fmt.Errorf("storage: mkdir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".upload-*")
	if err != nil {
		return fmt.Errorf("storage: temp: %w", err)
	}
	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return fmt.Errorf("storage: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("storage: close: %w", err)
	}
	if err := os.Chmod(tmp.Name(), 0o640); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("storage: chmod: %w", err)
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("storage: rename: %w", err)
	}
	return nil
}

// Open returns the file for key or ErrNotFound.
func (l *Local) Open(_ context.Context, key string) (io.ReadCloser, error) {
	p, err := l.resolve(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	return f, err
}

// Delete removes key; a missing file is not an error.
func (l *Local) Delete(_ context.Context, key string) error {
	p, err := l.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
