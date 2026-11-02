package blobstore

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogged_PassesEverythingThroughAndNeverLogsContents(t *testing.T) {
	var out bytes.Buffer
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	slog.SetDefault(slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})))
	s := Logged(NewMemory(), LogOptions{})
	ctx := context.Background()

	require.NoError(t, s.Put(ctx, "users/docs/a.pdf", strings.NewReader("TOP SECRET CONTENT")))
	rc, err := s.Open(ctx, "users/docs/a.pdf")
	require.NoError(t, err)
	data, _ := io.ReadAll(rc)
	assert.Equal(t, "TOP SECRET CONTENT", string(data))
	_, err = s.Open(ctx, "users/missing.pdf")
	assert.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, s.Delete(ctx, "users/docs/a.pdf"))

	log := out.String()
	assert.Contains(t, log, `"op":"put"`)
	assert.Contains(t, log, "users/docs/a.pdf")
	assert.Contains(t, log, "not found")
	assert.NotContains(t, log, "TOP SECRET")
}

type ctxKey struct{}

func TestLogged_UsesTheGivenLoggerAndTheContextLogger(t *testing.T) {
	var base, scoped bytes.Buffer
	opts := LogOptions{
		Logger:  slog.New(slog.NewJSONHandler(&base, &slog.HandlerOptions{Level: slog.LevelDebug})),
		Message: "files",
		FromContext: func(ctx context.Context) *slog.Logger {
			l, _ := ctx.Value(ctxKey{}).(*slog.Logger)
			return l
		},
	}
	s := Logged(NewMemory(), opts)

	require.NoError(t, s.Put(context.Background(), "things/1", strings.NewReader("x")))
	assert.Contains(t, base.String(), `"msg":"files"`)

	ctx := context.WithValue(context.Background(), ctxKey{}, slog.New(slog.NewJSONHandler(&scoped, &slog.HandlerOptions{Level: slog.LevelDebug})))
	_, err := s.Open(ctx, "things/missing")
	assert.ErrorIs(t, err, ErrNotFound)
	assert.Contains(t, scoped.String(), "files: not found")
	assert.NotContains(t, base.String(), "things/missing")
}
