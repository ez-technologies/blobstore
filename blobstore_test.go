package blobstore_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ez-technologies/blobstore"
)

func stores(t *testing.T) map[string]blobstore.Store {
	local, err := blobstore.NewLocal(t.TempDir())
	require.NoError(t, err)
	return map[string]blobstore.Store{"local": local, "memory": blobstore.NewMemory()}
}

func TestStores_RoundTripReplaceAndDelete(t *testing.T) {
	ctx := context.Background()
	for name, s := range stores(t) {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, s.Put(ctx, "things/a/one.jpg", strings.NewReader("first")))
			require.NoError(t, s.Put(ctx, "things/a/one.jpg", strings.NewReader("second")))

			r, err := s.Open(ctx, "things/a/one.jpg")
			require.NoError(t, err)
			got, _ := io.ReadAll(r)
			_ = r.Close()
			assert.Equal(t, "second", string(got))

			require.NoError(t, s.Delete(ctx, "things/a/one.jpg"))
			_, err = s.Open(ctx, "things/a/one.jpg")
			assert.ErrorIs(t, err, blobstore.ErrNotFound)
			assert.NoError(t, s.Delete(ctx, "things/a/one.jpg"), "deleting what is gone is fine")
		})
	}
}

func TestStores_RefuseKeysThatCouldEscape(t *testing.T) {
	ctx := context.Background()
	for name, s := range stores(t) {
		t.Run(name, func(t *testing.T) {
			for _, key := range []string{"", "../etc/passwd", "a/../../b", "/abs/path", "a b", "a\x00b", "a;rm"} {
				assert.Error(t, s.Put(ctx, key, strings.NewReader("x")), "%q", key)
			}
		})
	}
}

func TestLocal_NeverLeavesAPartialFileAndKeepsPermissionsTight(t *testing.T) {
	dir := t.TempDir()
	l, err := blobstore.NewLocal(dir)
	require.NoError(t, err)
	require.NoError(t, l.Put(context.Background(), "k/file.png", strings.NewReader("data")))

	entries, _ := os.ReadDir(filepath.Join(dir, "k"))
	require.Len(t, entries, 1, "no temporary files left behind")
	info, _ := entries[0].Info()
	assert.Zero(t, info.Mode().Perm()&0o007, "other users get no access")
}

func TestFromEnv(t *testing.T) {
	t.Setenv("APP_STORAGE_DIR", t.TempDir())
	t.Setenv("APP_STORAGE", "")
	s, err := blobstore.FromEnv("APP_")
	require.NoError(t, err)
	assert.NotNil(t, s)

	t.Setenv("APP_STORAGE", "gcs")
	_, err = blobstore.FromEnv("APP_")
	assert.Error(t, err, "a store that is asked for but cannot be built must fail, not fall back to disk")

	t.Setenv("APP_STORAGE", "ftp")
	_, err = blobstore.FromEnv("APP_")
	assert.Error(t, err, "an unknown kind is refused")
}

func TestFromEnv_NoPrefix(t *testing.T) {
	t.Setenv("STORAGE", "local")
	t.Setenv("STORAGE_DIR", t.TempDir())
	s, err := blobstore.FromEnv("")
	require.NoError(t, err)
	assert.IsType(t, &blobstore.Local{}, s)
}
