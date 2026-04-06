package cloud_storage

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/iterator"

	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/google"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

type GcpOption struct {
	PreSignature GcsPreSignature
	SpeedUps     map[string]string
}

func NewGcpStorage(bucket *google.Storage, options ...GcpOption) CloudStorage {
	var preSignature GcsPreSignature
	var speedUps map[string]string
	if len(options) > 0 {
		preSignature = options[0].PreSignature
		speedUps = options[0].SpeedUps
	}
	return &GcpStorage{
		bucket:       bucket,
		preSignature: preSignature,
		speedUps:     speedUps,
	}
}

type GcpStorage struct {
	bucket       *google.Storage
	preSignature GcsPreSignature
	speedUps     map[string]string
}

func (s *GcpStorage) Client() *StorageClient {
	return &StorageClient{
		bucket: s.bucket,
	}
}

// UploadObject 上传对象
func (s *GcpStorage) UploadObject(ctx context.Context, objectName string, file []byte, attrs ...ObjectAttr) error {
	writer := s.bucket.Object(objectName).NewWriter(ctx)
	defer writer.Close()
	if attrs != nil {
		writer = attrs[0].SetGcpWriter(writer)
	}
	if writer.Bucket == "" {
		writer.Bucket = s.bucket.Name()
	}
	buf := bufio.NewReader(bytes.NewReader(file))
	if _, err := io.CopyBuffer(writer, buf, make([]byte, 1024*1024)); err != nil {
		return err
	}
	return nil
}

// UploadObjectFromFile 上传对象通过文件
func (s *GcpStorage) UploadObjectFromFile(ctx context.Context, objectName string, file *os.File, attrs ...ObjectAttr) error {
	writer := s.bucket.Object(objectName).NewWriter(ctx)
	defer writer.Close()
	if attrs != nil {
		writer = attrs[0].SetGcpWriter(writer)
	}
	if _, err := io.Copy(writer, file); err != nil {
		return err
	}
	return nil
}

// ReadObject 读取对象
func (s *GcpStorage) ReadObject(ctx context.Context, objectName string) ([]byte, error) {
	object := s.bucket.Object(objectName)
	reader, err := object.NewReader(ctx)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

// buff read
func (s *GcpStorage) ReadObjectToBuff(ctx context.Context, objectName string, buff *bytes.Buffer) (err error) {
	object := s.bucket.Object(objectName)

	reader, err := object.NewReader(ctx)
	if err != nil {
		return err
	}

	defer reader.Close()
	_, err = io.Copy(buff, reader)

	return err
}

func (s *GcpStorage) ReadObjectToWriter(ctx context.Context, objName string, writer io.Writer) (err error) {
	object := s.bucket.Object(objName)
	// 读取之前, 先判断文件是否存在
	if !s.IsObjectExists(ctx, objName) {
		return errors.New("file is not exist or not ready")
	}
	reader, err := object.NewReader(ctx)
	if err != nil {
		return err
	}
	defer reader.Close()
	n, err := io.Copy(writer, reader)
	if err != nil {
		return err
	}
	if n == 0 {
		err = errors.New("read object is empty")
		return
	}
	return nil
}

func (s *GcpStorage) UploadObjectFromBuff(ctx context.Context, objName string, buff *bytes.Buffer, attrs ...ObjectAttr) (err error) {
	writer := s.bucket.Object(objName).NewWriter(ctx)
	defer writer.Close()
	if attrs != nil {
		writer = attrs[0].SetGcpWriter(writer)
	}
	if _, err := io.Copy(writer, buff); err != nil {
		return err
	}
	return nil
}

func (s *GcpStorage) UploadFromReader(ctx context.Context, objName string, reader io.Reader, attrs ...ObjectAttr) (err error) {
	writer := s.bucket.Object(objName).NewWriter(ctx)
	defer writer.Close()
	if attrs != nil {
		writer = attrs[0].SetGcpWriter(writer)
	}
	if _, err := io.Copy(writer, reader); err != nil {
		return err
	}
	return nil
}

func (s *GcpStorage) GetReader(ctx context.Context, objName string) (io.ReadCloser, error) {
	object := s.bucket.Object(objName)
	reader, err := object.NewReader(ctx)
	if err != nil {
		return nil, err
	}
	return reader, nil
}

func (s *GcpStorage) batchGetObject(ctx context.Context, objectNames []string) *sync.Map {
	buf := &sync.Map{}
	var bufCount int
	for _, objectName := range objectNames {
		if _, ok := buf.LoadOrStore(objectName, nil); !ok {
			bufCount++
		}
	}
	wg := sync.WaitGroup{}
	wg.Add(bufCount)
	buf.Range(func(key, value any) bool {
		objectName := key.(string)
		go func(objectName string) {
			defer wg.Done()
			var b []byte
			var err error
			if objectName != "" {
				b, err = s.ReadObject(ctx, objectName)
			}
			buf.Store(objectName, Resource{
				B:   b,
				Err: err,
			})
		}(objectName)
		return true
	})
	wg.Wait()
	return buf
}

// IsObjectExists 判断对象是否存在于存储桶中
func (s *GcpStorage) IsObjectExists(ctx context.Context, objectName string) bool {
	_, err := s.bucket.Object(objectName).Attrs(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return false
		}
		su_logger.Error(ctx, err, "get object size failed", su_logger.E().String("name", objectName))
		return false
	}
	return true
}

