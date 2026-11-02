# blobstore

Pluggable file storage for Go. Files are kept under opaque keys behind one small
interface, so where the bytes live is a deployment choice:

- `Local`: a folder on disk (atomic writes, files mode 0640)
- `Memory`: in memory, for tests
- `GCS`: a Google Cloud Storage bucket
- `S3`: an S3 bucket or any S3-compatible store

```go
type Store interface {
	Put(ctx context.Context, key string, r io.Reader) error
	Open(ctx context.Context, key string) (io.ReadCloser, error) // ErrNotFound if missing
	Delete(ctx context.Context, key string) error                // missing is not an error
}
```

Keys may contain only letters, digits and `/ - _ .`, must not contain `..` and
must not start with `/`.

## Install

```sh
go get github.com/ez-technologies/blobstore
```

## Usage

```go
store, err := blobstore.NewLocal("./data/files")
if err != nil {
	log.Fatal(err)
}
store = blobstore.Logged(store, blobstore.LogOptions{Logger: slog.Default()})

err = store.Put(ctx, "things/42/photo.jpg", file)
rc, err := store.Open(ctx, "things/42/photo.jpg")
```

Or choose the backend from environment variables with a prefix of your choice:

```go
store, err := blobstore.FromEnv("APP_")
```

| Variable | Meaning |
|---|---|
| `APP_STORAGE` | `local` (default), `gcs` or `s3`; anything else is an error |
| `APP_STORAGE_DIR` | folder for `local` (default `./data/files`) |
| `APP_GCS_BUCKET`, `APP_GCS_PREFIX` | bucket and optional key prefix for `gcs` |
| `APP_S3_BUCKET`, `APP_S3_REGION`, `APP_S3_PREFIX`, `APP_S3_ENDPOINT` | bucket, region (else `AWS_REGION`), key prefix and custom endpoint for `s3` |

`NewGCS` and `NewS3` check that the bucket can be reached, so a wrong name or a
missing permission fails at startup. Credentials come from the provider's
default chain.

## Licence

MIT, see [LICENSE](LICENSE).
