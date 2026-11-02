package blobstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3 keeps files in an S3 bucket (or any S3-compatible store). Credentials come
// from the default AWS chain: environment variables, shared config, or an
// instance or task role.
type S3 struct {
	client   *s3.Client
	uploader *manager.Uploader
	bucket   string
	prefix   string
}

// S3Options is what NewS3 needs to know.
type S3Options struct {
	Bucket string
	// Region defaults to AWS_REGION / AWS_DEFAULT_REGION.
	Region string
	// Prefix puts every key under a folder of the bucket ("files/"), so a bucket can be shared.
	Prefix string
	// Endpoint points at an S3-compatible server (a local test one); empty means AWS.
	Endpoint string
}

// NewS3 connects to the bucket and checks it can be reached, so a wrong name or
// missing permission fails at startup rather than at the first write.
func NewS3(ctx context.Context, o S3Options) (*S3, error) {
	if o.Bucket == "" {
		return nil, errors.New("blobstore: no S3 bucket given")
	}
	var loaders []func(*awsconfig.LoadOptions) error
	if o.Region != "" {
		loaders = append(loaders, awsconfig.WithRegion(o.Region))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, loaders...)
	if err != nil {
		return nil, fmt.Errorf("blobstore: load AWS settings: %w", err)
	}
	if cfg.Region == "" {
		return nil, errors.New("blobstore: no AWS region: set Region or AWS_REGION")
	}
	client := s3.NewFromConfig(cfg, func(opts *s3.Options) {
		if o.Endpoint != "" {
			opts.BaseEndpoint = aws.String(o.Endpoint)
			opts.UsePathStyle = true
		}
	})
	if _, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(o.Bucket)}); err != nil {
		return nil, fmt.Errorf("blobstore: cannot reach bucket %q: %w", o.Bucket, err)
	}
	prefix := strings.Trim(o.Prefix, "/")
	if prefix != "" {
		prefix += "/"
	}
	return &S3{client: client, uploader: manager.NewUploader(client), bucket: o.Bucket, prefix: prefix}, nil
}

func (s *S3) objectKey(key string) (*string, error) {
	if err := checkKey(key); err != nil {
		return nil, err
	}
	return aws.String(s.prefix + key), nil
}

func (s *S3) Put(ctx context.Context, key string, r io.Reader) error {
	k, err := s.objectKey(key)
	if err != nil {
		return err
	}
	// The uploader streams a reader of unknown length (in parts when it is big).
	_, err = s.uploader.Upload(ctx, &s3.PutObjectInput{Bucket: aws.String(s.bucket), Key: k, Body: r})
	return err
}

func (s *S3) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	k, err := s.objectKey(key)
	if err != nil {
		return nil, err
	}
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: k})
	if err != nil {
		var missing *types.NoSuchKey
		if errors.As(err, &missing) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return out.Body, nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	k, err := s.objectKey(key)
	if err != nil {
		return err
	}
	// S3 answers success for a key that is not there, which is what Store asks for.
	_, err = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: k})
	return err
}
