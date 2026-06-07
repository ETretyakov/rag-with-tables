package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/ETretyakov/rag-with-tables/internal/config"
)

// Client wraps minio-go and is compatible with both MinIO (dev) and AWS S3 (prod).
type Client struct {
	mc  *minio.Client
	cfg config.S3Config
}

// New creates a Client from config. The endpoint should be host:port (no scheme).
func New(cfg config.S3Config) (*Client, error) {
	// Strip http:// / https:// — minio-go takes a bare host:port.
	endpoint := cfg.Endpoint
	useSSL := strings.HasPrefix(endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "http://")

	lookup := minio.BucketLookupAuto
	if cfg.UsePathStyle {
		lookup = minio.BucketLookupPath
	}

	mc, err := minio.New(endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure:       useSSL,
		Region:       cfg.Region,
		BucketLookup: lookup,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}
	return &Client{mc: mc, cfg: cfg}, nil
}

// Upload puts data at the given key in the configured bucket.
func (c *Client) Upload(ctx context.Context, key string, data []byte) error {
	_, err := c.mc.PutObject(ctx, c.cfg.Bucket, key,
		bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: "application/octet-stream"},
	)
	if err != nil {
		return fmt.Errorf("upload %s: %w", key, err)
	}
	return nil
}

// Download retrieves the object at key and returns its bytes.
func (c *Client) Download(ctx context.Context, key string) ([]byte, error) {
	obj, err := c.mc.GetObject(ctx, c.cfg.Bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object %s: %w", key, err)
	}
	defer obj.Close()

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, fmt.Errorf("read object %s: %w", key, err)
	}
	return data, nil
}

// EnsureBucket creates the bucket if it does not exist yet.
func (c *Client) EnsureBucket(ctx context.Context) error {
	exists, err := c.mc.BucketExists(ctx, c.cfg.Bucket)
	if err != nil {
		return fmt.Errorf("bucket exists check: %w", err)
	}
	if exists {
		return nil
	}
	if err := c.mc.MakeBucket(ctx, c.cfg.Bucket,
		minio.MakeBucketOptions{Region: c.cfg.Region}); err != nil {
		return fmt.Errorf("make bucket: %w", err)
	}
	return nil
}