// GetObjectAttrs 获取文件属性
func (s *GcpStorage) GetObjectAttrs(ctx context.Context, objectName string) (*ObjectAttr, error) {
	attr, err := s.bucket.Object(objectName).Attrs(ctx)
	if err != nil {
		return nil, err
	}
	return GcpToAttr(attr), nil
}

// BatchGetObjectAttrs 获取文件属性
func (s *GcpStorage) BatchGetObjectAttrs(ctx context.Context, objectNames []string) ([]*ObjectAttr, error) {
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
func (s *GcpStorage) SetObjectAttrs(ctx context.Context, objectName string, attrs map[string]string) error {
	object := s.bucket.Object(objectName)
	_, err := object.Update(ctx, storage.ObjectAttrsToUpdate{
		Metadata: attrs,
	})
	return err
}

// GetObjectSize 获取对象大小
func (s *GcpStorage) GetObjectSize(ctx context.Context, objectName string) int64 {
	attrs, err := s.bucket.Object(objectName).Attrs(ctx)
	if err != nil {
		su_logger.Warn(ctx, "get object size failed", su_logger.E().String("name", objectName).Error(err))
		return 0
	}
	if attrs == nil || attrs.Size == 0 {
		su_logger.Error(ctx, errors.New("attrs is nil or size is zero"), "get object size failed", su_logger.E().String("name", objectName))
		return 0
	}
	return attrs.Size
}

// BatchGetPreSignatures 批量获取预签名
func (s *GcpStorage) BatchGetPreSignatures(prefix string, file File, items []ShardItem) (rs SignRs, err error) {
	rs.Items = make([]SignItem, len(items))
	for i, item := range items {
		objectName := prefix + GenPartName(file.Id, file.Ext, strconv.Itoa(i+1))
		if item.ObjName != "" {
			objectName = item.ObjName
		}
		preSignature, err := s.GetPreSignature(s.Client(), objectName, file)
		if err != nil {
			return rs, err
		}
		preSignature = s.toSpeedUp(preSignature)
		rs.Items[i] = SignItem{
			Url:     preSignature,
			Idx:     item.Idx,
			Size:    item.Size,
			ObjName: objectName,
		}
	}

	return rs, nil
}

type GcsPreSignature struct {
	GoogleAccessId string
	PrivateKey     []byte
	Method         string
}

// GetPreSignature 获取预签名
func (s *GcpStorage) GetPreSignature(bucket Client, objectName string, file File) (string, error) {
	method := s.preSignature.Method
	if file.Method != "" {
		method = file.Method
	}
	var expires = time.Now().Add(PresignedExpires)
	if !file.Expires.IsZero() {
		expires = file.Expires
	}
	opts := storage.SignedURLOptions{
		GoogleAccessID: s.preSignature.GoogleAccessId,
		PrivateKey:     s.preSignature.PrivateKey,
		Method:         method,
		ContentType:    file.ContentType,
		Expires:        expires,
	}
	signUrl, err := storage.SignedURL(bucket.ToGcs().Name(), objectName, &opts)
	if err != nil {
		return "", err
	}

	if method == "GET" {
		return signUrl, nil
	}
	return s.toSpeedUp(signUrl), nil
}

func (s *GcpStorage) SignedURl(ctx context.Context, bucket Client, objectName string, opt *SignedOptions) (string, error) {
	if opt.Method == "" {
		opt.Method = "POST"
	}
	opts := storage.SignedURLOptions{
		GoogleAccessID: s.preSignature.GoogleAccessId,
		PrivateKey:     s.preSignature.PrivateKey,
		Method:         opt.Method,
		ContentType:    opt.ContentType,
		Expires:        opt.ExpireAt,
	}

	signUrl, err := storage.SignedURL(bucket.ToGcs().Name(), objectName, &opts)

	return signUrl, err
}

func (s *GcpStorage) toSpeedUp(str string) string {
	for k, v := range s.speedUps {
		str = strings.Replace(str, k, v, 1)
	}
	return str
}

// MergeFileParts 合并文件分片
func (s *GcpStorage) MergeFileParts(record *MergeFileParts, param MergeFileParam) (string, error) {
	ctx := context.Background()
	mergeParts := make([]string, 0)
	mergeParts, err := merge(s.bucket, record, sortFileParts(record.Parts), mergeParts, 0)
	if err != nil {
		return "", err
	}

	// 最后一次合并，获取参数列表
	var objectHandlers []*storage.ObjectHandle
	objectHandlers = make([]*storage.ObjectHandle, 0, len(mergeParts))
	for i := 0; i < len(mergeParts); i++ {
		objectHandlers = append(objectHandlers, s.bucket.Object(record.Parts[mergeParts[i]].Url))
	}
	object := s.bucket.Object(record.Url)
	composer := object.ComposerFrom(objectHandlers...)
	composer.ContentType = "application/octet-stream"
	if _, err := composer.Run(ctx); err != nil {
		return "", err
	}
	attrs, err := object.Attrs(ctx)
	if attrs.Size != record.FileSize || attrs.Size > param.SizeLimit {
		return "", errors.New("wrong size")
	}

	// 移除之前的分片文件
	go func() {
		query := &storage.Query{
			Prefix: strings.Split(record.Url, ".")[0] + "_/",
		}
		iter := s.bucket.Objects(ctx, query)
		for {
			objAttrs, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				break
			}
			if err := s.bucket.Object(objAttrs.Name).Delete(ctx); err != nil {
			}
		}
	}()

	// 返回文件的 Crc32c 唯一校验标识
	return strconv.Itoa(int(attrs.CRC32C)), nil
}

