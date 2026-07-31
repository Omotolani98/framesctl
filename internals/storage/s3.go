// Package storage handles video storage to s3
package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Omotolani98/framesctl/internals/config"
	"github.com/Omotolani98/framesctl/internals/framesrvr"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const (
	uploadPartSize     int64 = 16 << 20 // 16 MiB
	uploadConcurrency        = 2
	multipartThreshold int64 = 16 << 20
)

type VideoStore struct {
	bucket   string
	region   string
	s3Client *s3.Client
	presign  *s3.PresignClient
	transfer *transfermanager.Client
}

type UploadResult struct {
	Bucket        string
	Key           string
	Location      string
	ETag          string
	ContentLength int64
}

type Object struct {
	Body          io.ReadCloser
	ContentLength int64
	ContentType   string
	ETag          string
	ContentRange  string
	AcceptRanges  string
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
		region:   cfg.AWSRegion,
		s3Client: s3Client,
		presign:  s3.NewPresignClient(s3Client),
		transfer: transferClient,
	}, nil
}

type MultipartUpload struct {
	Bucket   string
	Key      string
	UploadID string
}

type SignedUploadPart struct {
	URL string
}

func (store *VideoStore) CreateMultipartUpload(
	ctx context.Context,
	key string,
	contentType string,
) (MultipartUpload, error) {
	output, err := store.s3Client.CreateMultipartUpload(
		ctx,
		&s3.CreateMultipartUploadInput{
			Bucket:      aws.String(store.bucket),
			Key:         aws.String(key),
			ContentType: aws.String(contentType),
		},
	)
	if err != nil {
		return MultipartUpload{}, fmt.Errorf("create S3 multipart upload: %w", err)
	}

	return MultipartUpload{
		Bucket:   store.bucket,
		Key:      key,
		UploadID: aws.ToString(output.UploadId),
	}, nil
}

func (store *VideoStore) SignUploadPart(
	ctx context.Context,
	key string,
	uploadID string,
	partNumber int32,
) (SignedUploadPart, error) {
	request, err := store.presign.PresignUploadPart(
		ctx,
		&s3.UploadPartInput{
			Bucket:     aws.String(store.bucket),
			Key:        aws.String(key),
			UploadId:   aws.String(uploadID),
			PartNumber: aws.Int32(partNumber),
		},
		s3.WithPresignExpires(15*time.Minute),
	)
	if err != nil {
		return SignedUploadPart{}, fmt.Errorf("sign S3 upload part: %w", err)
	}

	return SignedUploadPart{URL: request.URL}, nil
}

func (store *VideoStore) CompleteMultipartUpload(
	ctx context.Context,
	key string,
	uploadID string,
	parts []framesrvr.CompleteUploadPart,
	contentLength int64,
	contentType string,
) (UploadResult, error) {
	completedParts := make([]types.CompletedPart, 0, len(parts))
	for _, part := range parts {
		completedParts = append(
			completedParts,
			types.CompletedPart{
				ETag:       aws.String(part.ETag),
				PartNumber: aws.Int32(part.PartNumber),
			},
		)
	}

	output, err := store.s3Client.CompleteMultipartUpload(
		ctx,
		&s3.CompleteMultipartUploadInput{
			Bucket:   aws.String(store.bucket),
			Key:      aws.String(key),
			UploadId: aws.String(uploadID),
			MultipartUpload: &types.CompletedMultipartUpload{
				Parts: completedParts,
			},
		},
	)
	if err != nil {
		return UploadResult{}, fmt.Errorf("complete S3 multipart upload: %w", err)
	}

	head, err := store.s3Client.HeadObject(
		ctx,
		&s3.HeadObjectInput{
			Bucket: aws.String(store.bucket),
			Key:    aws.String(key),
		},
	)
	if err != nil {
		return UploadResult{}, fmt.Errorf("verify S3 multipart upload: %w", err)
	}

	if aws.ToInt64(head.ContentLength) != contentLength {
		return UploadResult{}, fmt.Errorf(
			"verify S3 multipart upload: content length is %d, want %d",
			aws.ToInt64(head.ContentLength),
			contentLength,
		)
	}

	if actualType := aws.ToString(head.ContentType); actualType != "" && actualType != contentType {
		return UploadResult{}, fmt.Errorf(
			"verify S3 multipart upload: content type is %q, want %q",
			actualType,
			contentType,
		)
	}

	return UploadResult{
		Bucket:        store.bucket,
		Key:           key,
		Location:      objectLocation(store.bucket, store.region, key),
		ETag:          aws.ToString(output.ETag),
		ContentLength: aws.ToInt64(head.ContentLength),
	}, nil
}

func (store *VideoStore) AbortMultipartUpload(
	ctx context.Context,
	key string,
	uploadID string,
) error {
	_, err := store.s3Client.AbortMultipartUpload(
		ctx,
		&s3.AbortMultipartUploadInput{
			Bucket:   aws.String(store.bucket),
			Key:      aws.String(key),
			UploadId: aws.String(uploadID),
		},
	)
	if err != nil {
		return fmt.Errorf("abort S3 multipart upload: %w", err)
	}

	return nil
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

func (store *VideoStore) Download(
	ctx context.Context,
	key string,
	destination io.Writer,
) error {
	output, err := store.s3Client.GetObject(
		ctx,
		&s3.GetObjectInput{
			Bucket: aws.String(store.bucket),
			Key:    aws.String(key),
		},
	)
	if err != nil {
		return fmt.Errorf("download S3 object: %w", err)
	}
	defer output.Body.Close()

	if _, err := io.Copy(destination, output.Body); err != nil {
		return fmt.Errorf("write S3 object: %w", err)
	}

	return nil
}

func (store *VideoStore) UploadFile(
	ctx context.Context,
	key string,
	contentType string,
	path string,
) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open artifact: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect artifact: %w", err)
	}

	_, err = store.s3Client.PutObject(
		ctx,
		&s3.PutObjectInput{
			Bucket:        aws.String(store.bucket),
			Key:           aws.String(key),
			Body:          file,
			ContentLength: aws.Int64(info.Size()),
			ContentType:   aws.String(contentType),
		},
	)
	if err != nil {
		return fmt.Errorf("upload S3 artifact: %w", err)
	}

	return nil
}

func (store *VideoStore) ReadObject(
	ctx context.Context,
	key string,
	rangeHeader string,
) (Object, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(store.bucket),
		Key:    aws.String(key),
	}
	if rangeHeader != "" {
		input.Range = aws.String(rangeHeader)
	}

	output, err := store.s3Client.GetObject(ctx, input)
	if err != nil {
		return Object{}, fmt.Errorf("read S3 object: %w", err)
	}

	return Object{
		Body:          output.Body,
		ContentLength: aws.ToInt64(output.ContentLength),
		ContentType:   aws.ToString(output.ContentType),
		ETag:          aws.ToString(output.ETag),
		ContentRange:  aws.ToString(output.ContentRange),
		AcceptRanges:  aws.ToString(output.AcceptRanges),
	}, nil
}

func objectLocation(bucket string, region string, key string) string {
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", bucket, region, key)
}
