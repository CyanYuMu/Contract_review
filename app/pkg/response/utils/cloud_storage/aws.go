package cloud_storage

import (
	"bytes"
	"context"
	"io"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
)

type AwsStorage struct {
	bucketName string
	client     *s3.S3
}

func NewAwsStorage(client *s3.S3, bucketName string) CloudStorage {
	return &AwsStorage{
		client:     client,
		bucketName: bucketName,
	}
}

func (s *AwsStorage) Client() *StorageClient {
	return &StorageClient{
		bucket: s.client,
	}
}

// UploadObject 上传对象
func (s *AwsStorage) UploadObject(ctx context.Context, objectName string, file []byte, attrs ...ObjectAttr) error {
	return nil
}

// UploadObjectFromFile 上传对象通过文件
func (s *AwsStorage) UploadObjectFromFile(ctx context.Context, objectName string, file *os.File, attrs ...ObjectAttr) error {
	return nil
}

// ReadObject 读取对象
func (s *AwsStorage) ReadObject(ctx context.Context, objectName string) ([]byte, error) {
	return nil, nil
}

// buff read
func (s *AwsStorage) ReadObjectToBuff(ctx context.Context, objectName string, buff *bytes.Buffer) (err error) {
	return nil
}

func (s *AwsStorage) ReadObjectToWriter(ctx context.Context, objName string, writer io.Writer) (err error) {
	return nil
}

func (s *AwsStorage) UploadObjectFromBuff(ctx context.Context, objName string, buff *bytes.Buffer, attrs ...ObjectAttr) (err error) {
	return nil
}

func (s *AwsStorage) UploadFromReader(ctx context.Context, objName string, reader io.Reader, attrs ...ObjectAttr) (err error) {
	return nil
}

// IsObjectExists 判断对象是否存在于存储桶中
func (s *AwsStorage) IsObjectExists(ctx context.Context, objectName string) bool {
	_, err := s.client.HeadObject(&s3.HeadObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(objectName),
	})
	if err != nil {
		return false
	}
	return true
}

// GetObjectAttrs 获取文件属性
func (s *AwsStorage) GetObjectAttrs(ctx context.Context, objectName string) (*ObjectAttr, error) {
	return nil, nil
}

// BatchGetObjectAttrs 获取文件属性
func (s *AwsStorage) BatchGetObjectAttrs(ctx context.Context, objectNames []string) ([]*ObjectAttr, error) {
	return nil, nil
}

// SetObjectAttrs 设置对象属性
func (s *AwsStorage) SetObjectAttrs(ctx context.Context, objectName string, attrs map[string]string) error {
	return nil
}

// GetObjectSize 获取对象大小
func (s *AwsStorage) GetObjectSize(ctx context.Context, objectName string) int64 {
	return 0
}

// BatchGetPreSignatures 批量获取预签名
func (s *AwsStorage) BatchGetPreSignatures(prefix string, file File, items []ShardItem) (rs SignRs, err error) {
	return SignRs{}, nil
}

// GetPreSignature 获取预签名
func (s *AwsStorage) GetPreSignature(bucket Client, objectName string, file File) (string, error) {
	return "", nil
}

func (s *AwsStorage) SignedURl(ctx context.Context, bucket Client, objectName string, opt *SignedOptions) (string, error) {
	return "", nil
}

// MergeFileParts 合并文件分片
func (s *AwsStorage) MergeFileParts(record *MergeFileParts, param MergeFileParam) (string, error) {
	return "", nil
}

// MoveObject 移动对象
func (s *AwsStorage) MoveObject(ctx context.Context, objectName string, newBucket Client, newObjectName string) error {
	return nil
}

// DeleteObject 删除对象
func (s *AwsStorage) DeleteObject(ctx context.Context, objectName string) error {
	return nil
}

func (s *AwsStorage) ComposerAndRun(ctx context.Context, objectName string, objects []interface{}, attrs ...ObjectAttr) (*ObjectAttr, error) {
	return nil, nil
}

func (s *AwsStorage) GetReader(ctx context.Context, objName string) (io.ReadCloser, error) {
	object, err := s.client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(objName),
	})
	if err != nil {
		return nil, err
	}
	return object.Body, nil
}
