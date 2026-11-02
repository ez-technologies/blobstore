package blobstore_test

import (
	"context"
	"fmt"
	"google.golang.org/api/option"
	"io"
	"net"
	"strings"
	"testing"

	"github.com/fsouza/fake-gcs-server/fakestorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ez-technologies/blobstore"
)

// An in-process Cloud Storage emulator: the real client talks to it over HTTP, so what is
// checked is the code that will talk to Google, not a fake of it.
func fakeGCS(t *testing.T, bucket string) []option.ClientOption {
	t.Helper()
	// The emulator answers reads on its "public host", so that has to be the address the client
	// really uses: take a free port first, then start it there.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())
	host := fmt.Sprintf("127.0.0.1:%d", port)
	srv, err := fakestorage.NewServerWithOptions(fakestorage.Options{Scheme: "http", Host: "127.0.0.1", Port: uint16(port), PublicHost: host})
	require.NoError(t, err)
	t.Cleanup(srv.Stop)
	srv.CreateBucketWithOpts(fakestorage.CreateBucketOpts{Name: bucket})
	// The client reads this variable to talk to an emulator instead of Google, for reads and writes alike.
	t.Setenv("STORAGE_EMULATOR_HOST", host)
	return nil
}

func TestGCS_StoresServesAndDeletesFiles(t *testing.T) {
	ctx := context.Background()
	g, err := blobstore.NewGCS(ctx, blobstore.GCSOptions{Bucket: "files", Prefix: "tenant-a", ClientOptions: fakeGCS(t, "files")})
	require.NoError(t, err)

	require.NoError(t, g.Put(ctx, "items/a/one.jpg", strings.NewReader("first")))
	require.NoError(t, g.Put(ctx, "items/a/one.jpg", strings.NewReader("second")), "putting again replaces")
	rc, err := g.Open(ctx, "items/a/one.jpg")
	require.NoError(t, err)
	got, _ := io.ReadAll(rc)
	rc.Close()
	assert.Equal(t, "second", string(got))

	_, err = g.Open(ctx, "items/a/none.jpg")
	assert.ErrorIs(t, err, blobstore.ErrNotFound)

	require.NoError(t, g.Delete(ctx, "items/a/one.jpg"))
	require.NoError(t, g.Delete(ctx, "items/a/one.jpg"), "deleting what is not there is not an error")
	_, err = g.Open(ctx, "items/a/one.jpg")
	assert.ErrorIs(t, err, blobstore.ErrNotFound)
}

func TestGCS_RefusesTheKeysEveryStoreRefusesAndAWrongBucket(t *testing.T) {
	ctx := context.Background()
	opts := fakeGCS(t, "files")
	g, err := blobstore.NewGCS(ctx, blobstore.GCSOptions{Bucket: "files", ClientOptions: opts})
	require.NoError(t, err)
	for _, key := range []string{"", "../x", "/abs", "a b", "a/../b"} {
		assert.Error(t, g.Put(ctx, key, strings.NewReader("x")), key)
	}
	_, err = blobstore.NewGCS(ctx, blobstore.GCSOptions{Bucket: "no-such-bucket", ClientOptions: opts})
	assert.Error(t, err, "a bucket that cannot be reached is found at startup")
	_, err = blobstore.NewGCS(ctx, blobstore.GCSOptions{})
	assert.Error(t, err, "no bucket named")
}
