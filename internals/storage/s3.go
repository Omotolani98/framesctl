// Package storage handles video storage to s3
package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/Omotolani98/framesctl/internals/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	uploadPartSize     int64 = 16 << 20 // 16 MiB
	uploadConcurrency        = 2
	multipartThreshold int64 = 16 << 20
)

type VideoStore struct {
	bucket   string
	transfer *transfermanager.Client
}

type UploadResult struct {
	Bucket        string
	Key           string
	Location      string
	ETag          string
	ContentLength int64
}

func NewVideoStore(
	ctx context.Context,
	cfg config.Config,
) (*VideoStore, error) {
	awsConfig, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(cfg.AWSRegion),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				cfg.AWSAccessKeyID,
				cfg.AWSSecretKey,
				cfg.AWSSessionToken,
			),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration: %w", err)
	}

	s3Client := s3.NewFromConfig(awsConfig)

	transferClient := transfermanager.New(
		s3Client,
		func(options *transfermanager.Options) {
			options.PartSizeBytes = uploadPartSize
			options.MultipartUploadThreshold = multipartThreshold
			options.Concurrency = uploadConcurrency

			// Allows multipart cleanup to continue briefly if the
			// incoming HTTP request is cancelled.
			options.FailTimeout = 30 * time.Second
		},
	)

	return &VideoStore{
		bucket:   cfg.AWSBucket,
		transfer: transferClient,
	}, nil
}

func (store *VideoStore) Upload(
	ctx context.Context,
	key string,
	contentType string,
	body io.Reader,
) (UploadResult, error) {
	output, err := store.transfer.UploadObject(
		ctx,
		&transfermanager.UploadObjectInput{
			Bucket:      aws.String(store.bucket),
			Key:         aws.String(key),
			Body:        body,
			ContentType: aws.String(contentType),
		},
	)
	if err != nil {
		return UploadResult{}, fmt.Errorf(
			"upload video to S3: %w",
			err,
		)
	}

	return UploadResult{
		Bucket:        store.bucket,
		Key:           key,
		Location:      aws.ToString(output.Location),
		ETag:          aws.ToString(output.ETag),
		ContentLength: aws.ToInt64(output.ContentLength),
	}, nil
}
