package cloud_storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/spf13/cast"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
	"golang.org/x/sync/errgroup"
)

func NewOssStorage(bucket *oss.Bucket) CloudStorage {
	return &OssStorage{
		bucket: bucket,
		client: &bucket.Client,
	}
}

type OssStorage struct {
	bucket *oss.Bucket
	client *oss.Client
}

func (s *OssStorage) Client() *StorageClient {
	return &StorageClient{
		client: s.bucket.Client,
		bucket: s.bucket,
	}
}

func (s *OssStorage) ReadObjectToBuff(ctx context.Context, objectName string, buff *bytes.Buffer) (err error) {
	panic("implement me")
}

func (s *OssStorage) ReadObjectToWriter(ctx context.Context, objName string, writer io.Writer) (err error) {
	panic("implement me")
}

func (s *OssStorage) UploadObjectFromBuff(ctx context.Context, objName string, buff *bytes.Buffer, attrs ...ObjectAttr) (err error) {
	panic("implement me")
}

func (s *OssStorage) UploadFromReader(ctx context.Context, objName string, reader io.Reader, attrs ...ObjectAttr) (err error) {
	panic("implement me")
}

// SetObjectMetas 设置对象元信息
func (s *OssStorage) SetObjectMetas(objectName string, attrs ...ObjectAttr) error {
	options := make([]oss.Option, 0, len(attrs))
	for _, attr := range attrs {
		if attr.ContentType != "" {
			options = append(options, oss.Meta("ContentType", attr.ContentType))
		}
		if attr.Etag != "" {
			options = append(options, oss.Meta("ETag", attr.Etag))
		}
	}
	if len(options) <= 0 {
		return nil
	}
	return s.bucket.SetObjectMeta(objectName, options...)
}

// UploadObject 上传对象
func (s *OssStorage) UploadObject(ctx context.Context, objectName string, file []byte, attrs ...ObjectAttr) error {
	var options []oss.Option
	for _, attr := range attrs {
		if attr.ContentType != "" {
			options = append(options, oss.ContentType(attr.ContentType))
		}
	}
	if err := s.bucket.PutObject(objectName, bytes.NewReader(file), options...); err != nil {
		return err
	}
	return s.SetObjectMetas(objectName, attrs...)
}

// UploadObjectFromFile 上传对象通过文件
func (s *OssStorage) UploadObjectFromFile(ctx context.Context, objectName string, file *os.File, attrs ...ObjectAttr) error {
	return nil
}

