package blobstore_test

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ez-technologies/blobstore"
)

// An in-process S3-compatible server: the real S3 client talks to it over HTTP,
// so what is checked is the code that will talk to AWS, not a fake of it.
func fakeS3(t *testing.T, bucket string) string {
	t.Helper()
	backend := s3mem.New()
	require.NoError(t, backend.CreateBucket(bucket))
	srv := httptest.NewServer(gofakes3.New(backend).Server())
	t.Cleanup(srv.Close)
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	return srv.URL
}

func TestS3_StoresServesAndDeletesFiles(t *testing.T) {
	ctx := context.Background()
	url := fakeS3(t, "files")
	s, err := blobstore.NewS3(ctx, blobstore.S3Options{Bucket: "files", Region: "eu-west-1", Endpoint: url, Prefix: "tenant-a"})
	require.NoError(t, err)

	require.NoError(t, s.Put(ctx, "items/a/one.jpg", strings.NewReader("first")))
	require.NoError(t, s.Put(ctx, "items/a/one.jpg", strings.NewReader("second")), "putting again replaces")

	rc, err := s.Open(ctx, "items/a/one.jpg")
	require.NoError(t, err)
	got, _ := io.ReadAll(rc)
	rc.Close()
	assert.Equal(t, "second", string(got))

	_, err = s.Open(ctx, "items/a/none.jpg")
	assert.ErrorIs(t, err, blobstore.ErrNotFound)

	require.NoError(t, s.Delete(ctx, "items/a/one.jpg"))
	require.NoError(t, s.Delete(ctx, "items/a/one.jpg"), "deleting what is not there is not an error")
	_, err = s.Open(ctx, "items/a/one.jpg")
	assert.ErrorIs(t, err, blobstore.ErrNotFound)
}

func TestS3_HoldsABigFileThatHasNoKnownLength(t *testing.T) {
	ctx := context.Background()
	url := fakeS3(t, "files")
	s, err := blobstore.NewS3(ctx, blobstore.S3Options{Bucket: "files", Region: "eu-west-1", Endpoint: url})
	require.NoError(t, err)
	big := strings.Repeat("x", 12<<20) // more than one upload part
	// A pipe has no length, as an upload stream does not.
	pr, pw := io.Pipe()
	go func() { _, _ = io.WriteString(pw, big); _ = pw.Close() }()
	require.NoError(t, s.Put(ctx, "things/big.bin", pr))
	rc, err := s.Open(ctx, "things/big.bin")
	require.NoError(t, err)
	got, _ := io.ReadAll(rc)
	rc.Close()
	assert.Len(t, got, len(big))
}

func TestS3_RefusesTheKeysEveryStoreRefuses(t *testing.T) {
	ctx := context.Background()
	url := fakeS3(t, "files")
	s, err := blobstore.NewS3(ctx, blobstore.S3Options{Bucket: "files", Region: "eu-west-1", Endpoint: url})
	require.NoError(t, err)
	for _, key := range []string{"", "../x", "/abs", "a b", "a/../b"} {
		assert.Error(t, s.Put(ctx, key, strings.NewReader("x")), key)
	}
}

func TestS3_StopsAtStartupWhenTheBucketIsWrongOrSettingsMissing(t *testing.T) {
	ctx := context.Background()
	url := fakeS3(t, "files")
	_, err := blobstore.NewS3(ctx, blobstore.S3Options{Bucket: "no-such-bucket", Region: "eu-west-1", Endpoint: url})
	assert.Error(t, err, "a bucket that cannot be reached is found at startup")
	_, err = blobstore.NewS3(ctx, blobstore.S3Options{Region: "eu-west-1"})
	assert.Error(t, err, "no bucket named")
}

func TestFromEnv_S3AsksForItsSettings(t *testing.T) {
	t.Setenv("APP_STORAGE", "s3")
	t.Setenv("APP_S3_BUCKET", "")
	_, err := blobstore.FromEnv("APP_")
	assert.Error(t, err, "asked for S3 without a bucket must fail, not fall back to disk")

	url := fakeS3(t, "files")
	t.Setenv("APP_S3_BUCKET", "files")
	t.Setenv("APP_S3_REGION", "eu-west-1")
	t.Setenv("APP_S3_ENDPOINT", url)
	s, err := blobstore.FromEnv("APP_")
	require.NoError(t, err)
	assert.IsType(t, &blobstore.S3{}, s)
}
