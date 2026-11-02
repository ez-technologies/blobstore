// Package blobstore stores file contents under opaque keys.
//
// Callers only use the Store interface, so where the bytes live is a
// deployment decision: a folder on local disk, memory (for tests), Google
// Cloud Storage, or S3 and S3-compatible object stores.
package blobstore

import (
	"context"
	"errors"
	"io"
)

// ErrNotFound means no file is stored under that key.
var ErrNotFound = errors.New("blobstore: no such file")

// Store is where file contents are kept, addressed by an opaque key.
//
// Keys should be chosen by the program, not by a client, and are made of
// letters, digits and "/", "-", "_", "." only; a store refuses anything else,
// including keys containing ".." or starting with "/".
type Store interface {
	// Put stores the contents under key, replacing any previous file.
	Put(ctx context.Context, key string, r io.Reader) error
	// Open returns the contents, or ErrNotFound.
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	// Delete removes the file. Deleting what is not there is not an error.
	Delete(ctx context.Context, key string) error
}