func merge(bucket *google.Storage, record *MergeFileParts, parts []string, mergeParts []string, count int) ([]string, error) {
	remain := len(parts)
	if remain <= 0 {
		return mergeParts, nil
	}

	// 获取参数列表
	var objectHandlers []*storage.ObjectHandle
	if remain >= MaxChunk {
		objectHandlers = make([]*storage.ObjectHandle, 0, MaxChunk)
		for i := 0; i < MaxChunk; i++ {
			objectHandlers = append(objectHandlers, bucket.Object(record.Parts[parts[i]].Url))
		}
		parts = parts[MaxChunk:]
	} else {
		objectHandlers = make([]*storage.ObjectHandle, 0, remain)
		for i := 0; i < remain; i++ {
			objectHandlers = append(objectHandlers, bucket.Object(record.Parts[parts[i]].Url))
		}
		parts = []string{}
	}
	mergeKey := "merge#" + strconv.Itoa(count)
	url := record.Prefix + GenPartName(record.FileId, record.FileExt, mergeKey)
	object := bucket.Object(url)
	composer := object.ComposerFrom(objectHandlers...)
	composer.ContentType = "application/octet-stream"
	if _, err := composer.Run(context.Background()); err != nil {
		return nil, err
	}
	mergeParts = append(mergeParts, mergeKey)
	record.Parts[mergeKey] = &Part{
		Number:    mergeKey,
		Url:       url,
		Confirmed: true,
	}
	return merge(bucket, record, parts, mergeParts, count+1)
}

