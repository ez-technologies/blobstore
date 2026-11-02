package blobstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// Local keeps files in a folder on local disk. Fine for a
// laptop and for a single server with a persistent disk; not for several
// servers, which is what an object store is for.
type Local struct{ root string }

// NewLocal returns a store rooted at dir, creating it if needed.
func NewLocal(dir string) (*Local, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, fmt.Errorf("blobstore: create %s: %w", abs, err)
	}
	return &Local{root: abs}, nil
}

var safeKey = regexp.MustCompile(`^[A-Za-z0-9_\-./]+$`)

// checkKey is the one definition of an acceptable key, shared by every store.
func checkKey(key string) error {
	if key == "" || !safeKey.MatchString(key) || strings.Contains(key, "..") || strings.HasPrefix(key, "/") {
		return fmt.Errorf("blobstore: invalid key %q", key)
	}
	return nil
}

// path turns a key into a file path inside the root, refusing anything that
// could step outside it.
func (l *Local) path(key string) (string, error) {
	if err := checkKey(key); err != nil {
		return "", err
	}
	full := filepath.Join(l.root, filepath.FromSlash(key))
	if !strings.HasPrefix(full, l.root+string(filepath.Separator)) {
		return "", fmt.Errorf("blobstore: invalid key %q", key)
	}
	return full, nil
}

func (l *Local) Put(_ context.Context, key string, r io.Reader) error {
	full, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return err
	}
	// Written beside the target and renamed, so a reader never sees half a file.
	tmp, err := os.CreateTemp(filepath.Dir(full), ".upload-*")
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(tmp, r)
	closeErr := tmp.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o640); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), full)
}

func (l *Local) Open(_ context.Context, key string) (io.ReadCloser, error) {
	full, err := l.path(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(full)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	return f, err
}

func (l *Local) Delete(_ context.Context, key string) error {
	full, err := l.path(key)
	if err != nil {
		return err
	}
	err = os.Remove(full)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// Memory keeps files in memory. For tests.
type Memory struct {
	mu    sync.Mutex
	files map[string][]byte
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory { return &Memory{files: map[string][]byte{}} }

func (m *Memory) Put(_ context.Context, key string, r io.Reader) error {
	if err := checkKey(key); err != nil {
		return err
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[key] = data
	return nil
}

func (m *Memory) Open(_ context.Context, key string) (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.files[key]
	if !ok {
		return nil, ErrNotFound
	}
	return io.NopCloser(strings.NewReader(string(data))), nil
}

func (m *Memory) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, key)
	return nil
}

// Keys lists what is stored, for tests.
func (m *Memory) Keys() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.files))
	for k := range m.files {
		out = append(out, k)
	}
	return out
}
