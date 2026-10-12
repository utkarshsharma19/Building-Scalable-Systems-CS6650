package s3util

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Client struct{ s3 *s3.Client }

func New() (*Client, error) {
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion("us-west-2"))
	if err != nil {
		return nil, err
	}
	return &Client{s3: s3.NewFromConfig(cfg)}, nil
}

func Parse(url string) (bucket, key string, err error) {
	rest, ok := strings.CutPrefix(url, "s3://")
	if ok {
		bucket, key, ok = strings.Cut(rest, "/")
	}
	if !ok || bucket == "" || key == "" {
		return "", "", fmt.Errorf("bad s3 url: %q", url)
	}
	return bucket, key, nil
}

func URL(bucket, key string) string { return "s3://" + bucket + "/" + key }

func (c *Client) Get(ctx context.Context, url string) ([]byte, error) {
	bucket, key, err := Parse(url)
	if err != nil {
		return nil, err
	}
	out, err := c.s3.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}

func (c *Client) Put(ctx context.Context, url string, body []byte) error {
	bucket, key, err := Parse(url)
	if err != nil {
		return err
	}
	_, err = c.s3.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), Body: bytes.NewReader(body)})
	return err
}