func sortFileParts(parts map[string]*Part) []string {
	numbers := make([]string, 0, len(parts))
	for _, part := range parts {
		numbers = append(numbers, part.Number)
	}
	sort.Slice(numbers, func(i, j int) bool {
		a, _ := strconv.Atoi(numbers[i])
		b, _ := strconv.Atoi(numbers[j])
		return a < b
	})
	return numbers
}

// MoveObject 移动对象
func (s *GcpStorage) MoveObject(ctx context.Context, objectName string, newBucket Client, newObjectName string) error {
	newGcpBucket := newBucket.ToGcs()
	_, err := newGcpBucket.Object(newObjectName).CopierFrom(s.bucket.Object(objectName)).Run(ctx)
	if err != nil {
		return err
	}
	return s.bucket.Object(objectName).Delete(ctx)
}

// DeleteObject 删除对象
func (s *GcpStorage) DeleteObject(ctx context.Context, objectName string) error {
	return s.bucket.Object(objectName).Delete(ctx)
}

func (s *GcpStorage) ComposerAndRun(ctx context.Context, objectName string, objects []interface{}, attrs ...ObjectAttr) (*ObjectAttr, error) {
	var objectHandlers []*storage.ObjectHandle
	for _, obj := range objects {
		objectHandlers = append(objectHandlers, obj.(*storage.ObjectHandle))
	}

	// 如果只有一个分片，直接移动
	if len(objectHandlers) == 1 {
		copier := s.bucket.Object(objectName).CopierFrom(objectHandlers[0])
		for _, val := range attrs {
			if val.ContentType != "" {
				copier.ContentType = val.ContentType
			}
		}
		if _, err := copier.Run(ctx); err != nil {
			return nil, err
		}
		if err := objectHandlers[0].Delete(ctx); err != nil {
			return nil, err
		}
		attr, err := s.bucket.Object(objectName).Attrs(ctx)
		if err != nil {
			return nil, err
		}
		return GcpToAttr(attr), nil
	}

	// 如果超过10个分片，分批次合并
	if len(objectHandlers) >= 4 {
		batchSize := 4
		var tempObjects []*storage.ObjectHandle

		// 使用 errgroup 进行并发控制
		var mu sync.Mutex // 保护 tempObjects
		wg := sync.WaitGroup{}

		tempObjects = make([]*storage.ObjectHandle, (len(objectHandlers)+batchSize-1)/batchSize)
		batchIdx := 0

		// 分批并发合并
		for i := 0; i < len(objectHandlers); i += batchSize {
			end := i + batchSize
			if end > len(objectHandlers) {
				end = len(objectHandlers)
			}

			// 创建临时对象名
			tempName := objectName + fmt.Sprintf(".temp.%d", i/batchSize)
			tempObj := s.bucket.Object(tempName)
			currentBatch := objectHandlers[i:end]

			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				// 合并当前批次
				composer := tempObj.ComposerFrom(currentBatch...)
				if _, err := composer.Run(ctx); err != nil {
					return
				}

				mu.Lock()
				tempObjects[idx] = tempObj
				mu.Unlock()
			}(batchIdx)

			batchIdx++
		}

		// 等待所有合并完成
		wg.Wait()

		// 合并临时对象并设置属性
		object := s.bucket.Object(objectName)
		composer := object.ComposerFrom(tempObjects...)
		for _, val := range attrs {
			if val.ContentType != "" {
				composer.ContentType = val.ContentType
			}
		}

		attr, err := composer.Run(ctx)
		if err != nil {
			return nil, err
		}

		// 清理临时对象
		for _, obj := range tempObjects {
			if err := obj.Delete(ctx); err != nil {
				return nil, err
			}
		}

		return GcpToAttr(attr), nil
	}

	// 少于10个分片直接合并
	object := s.bucket.Object(objectName)
	composer := object.ComposerFrom(objectHandlers...)
	for _, val := range attrs {
		if val.ContentType != "" {
			composer.ContentType = val.ContentType
		}
	}
	attr, err := composer.Run(ctx)
	if err != nil {
		return nil, err
	}
	return GcpToAttr(attr), nil
}