// ReadObject 读取对象
func (s *OssStorage) ReadObject(ctx context.Context, objectName string) ([]byte, error) {
	reader, err := s.bucket.GetObject(objectName)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

// IsObjectExists 判断对象是否存在于存储桶中
func (s *OssStorage) IsObjectExists(ctx context.Context, objectName string) bool {
	exists, err := s.bucket.IsObjectExist(objectName)
	if err != nil {
		su_logger.Error(ctx, err, "oss get object size failed", su_logger.E().String("name", objectName))
		return false
	}
	return exists
}

// GetObjectAttrs 获取文件属性
func (s *OssStorage) GetObjectAttrs(ctx context.Context, objectName string) (*ObjectAttr, error) {
	rsp, err := s.bucket.GetObjectDetailedMeta(objectName)
	if err != nil {
		return nil, err
	}
	return &ObjectAttr{
		Bucket:      s.bucket.BucketName,
		Name:        objectName,
		ContentType: rsp.Get(ContentType),
		Size:        cast.ToInt64(rsp.Get(ContentLength)),
		Etag:        rsp.Get("Etag"),
		Metadata: map[string]string{
			"frameCount":     rsp.Get("frameCount"),
			"resolution":     rsp.Get("resolution"),
			"X-Oss-Meta-Md5": rsp.Get("X-Oss-Meta-Md5"),
		},
		CRC64C: rsp.Get("X-Oss-Hash-Crc64ecma"),
	}, nil
}

// BatchGetObjectAttrs 获取文件属性
func (s *OssStorage) BatchGetObjectAttrs(ctx context.Context, objectNames []string) ([]*ObjectAttr, error) {
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
func (s *OssStorage) SetObjectAttrs(ctx context.Context, objectName string, attrs map[string]string) error {
	options := make([]oss.Option, 0, len(attrs))
	for k, v := range attrs {
		options = append(options, oss.Meta(k, v))
	}
	return s.bucket.SetObjectMeta(objectName, options...)
}

// GetObjectSize 获取对象大小
func (s *OssStorage) GetObjectSize(ctx context.Context, objectName string) int64 {
	attr, err := s.GetObjectAttrs(ctx, objectName)
	if err != nil {
		return 0
	}
	return attr.Size
}

// BatchGetPreSignatures 批量获取预签名
func (s *OssStorage) BatchGetPreSignatures(prefix string, file File, items []ShardItem) (rs SignRs, err error) {
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
		rs.Items[i] = SignItem{
			Url:     preSignature,
			Idx:     item.Idx,
			Size:    item.Size,
			ObjName: objectName,
		}
	}
	return rs, nil
}

// GetPreSignature 获取预签名
func (s *OssStorage) GetPreSignature(bucket Client, objectName string, file File) (string, error) {
	// 生成签名 URL
	var Method oss.HTTPMethod
	if file.Method != "" {
		Method = oss.HTTPMethod(file.Method)
	} else {
		Method = oss.HTTPPut
	}
	output, err := s.bucket.SignURL(objectName, Method, int64(PresignedExpires.Seconds()))
	if err != nil {
		return "", fmt.Errorf("failed to create signed URL: %w", err)
	}
	if !strings.Contains(output, "https") {
		output = strings.ReplaceAll(output, "http", "https")
	}

	return output, nil
}

// MergeFileParts 合并文件分片
func (s *OssStorage) MergeFileParts(record *MergeFileParts, param MergeFileParam) (string, error) {
	return "", nil
}

// MoveObject 移动对象
func (s *OssStorage) MoveObject(ctx context.Context, objectName string, newBucket Client, newObjectName string) error {
	return nil
}

// DeleteObject 删除对象
func (s *OssStorage) DeleteObject(ctx context.Context, objectName string) error {
	return s.bucket.DeleteObject(objectName)
}

// SignedURl(ctx context.Context, bucket Client, objectName string, opt *SignedOptions) (string, error)
func (s *OssStorage) SignedURl(ctx context.Context, bucket Client, objectName string, opt *SignedOptions) (string, error) {
	return s.bucket.SignURL(objectName, oss.HTTPGet, int64(opt.ExpireAt.Sub(time.Now()).Seconds()))
}

func (s *OssStorage) ComposerAndRun(ctx context.Context, objectName string, objects []interface{}, attrs ...ObjectAttr) (*ObjectAttr, error) {
	// 检查参数
	if len(objects) == 0 {
		return nil, fmt.Errorf("没有提供分片对象")
	}

	// 类型检查
	for i, obj := range objects {
		switch obj.(type) {
		case *ObjectAttr:
			// 支持的类型
		default:
			return nil, fmt.Errorf("不支持的对象类型: %T at index %d", obj, i)
		}
	}

	// 如果只有一个分片，直接复制
	if len(objects) == 1 {
		var sourceKey string
		switch objects[0].(type) {
		case *ObjectAttr:
			sourceKey = objects[0].(*ObjectAttr).Name
		default:
			return nil, fmt.Errorf("不支持的对象类型: %T", objects[0])
		}

		// 创建复制请求
		_, err := s.bucket.CopyObject(sourceKey, objectName)
		if err != nil {
			su_logger.Error(ctx, err, "复制对象失败", su_logger.E().String("sourceKey", sourceKey).String("targetKey", objectName))
			return nil, fmt.Errorf("复制对象失败: %w", err)
		}

		// 删除源对象
		err = s.bucket.DeleteObject(sourceKey)
		if err != nil {
			su_logger.Warn(ctx, "删除源对象失败", su_logger.E().String("key", sourceKey).Error(err))
			// 不返回错误，因为复制已经成功
		}

		// 获取新对象的属性
		props, err := s.bucket.GetObjectDetailedMeta(objectName)
		if err != nil {
			su_logger.Error(ctx, err, "获取对象元数据失败", su_logger.E().String("key", objectName))
			return nil, fmt.Errorf("获取对象元数据失败: %w", err)
		}

		// 设置内容类型
		if len(attrs) > 0 && attrs[0].ContentType != "" {
			err = s.bucket.SetObjectMeta(objectName, oss.Meta("ContentType", attrs[0].ContentType))
			if err != nil {
				su_logger.Warn(ctx, "设置对象内容类型失败", su_logger.E().String("key", objectName).Error(err))
			}
		}

		return &ObjectAttr{
			Name:        objectName,
			Size:        cast.ToInt64(props.Get("Content-Length")),
			ContentType: props.Get("Content-Type"),
			Metadata: map[string]string{
				"frameCount": props.Get("frameCount"),
				"resolution": props.Get("resolution"),
				"ETag":       props.Get("ETag"),
			},
			CRC64C: props.Get("X-Oss-Hash-Crc64ecma"),
		}, nil
	}

	// 如果超过32个分片，分批次合并
	//if len(objects) > MaxChunk {
	//	// 使用errgroup来管理goroutine和错误
	//	eg, egCtx := errgroup.WithContext(ctx)
	//
	//	// 创建一个唯一的前缀，避免命名冲突
	//	uniquePrefix := fmt.Sprintf("%s.%d.", objectName, time.Now().UnixNano())
	//
	//	// 计算需要多少批次
	//	batchSize := MaxChunk
	//	batchCount := (len(objects) + batchSize - 1) / batchSize
	//	tempObjects := make([]string, batchCount)
	//
	//	// 分批并发合并
	//	for i := 0; i < len(objects); i += batchSize {
	//		end := i + batchSize
	//		if end > len(objects) {
	//			end = len(objects)
	//		}
	//
	//		// 创建临时对象名，使用唯一前缀
	//		tempName := uniquePrefix + fmt.Sprintf("temp.%d", i/batchSize)
	//		currentBatch := objects[i:end]
	//		batchIdx := i / batchSize
	//
	//		eg.Go(func() error {
	//			// 准备分片
	//			parts := make([]string, len(currentBatch))
	//			for j, obj := range currentBatch {
	//				attr, ok := obj.(*ObjectAttr)
	//				if !ok {
	//					return fmt.Errorf("不支持的对象类型: %T", obj)
	//				}
	//				parts[j] = attr.Name
	//			}
	//
	//			// 使用OSS的分片文件上传接口进行合并
	//			imurUpload, err := s.bucket.InitiateMultipartUpload(tempName)
	//			if err != nil {
	//				su_logger.Error(egCtx, err, "初始化分段上传失败", su_logger.E().String("objName", tempName))
	//				return fmt.Errorf("初始化分段上传失败: %w", err)
	//			}
	//
	//			uploadParts := make([]oss.UploadPart, len(parts))
	//			for j, obj := range currentBatch {
	//				attr, _ := obj.(*ObjectAttr)
	//				part, err := s.bucket.UploadPartCopy(
	//					imurUpload,
	//					s.bucket.BucketName,
	//					attr.Name,
	//					0,
	//					attr.Size,   // startPosition - 起始位置
	//					j+1, // partSize - 分片大小，0表示从起始位置到文件末尾
	//				)
	//				if err != nil {
	//					su_logger.Error(egCtx, err, "复制分段失败", su_logger.E().String("key", attr.Name))
	//					return fmt.Errorf("复制分段失败: %w", err)
	//				}
	//				uploadParts[j] = part
	//			}
	//
	//			// 完成分段上传
	//			_, err = s.bucket.CompleteMultipartUpload(imurUpload, uploadParts)
	//			if err != nil {
	//				su_logger.Error(egCtx, err, "完成分段上传失败", su_logger.E().String("objName", tempName))
	//				return fmt.Errorf("完成分段上传失败: %w", err)
	//			}
	//
	//			// 保存临时对象名
	//			tempObjects[batchIdx] = tempName
	//			return nil
	//		})
	//	}
	//
	//	// 等待所有合并完成，如果有错误则返回
	//	if err := eg.Wait(); err != nil {
	//		// 清理已创建的临时对象
	//		for _, objKey := range tempObjects {
	//			if objKey != "" {
	//				cleanErr := s.bucket.DeleteObject(objKey)
	//				if cleanErr != nil {
	//					su_logger.Warn(ctx, "清理临时对象失败", su_logger.E().String("key", objKey).Error(cleanErr))
	//				}
	//			}
	//		}
	//		return nil, err
	//	}
	//
	//	// 最终合并临时对象
	//	imurFinal, err := s.bucket.InitiateMultipartUpload(objectName)
	//	if err != nil {
	//		// 清理临时对象
	//		for _, objKey := range tempObjects {
	//			cleanErr := s.bucket.DeleteObject(objKey)
	//			if cleanErr != nil {
	//				su_logger.Warn(ctx, "清理临时对象失败", su_logger.E().String("key", objKey).Error(cleanErr))
	//			}
	//		}
	//		su_logger.Error(ctx, err, "初始化最终分段上传失败", su_logger.E().String("objName", objectName))
	//		return nil, fmt.Errorf("初始化最终分段上传失败: %w", err)
	//	}
	//
	//	// 准备最终合并的部分
	//	finalParts := make([]oss.UploadPart, len(tempObjects))
	//	var finalErr error
	//	for i, objKey := range tempObjects {
	//		part, err := s.bucket.UploadPartCopy(
	//			imurFinal,
	//			s.bucket.BucketName,
	//			objKey,
	//			0,
	//			0,
	//			i+1,
	//		)
	//		if err != nil {
	//			finalErr = err
	//			su_logger.Error(ctx, err, "复制最终分段失败", su_logger.E().String("key", objKey))
	//			break
	//		}
	//		finalParts[i] = part
	//	}
	//
	//	// 如果最终合并过程中出现错误，清理资源并返回错误
	//	if finalErr != nil {
	//		// 中止分段上传
	//		err = s.bucket.AbortMultipartUpload(imurFinal)
	//		if err != nil {
	//			su_logger.Warn(ctx, "中止分段上传失败", su_logger.E().String("key", objectName).Error(err))
	//		}
	//
	//		// 清理临时对象
	//		for _, objKey := range tempObjects {
	//			cleanErr := s.bucket.DeleteObject(objKey)
	//			if cleanErr != nil {
	//				su_logger.Warn(ctx, "清理临时对象失败", su_logger.E().String("key", objKey).Error(cleanErr))
	//			}
	//		}
	//
	//		return nil, fmt.Errorf("复制最终分段失败: %w", finalErr)
	//	}
	//
	//	// 完成最终合并
	//	_, err = s.bucket.CompleteMultipartUpload(imurFinal, finalParts)
	//	if err != nil {
	//		su_logger.Error(ctx, err, "完成最终分段上传失败", su_logger.E().String("objName", objectName))
	//
	//		// 清理临时对象
	//		for _, objKey := range tempObjects {
	//			cleanErr := s.bucket.DeleteObject(objKey)
	//			if cleanErr != nil {
	//				su_logger.Warn(ctx, "清理临时对象失败", su_logger.E().String("key", objKey).Error(cleanErr))
	//			}
	//		}
	//
	//		return nil, fmt.Errorf("完成最终分段上传失败: %w", err)
	//	}
	//
	//	// 清理临时对象
	//	for _, objKey := range tempObjects {
	//		err := s.bucket.DeleteObject(objKey)
	//		if err != nil {
	//			su_logger.Warn(ctx, "删除临时对象失败", su_logger.E().String("key", objKey).Error(err))
	//		}
	//	}
	//
	//	// 获取最终对象的元数据
	//	meta, err := s.bucket.GetObjectDetailedMeta(objectName)
	//	if err != nil {
	//		su_logger.Error(ctx, err, "获取最终对象元数据失败", su_logger.E().String("key", objectName))
	//		return nil, fmt.Errorf("获取最终对象元数据失败: %w", err)
	//	}
	//
	//	// 设置内容类型
	//	if len(attrs) > 0 && attrs[0].ContentType != "" {
	//		err = s.bucket.SetObjectMeta(objectName, oss.Meta("ContentType", attrs[0].ContentType))
	//		if err != nil {
	//			su_logger.Warn(ctx, "设置对象内容类型失败", su_logger.E().String("key", objectName).Error(err))
	//		}
	//	}
	//
	//	return &ObjectAttr{
	//		Name:        objectName,
	//		Size:        cast.ToInt64(meta.Get("Content-Length")),
	//		ContentType: meta.Get("Content-Type"),
	//		Metadata:    map[string]string{"ETag": meta.Get("ETag")},
	//	}, nil
	//}

	// 少于32个分片直接合并
	imur, err := s.bucket.InitiateMultipartUpload(objectName)
	if err != nil {
		su_logger.Error(ctx, err, "初始化分段上传失败", su_logger.E().String("objName", objectName))
		return nil, fmt.Errorf("初始化分段上传失败: %w", err)
	}

	// 准备分段上传的部分
	uploadParts := make([]oss.UploadPart, len(objects))
	var copyErr error
	for i, obj := range objects {
		var sourceKey string
		switch obj.(type) {
		case *ObjectAttr:
			sourceKey = objects[i].(*ObjectAttr).Name
		default:
			copyErr = fmt.Errorf("不支持的对象类型: %T at index %d", obj, i)
			break
		}

		if copyErr != nil {
			break
		}

		part, err := s.bucket.UploadPartCopy(
			imur,
			s.bucket.BucketName,
			sourceKey,
			0,
			obj.(*ObjectAttr).Size,
			i+1,
		)
		if err != nil {
			copyErr = err
			su_logger.Error(ctx, err, "复制分段失败", su_logger.E().String("key", sourceKey))
			break
		}

		uploadParts[i] = part
	}

	// 如果复制过程中出现错误，中止分段上传并返回错误
	if copyErr != nil {
		err := s.bucket.AbortMultipartUpload(imur)
		if err != nil {
			su_logger.Warn(ctx, "中止分段上传失败", su_logger.E().String("key", objectName).Error(err))
		}
		return nil, fmt.Errorf("复制分段失败: %w", copyErr)
	}

	// 完成分段上传
	_, err = s.bucket.CompleteMultipartUpload(imur, uploadParts)
	if err != nil {
		su_logger.Error(ctx, err, "完成分段上传失败", su_logger.E().String("objName", objectName))
		return nil, fmt.Errorf("完成分段上传失败: %w", err)
	}

	// 设置内容类型
	if len(attrs) > 0 && attrs[0].ContentType != "" {
		err = s.bucket.SetObjectMeta(objectName, oss.Meta("ContentType", attrs[0].ContentType))
		if err != nil {
			su_logger.Warn(ctx, "设置对象内容类型失败", su_logger.E().String("key", objectName).Error(err))
		}
	}

	// 获取对象属性
	props, err := s.bucket.GetObjectDetailedMeta(objectName)
	if err != nil {
		su_logger.Error(ctx, err, "获取对象元数据失败", su_logger.E().String("key", objectName))
		return nil, fmt.Errorf("获取对象元数据失败: %w", err)
	}

	return &ObjectAttr{
		Name:        objectName,
		Size:        cast.ToInt64(props.Get("Content-Length")),
		ContentType: props.Get("Content-Type"),
		Metadata: map[string]string{
			"frameCount": props.Get("frameCount"),
			"resolution": props.Get("resolution"),
			"ETag":       props.Get("ETag"),
		},
		CRC64C: props.Get("X-Oss-Hash-Crc64ecma"),
	}, nil

}

func (s *OssStorage) GetReader(ctx context.Context, objName string) (io.ReadCloser, error) {
	reader, err := s.bucket.GetObject(objName)
	if err != nil {
		return nil, err
	}
	return reader, nil
}
