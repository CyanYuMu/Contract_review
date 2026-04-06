package cloud_storage

import (
	"context"
	"io"
	"os"

	"github.com/minio/minio-go/v7"
	"golang.org/x/sync/errgroup"
)

type MinioStorage struct {
	client     *minio.Client
	bucketName string
}

type MinioParam struct {
	BucketName string
}

func NewMinioStorage(client *minio.Client, param MinioParam) CloudStorage {
	return &MinioStorage{
		client:     client,
		bucketName: param.BucketName,
	}
}

func (s *MinioStorage) Client() *StorageClient {
	return &StorageClient{
		bucket: s.client,
	}
}

// UploadObject 上传对象
func (s *MinioStorage) UploadObject(ctx context.Context, objectName string, file []byte, attrs ...ObjectAttr) error {
	return nil
}

// UploadObjectFromFile 上传对象通过文件
func (s *MinioStorage) UploadObjectFromFile(ctx context.Context, objectName string, file *os.File, attrs ...ObjectAttr) error {
	return nil
}

// ReadObject 读取对象
func (s *MinioStorage) ReadObject(ctx context.Context, objectName string) ([]byte, error) {
	return nil, nil
}

func (s *MinioStorage) UploadFromReader(ctx context.Context, objName string, reader io.Reader, attrs ...ObjectAttr) (err error) {
	return nil
}

// IsObjectExists 判断对象是否存在于存储桶中
func (s *MinioStorage) IsObjectExists(ctx context.Context, objectName string) bool {
	_, err := s.GetObjectAttrs(ctx, objectName)
	if err != nil {
		return false
	}
	return true
}

// GetObjectAttrs 获取文件属性
func (s *MinioStorage) GetObjectAttrs(ctx context.Context, objectName string) (*ObjectAttr, error) {
	attr, err := s.client.StatObject(ctx, s.bucketName, objectName, minio.StatObjectOptions{})
	if err != nil {
		return nil, err
	}
	return &ObjectAttr{
		Bucket:      s.bucketName,
		Name:        objectName,
		ContentType: attr.ContentType,
		Size:        attr.Size,
		Etag:        attr.ETag,
	}, nil
}

// BatchGetObjectAttrs 获取文件属性
func (s *MinioStorage) BatchGetObjectAttrs(ctx context.Context, objectNames []string) ([]*ObjectAttr, error) {
	var g errgroup.Group
	objAttrs := make([]*ObjectAttr, len(objectNames))
	for i, objectName := range objectNames {
		idx := i
		name := objectName
		g.Go(func() error {
			if name == "" {
				objAttrs[idx] = &ObjectAttr{}
				return nil
			}
			attr, err := s.GetObjectAttrs(ctx, name)
			if err != nil {
				return err
			}
			objAttrs[idx] = attr
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return objAttrs, nil
}

// SetObjectAttrs 设置对象属性
func (s *MinioStorage) SetObjectAttrs(ctx context.Context, objectName string, attrs map[string]string) error {
	return nil
}

// GetObjectSize 获取对象大小
func (s *MinioStorage) GetObjectSize(ctx context.Context, objectName string) int64 {
	attr, _ := s.GetObjectAttrs(ctx, objectName)
	if attr == nil {
		return 0
	}
	return attr.Size
}

// BatchGetPreSignatures 批量获取预签名
func (s *MinioStorage) BatchGetPreSignatures(prefix string, file File, items []ShardItem) (rs SignRs, err error) {
	return SignRs{}, nil
}

// GetPreSignature 获取预签名
func (s *MinioStorage) GetPreSignature(bucket Client, objectName string, file File) (string, error) {
	return "", nil
}

func (s *MinioStorage) SignedURl(ctx context.Context, bucket Client, objectName string, opt *SignedOptions) (string, error) {
	return "", nil
}

// MergeFileParts 合并文件分片
func (s *MinioStorage) MergeFileParts(record *MergeFileParts, param MergeFileParam) (string, error) {
	return "", nil
}

// MoveObject 移动对象
func (s *MinioStorage) MoveObject(ctx context.Context, objectName string, newBucket Client, newObjectName string) error {
	return nil
}

// DeleteObject 删除对象
func (s *MinioStorage) DeleteObject(ctx context.Context, objectName string) error {
	return s.client.RemoveObject(ctx, s.bucketName, objectName, minio.RemoveObjectOptions{})
}

func (s *MinioStorage) ComposerAndRun(ctx context.Context, objectName string, objects []interface{}, attrs ...ObjectAttr) (*ObjectAttr, error) {
	return nil, nil
}

func (s *MinioStorage) GetReader(ctx context.Context, objName string) (io.ReadCloser, error) {
	object, err := s.client.GetObject(ctx, s.bucketName, objName, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	return object, nil
}
