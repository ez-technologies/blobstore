// Command fileserver is a small HTTP file server built on blobstore.
//
// Uploaded files are stored under opaque keys ("files/<random id>"), never
// under the name the client sent, and every store operation is logged with
// slog. The backend is chosen by environment variables starting with
// FILESERVER_ (see blobstore.FromEnv); with none set, files go to
// ./data/files on local disk.
//
//	go run ./examples/fileserver
//
//	curl -F file=@notes.txt localhost:8080/files      # upload
//	curl localhost:8080/files                         # list
//	curl localhost:8080/files/<id>                    # download
//	curl -X DELETE localhost:8080/files/<id>          # delete
//
// The list of uploaded files is kept in memory, so it is empty after a
// restart even though the contents are still in the store. A real program
// keeps that index in its database.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/ez-technologies/blobstore"
)

// file is what the server remembers about an upload. The contents are in the store.
type file struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Uploaded time.Time `json:"uploaded"`
}

type server struct {
	store blobstore.Store
	mu    sync.Mutex
	files map[string]file
}

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(log)

	inner, err := blobstore.FromEnv("FILESERVER_")
	if err != nil {
		log.Error("cannot open the file store", "error", err)
		os.Exit(1)
	}
	store := blobstore.Logged(inner, blobstore.LogOptions{
		Logger:      log,
		FromContext: loggerFrom,
	})
	backend := os.Getenv("FILESERVER_STORAGE")
	if backend == "" {
		backend = "local"
	}

	addr := os.Getenv("FILESERVER_ADDR")
	if addr == "" {
		addr = "localhost:8080"
	}
	log.Info("listening", "addr", addr, "storage", backend)
	if err := http.ListenAndServe(addr, withRequestLogger(log, newHandler(store))); err != nil {
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

// newHandler serves upload, list, download and delete on top of store.
func newHandler(store blobstore.Store) http.Handler {
	s := &server{store: store, files: map[string]file{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /files", s.upload)
	mux.HandleFunc("GET /files", s.list)
	mux.HandleFunc("GET /files/{id}", s.download)
	mux.HandleFunc("DELETE /files/{id}", s.remove)
	return mux
}

// key is where a file's contents live in the store. The client's file name is
// never part of it.
func key(id string) string { return "files/" + id }

func (s *server) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	part, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "send the file as multipart field \"file\"", http.StatusBadRequest)
		return
	}
	defer part.Close()

	id := newID()
	counted := &counter{r: part}
	if err := s.store.Put(r.Context(), key(id), counted); err != nil {
		http.Error(w, "could not store the file", http.StatusInternalServerError)
		return
	}
	f := file{ID: id, Name: header.Filename, Size: counted.n, Uploaded: time.Now().UTC().Truncate(time.Second)}
	s.mu.Lock()
	s.files[id] = f
	s.mu.Unlock()
	writeJSON(w, http.StatusCreated, f)
}

func (s *server) list(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	out := make([]file, 0, len(s.files))
	for _, f := range s.files {
		out = append(out, f)
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, http.StatusOK, out)
}

func (s *server) download(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	f, known := s.files[id]
	s.mu.Unlock()

	rc, err := s.store.Open(r.Context(), key(id))
	if errors.Is(err, blobstore.ErrNotFound) {
		http.Error(w, "no such file", http.StatusNotFound)
		return
	}
	if err != nil {
		// An id the store refuses (for example one with a space) ends here too.
		http.Error(w, "could not read the file", http.StatusBadRequest)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	if known {
		w.Header().Set("Content-Disposition", `attachment; filename="`+safeName(f.Name)+`"`)
	}
	_, _ = io.Copy(w, rc)
}

func (s *server) remove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.Delete(r.Context(), key(id)); err != nil {
		http.Error(w, "could not delete the file", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	delete(s.files, id)
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// safeName keeps a file name usable in a header.
func safeName(name string) string {
	out := []rune{}
	for _, c := range name {
		if c >= 0x20 && c != '"' && c != '\\' && c != 0x7f {
			out = append(out, c)
		}
	}
	return string(out)
}

type counter struct {
	r io.Reader
	n int64
}

func (c *counter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// Each request gets a logger carrying its method and path, so the store's log
// lines say which request they belong to (LogOptions.FromContext).

type loggerKey struct{}

func withRequestLogger(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqLog := log.With("req", r.Method+" "+r.URL.Path)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), loggerKey{}, reqLog)))
	})
}

func loggerFrom(ctx context.Context) *slog.Logger {
	lg, _ := ctx.Value(loggerKey{}).(*slog.Logger)
	return lg
}
