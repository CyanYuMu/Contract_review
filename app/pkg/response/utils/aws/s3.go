package aws

import (
	"context"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

type Storage struct {
	name   string
	client *s3.S3
}

func (s *Storage) Client() *s3.S3 {
	return s.client
}

func (s *Storage) Name() string {
	return s.name
}

type ConfigS3 struct {
	AccessKeyID     string
	SecretAccessKey string
	Region          string
	Bucket          string
}

func NewS3Client(ctx context.Context, cnf ConfigS3) (*Storage, error) {
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(cnf.Region),
		Credentials: credentials.NewStaticCredentials(cnf.AccessKeyID, cnf.SecretAccessKey, ""),
	})
	if err != nil {
		return nil, err
	}
	client := s3.New(sess)
	storage := &Storage{
		name:   cnf.Bucket,
		client: client,
	}
	return storage, nil
}
