package blobstore

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"
)

// LogOptions configures Logged.
type LogOptions struct {
	// Logger receives the lines; nil means slog.Default().
	Logger *slog.Logger
	// FromContext, if set, returns a logger for the call's context (for example
	// one carrying a request id). A nil result falls back to Logger.
	FromContext func(context.Context) *slog.Logger
	// Message is the log message; empty means "file store".
	Message string
}

// Logged wraps a store so every operation logs one line: the operation, the
// key, the duration and any error. Success is logged at Debug, ErrNotFound at
// Info, other errors at Error. File contents are never logged.
func Logged(inner Store, o LogOptions) Store { return &logged{inner: inner, o: o} }

type logged struct {
	inner Store
	o     LogOptions
}

func (l *logged) logger(ctx context.Context) *slog.Logger {
	if l.o.FromContext != nil {
		if lg := l.o.FromContext(ctx); lg != nil {
			return lg
		}
	}
	if l.o.Logger != nil {
		return l.o.Logger
	}
	return slog.Default()
}

func (l *logged) done(ctx context.Context, op, key string, start time.Time, err error) {
	msg := l.o.Message
	if msg == "" {
		msg = "file store"
	}
	attrs := []any{"op", op, "key", key, "duration_ms", time.Since(start).Milliseconds()}
	lg := l.logger(ctx)
	switch {
	case err == nil:
		lg.DebugContext(ctx, msg, attrs...)
	case errors.Is(err, ErrNotFound):
		lg.InfoContext(ctx, msg+": not found", attrs...)
	default:
		lg.ErrorContext(ctx, msg+" failed", append(attrs, "error", err.Error())...)
	}
}

func (l *logged) Put(ctx context.Context, key string, r io.Reader) error {
	start := time.Now()
	err := l.inner.Put(ctx, key, r)
	l.done(ctx, "put", key, start, err)
	return err
}

func (l *logged) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	start := time.Now()
	rc, err := l.inner.Open(ctx, key)
	l.done(ctx, "open", key, start, err)
	return rc, err
}

func (l *logged) Delete(ctx context.Context, key string) error {
	start := time.Now()
	err := l.inner.Delete(ctx, key)
	l.done(ctx, "delete", key, start, err)
	return err
}
