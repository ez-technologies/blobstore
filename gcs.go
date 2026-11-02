package blobstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

// GCS keeps files in a Google Cloud Storage bucket. Credentials come from the environment
// (Application Default Credentials).
type GCS struct {
	client *storage.Client
	bucket *storage.BucketHandle
	name   string
	prefix string
}

// GCSOptions is what NewGCS needs to know.
type GCSOptions struct {
	Bucket string
	// Prefix puts every key under a folder of the bucket ("files/"), so a bucket can be shared.
	Prefix string
	// ClientOptions is for tests (an emulator); empty means the environment's credentials.
	ClientOptions []option.ClientOption
}

// NewGCS connects and checks the bucket can be reached, so a wrong name or a missing permission
// fails at startup rather than at the first write.
func NewGCS(ctx context.Context, o GCSOptions) (*GCS, error) {
	if o.Bucket == "" {
		return nil, errors.New("blobstore: no GCS bucket given")
	}
	client, err := storage.NewClient(ctx, o.ClientOptions...)
	if err != nil {
		return nil, fmt.Errorf("blobstore: connect to Cloud Storage: %w", err)
	}
	handle := client.Bucket(o.Bucket)
	if _, err := handle.Attrs(ctx); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("blobstore: cannot reach bucket %q: %w", o.Bucket, err)
	}
	prefix := strings.Trim(o.Prefix, "/")
	if prefix != "" {
		prefix += "/"
	}
	return &GCS{client: client, bucket: handle, name: o.Bucket, prefix: prefix}, nil
}

func (g *GCS) object(key string) (*storage.ObjectHandle, error) {
	if err := checkKey(key); err != nil {
		return nil, err
	}
	return g.bucket.Object(g.prefix + key), nil
}

func (g *GCS) Put(ctx context.Context, key string, r io.Reader) error {
	obj, err := g.object(key)
	if err != nil {
		return err
	}
	w := obj.NewWriter(ctx)
	if _, err := io.Copy(w, r); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

func (g *GCS) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := g.object(key)
	if err != nil {
		return nil, err
	}
	rc, err := obj.NewReader(ctx)
	if errors.Is(err, storage.ErrObjectNotExist) {
		return nil, ErrNotFound
	}
	return rc, err
}

func (g *GCS) Delete(ctx context.Context, key string) error {
	obj, err := g.object(key)
	if err != nil {
		return err
	}
	err = obj.Delete(ctx)
	if errors.Is(err, storage.ErrObjectNotExist) {
		return nil
	}
	return err
}
