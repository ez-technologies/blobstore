package blobstore

import (
	"context"
	"fmt"
	"os"
)

// DefaultDir is the folder FromEnv uses for the local store when none is set.
const DefaultDir = "./data/files"

// FromEnv builds the store selected by environment variables whose names all
// start with prefix (for example "APP_"; may be empty):
//
//	<prefix>STORAGE=local (default)  files in <prefix>STORAGE_DIR (default DefaultDir)
//	<prefix>STORAGE=gcs              Cloud Storage bucket <prefix>GCS_BUCKET, optional <prefix>GCS_PREFIX
//	<prefix>STORAGE=s3               S3 bucket <prefix>S3_BUCKET, optional <prefix>S3_REGION
//	                                 (else AWS_REGION), <prefix>S3_PREFIX and <prefix>S3_ENDPOINT
//
// Any other value is an error rather than a silent fallback to local disk.
func FromEnv(prefix string) (Store, error) {
	env := func(name string) string { return os.Getenv(prefix + name) }
	switch kind := env("STORAGE"); kind {
	case "", "local":
		dir := env("STORAGE_DIR")
		if dir == "" {
			dir = DefaultDir
		}
		return NewLocal(dir)
	case "gcs":
		return NewGCS(context.Background(), GCSOptions{
			Bucket: env("GCS_BUCKET"),
			Prefix: env("GCS_PREFIX"),
		})
	case "s3":
		return NewS3(context.Background(), S3Options{
			Bucket:   env("S3_BUCKET"),
			Region:   env("S3_REGION"),
			Prefix:   env("S3_PREFIX"),
			Endpoint: env("S3_ENDPOINT"),
		})
	default:
		return nil, fmt.Errorf("blobstore: %sSTORAGE=%q is not supported (\"local\", \"gcs\" or \"s3\")", prefix, kind)
	}
}
