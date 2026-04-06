package cloud_storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/huaweicloud/huaweicloud-sdk-go-obs/obs"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
	"golang.org/x/sync/errgroup"
)

type ObsStorage struct {
	client *obs.ObsClient
	bucket *obs.Bucket
}

func NewObsStorage(client *obs.ObsClient, bucket *obs.Bucket) CloudStorage {
	return &ObsStorage{
		client: client,
		bucket: bucket,
	}
}

func (s *ObsStorage) ReadObjectToBuff(ctx context.Context, objectName string, buff *bytes.Buffer) (err error) {
	output, err := s.client.GetObject(&obs.GetObjectInput{
		GetObjectMetadataInput: obs.GetObjectMetadataInput{
			Bucket: s.bucket.Name,
			Key:    objectName,
		},
	})
	if err != nil {
		return err
	}
	_, err = io.Copy(buff, output.Body)
	return err
}

func (s *ObsStorage) ReadObjectToWriter(ctx context.Context, objName string, writer io.Writer) (err error) {
	output, err := s.client.GetObject(&obs.GetObjectInput{
		GetObjectMetadataInput: obs.GetObjectMetadataInput{
			Bucket: s.bucket.Name,
			Key:    objName,
		},
	})
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, output.Body)
	return err
}

func (s *ObsStorage) UploadObjectFromBuff(ctx context.Context, objName string, buff *bytes.Buffer, attrs ...ObjectAttr) (err error) {
	input := &obs.PutObjectInput{
		PutObjectBasicInput: obs.PutObjectBasicInput{
			ObjectOperationInput: obs.ObjectOperationInput{
				Bucket: s.bucket.Name,
				Key:    objName,
			},
		},
	}
	if len(attrs) > 0 {
		input.HttpHeader = obs.HttpHeader{
			ContentType: attrs[0].ContentType,
		}
	}
	_, err = s.client.PutObject(input)
	return err
}

func (s *ObsStorage) UploadFromReader(ctx context.Context, objName string, reader io.Reader, attrs ...ObjectAttr) (err error) {
	input := &obs.PutObjectInput{
		PutObjectBasicInput: obs.PutObjectBasicInput{
			ObjectOperationInput: obs.ObjectOperationInput{
				Bucket: s.bucket.Name,
				Key:    objName,
			},
		},
	}
	if len(attrs) > 0 {
		input.HttpHeader = obs.HttpHeader{
			ContentType: attrs[0].ContentType,
		}
	}
	_, err = s.client.PutObject(input)
	return err
}

func (s *ObsStorage) Client() *StorageClient {
	return &StorageClient{
		bucket: s.bucket,
		client: s.client,
	}
}

// UploadObject 上传对象
func (s *ObsStorage) UploadObject(ctx context.Context, objectName string, file []byte, attrs ...ObjectAttr) error {
	var contentType string
	var metadata map[string]string
	for _, attr := range attrs {
		if attr.ContentType != "" {
			contentType = attr.ContentType
		}
		if attr.Metadata != nil {
			metadata = attr.Metadata
		}
	}
	input := &obs.PutObjectInput{
		PutObjectBasicInput: obs.PutObjectBasicInput{
			ObjectOperationInput: obs.ObjectOperationInput{
				Bucket:   s.bucket.Name,
				Key:      objectName,
				Metadata: metadata,
			},
			HttpHeader: obs.HttpHeader{
				ContentType: contentType,
			},
		},
		Body: bytes.NewReader(file),
	}
	if contentType != "" {
		input.ContentType = contentType
	}
	_, err := s.client.PutObject(input)
	return err
}

func fileToBytes(file *os.File) ([]byte, error) {
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	size := fileInfo.Size()
	byt := make([]byte, size)
	_, err = io.ReadFull(file, byt)
	if err != nil {
		return nil, err
	}
	return byt, nil
}

// UploadObjectFromFile 上传对象通过文件
func (s *ObsStorage) UploadObjectFromFile(ctx context.Context, objectName string, file *os.File, attrs ...ObjectAttr) error {
	fileContent, err := fileToBytes(file)
	if err != nil {
		return err
	}
	return s.UploadObject(ctx, objectName, fileContent, attrs...)
}

// ReadObject 读取对象
func (s *ObsStorage) ReadObject(ctx context.Context, objectName string) ([]byte, error) {
	objectName = getPathWithoutSlash(objectName)
	output, err := s.client.GetObject(&obs.GetObjectInput{
		GetObjectMetadataInput: obs.GetObjectMetadataInput{
			Bucket: s.bucket.Name,
			Key:    objectName,
		},
	})
	if err != nil {
		return nil, err
	}
	return io.ReadAll(output.Body)
}

// IsObjectExists 判断对象是否存在于存储桶中
func (s *ObsStorage) IsObjectExists(ctx context.Context, objectName string) bool {
	output, err := s.client.GetObjectMetadata(&obs.GetObjectMetadataInput{
		Bucket: s.bucket.Name,
		Key:    objectName,
	})
	if err != nil {
		return false
	}
	if output.ContentLength == 0 {
		return false
	}
	return true
}

// GetObjectAttrs 获取文件属性
func (s *ObsStorage) GetObjectAttrs(ctx context.Context, objectName string) (*ObjectAttr, error) {
	output, err := s.client.GetObjectMetadata(&obs.GetObjectMetadataInput{
		Bucket: s.bucket.Name,
		Key:    objectName,
	})
	if err != nil {
		return nil, err
	}
	return &ObjectAttr{
		Bucket:      s.bucket.Name,
		Name:        objectName,
		ContentType: output.ContentType,
		Size:        output.ContentLength,
		Etag:        output.ETag,
		Metadata:    output.Metadata,
	}, nil
}

// BatchGetObjectAttrs 获取文件属性
func (s *ObsStorage) BatchGetObjectAttrs(ctx context.Context, objectNames []string) ([]*ObjectAttr, error) {
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
func (s *ObsStorage) SetObjectAttrs(ctx context.Context, objectName string, attrs map[string]string) error {
	_, err := s.client.SetObjectMetadata(&obs.SetObjectMetadataInput{
		Bucket:   s.bucket.Name,
		Key:      objectName,
		Metadata: attrs,
		HttpHeader: obs.HttpHeader{
			ContentType: attrs["ContentType"],
		},
	})
	return err
}

// GetObjectSize 获取对象大小
func (s *ObsStorage) GetObjectSize(ctx context.Context, objectName string) int64 {
	attr, err := s.GetObjectAttrs(ctx, objectName)
	if err != nil {
		return 0
	}
	return attr.Size
}

// BatchGetPreSignatures 批量获取预签名
func (s *ObsStorage) BatchGetPreSignatures(prefix string, file File, items []ShardItem) (rs SignRs, err error) {
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
func (s *ObsStorage) GetPreSignature(bucket Client, objectName string, file File) (string, error) {
	// 创建签名 URL 的参数
	input := &obs.CreateSignedUrlInput{
		Method:  obs.HttpMethodPut,
		Bucket:  bucket.ToObsBucket().Name,
		Key:     objectName,
		Expires: int(PresignedExpires.Seconds()),
		Headers: map[string]string{},
	}
	// 添加 ContentType 参数
	if file.ContentType != "" {
		input.Headers["Content-Type"] = file.ContentType
	}
	// 生成签名 URL
	output, err := s.client.CreateSignedUrl(input)
	if err != nil {
		return "", fmt.Errorf("failed to create signed URL: %w", err)
	}
	return output.SignedUrl, nil
}

// MergeFileParts 合并文件分片
func (s *ObsStorage) MergeFileParts(record *MergeFileParts, param MergeFileParam) (string, error) {
	return "", nil
}

// MoveObject 移动对象
func (s *ObsStorage) MoveObject(ctx context.Context, objectName string, newBucket Client, newObjectName string) error {
	_, err := s.client.CopyObject(&obs.CopyObjectInput{
		ObjectOperationInput: obs.ObjectOperationInput{
			Bucket: newBucket.ToObsBucket().Name,
			Key:    newObjectName,
		},
		CopySourceBucket: s.bucket.Name,
		CopySourceKey:    objectName,
	})
	if err != nil {
		return err
	}
	_, err = s.client.DeleteObject(&obs.DeleteObjectInput{
		Bucket: s.bucket.Name,
		Key:    objectName,
	})
	return err
}

// DeleteObject 删除对象
func (s *ObsStorage) DeleteObject(ctx context.Context, objectName string) error {
	_, err := s.client.DeleteObject(&obs.DeleteObjectInput{
		Bucket: s.bucket.Name,
		Key:    objectName,
	})
	return err
}

// SignedURl(ctx context.Context, bucket Client, objectName string, opt *SignedOptions) (string, error)
func (s *ObsStorage) SignedURl(ctx context.Context, bucket Client, objectName string, opt *SignedOptions) (string, error) {
	method := opt.GetMethod()
	option := &obs.CreateSignedUrlInput{
		Method:      obs.HttpMethodType(method),
		Bucket:      bucket.ToObsBucket().Name,
		Key:         objectName,
		Policy:      "",
		SubResource: "",
		Expires:     int(opt.ExpireAt.Unix()),
		Headers:     map[string]string{},
		QueryParams: map[string]string{},
	}
	if opt.ContentType != "" {
		option.Headers["Content-Type"] = opt.ContentType
	}
	output, err := s.client.CreateSignedUrl(option)
	if err != nil {
		return "", err
	}
	return output.SignedUrl, nil
}

// 合并分片
func (s *ObsStorage) ComposerAndRun(ctx context.Context, objectName string, objects []interface{}, attrs ...ObjectAttr) (*ObjectAttr, error) {
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
		_, err := s.client.CopyObject(&obs.CopyObjectInput{
			ObjectOperationInput: obs.ObjectOperationInput{
				Bucket: s.bucket.Name,
				Key:    objectName,
			},
			CopySourceBucket: s.bucket.Name,
			CopySourceKey:    sourceKey,
		})
		if err != nil {
			su_logger.Error(ctx, err, "复制对象失败", su_logger.E().String("sourceKey", sourceKey).String("targetKey", objectName))
			return nil, fmt.Errorf("复制对象失败: %w", err)
		}

		// 删除源对象
		_, err = s.client.DeleteObject(&obs.DeleteObjectInput{
			Bucket: s.bucket.Name,
			Key:    sourceKey,
		})
		if err != nil {
			su_logger.Warn(ctx, "删除源对象失败", su_logger.E().String("key", sourceKey).Error(err))
			// 不返回错误，因为复制已经成功
		}

		// 获取新对象的属性
		output, err := s.client.GetObjectMetadata(&obs.GetObjectMetadataInput{
			Bucket: s.bucket.Name,
			Key:    objectName,
		})
		if err != nil {
			su_logger.Error(ctx, err, "获取对象元数据失败", su_logger.E().String("key", objectName))
			return nil, fmt.Errorf("获取对象元数据失败: %w", err)
		}

		// 设置内容类型
		if len(attrs) > 0 && attrs[0].ContentType != "" {
			_, err = s.client.SetObjectMetadata(&obs.SetObjectMetadataInput{
				Bucket: s.bucket.Name,
				Key:    objectName,
				HttpHeader: obs.HttpHeader{
					ContentType: attrs[0].ContentType,
				},
			})
			if err != nil {
				su_logger.Warn(ctx, "设置对象内容类型失败", su_logger.E().String("key", objectName).Error(err))
			}
		}

		return &ObjectAttr{
			Name:        objectName,
			Size:        output.ContentLength,
			ContentType: output.ContentType,
			Metadata:    output.Metadata,
		}, nil
	}

	// 如果超过32个分片，分批次合并
	if len(objects) > MaxChunk {
		// 使用errgroup来管理goroutine和错误
		eg, egCtx := errgroup.WithContext(ctx)

		// 创建一个唯一的前缀，避免命名冲突
		uniquePrefix := fmt.Sprintf("%s.%d.", objectName, time.Now().UnixNano())

		// 计算需要多少批次
		batchSize := MaxChunk
		batchCount := (len(objects) + batchSize - 1) / batchSize
		tempObjects := make([]*struct {
			Bucket string
			Key    string
		}, batchCount)

		// 分批并发合并
		for i := 0; i < len(objects); i += batchSize {
			end := i + batchSize
			if end > len(objects) {
				end = len(objects)
			}

			// 创建临时对象名，使用唯一前缀
			tempName := uniquePrefix + fmt.Sprintf("temp.%d", i/batchSize)
			currentBatch := objects[i:end]
			batchIdx := i / batchSize

			eg.Go(func() error {
				// 创建分段上传任务
				initOutput, err := s.client.InitiateMultipartUpload(&obs.InitiateMultipartUploadInput{
					ObjectOperationInput: obs.ObjectOperationInput{
						Bucket: s.bucket.Name,
						Key:    tempName,
					},
				})
				if err != nil {
					su_logger.Error(egCtx, err, "初始化分段上传失败", su_logger.E().String("objName", tempName))
					return fmt.Errorf("初始化分段上传失败: %w", err)
				}

				// 准备分段上传的部分
				parts := make([]obs.Part, len(currentBatch))
				for j, obj := range currentBatch {
					var sourceKey string
					switch o := obj.(type) {
					case *obs.AppendObjectInput:
						sourceKey = o.Key
					default:
						return fmt.Errorf("不支持的对象类型: %T", obj)
					}

					// 获取对象元数据
					_, err := s.client.GetObjectMetadata(&obs.GetObjectMetadataInput{
						Bucket: s.bucket.Name,
						Key:    sourceKey,
					})
					if err != nil {
						su_logger.Error(egCtx, err, "获取对象元数据失败", su_logger.E().String("key", sourceKey))
						return fmt.Errorf("获取对象元数据失败: %w", err)
					}

					// 上传分段
					copyOutput, err := s.client.CopyPart(&obs.CopyPartInput{
						Bucket:           s.bucket.Name,
						Key:              tempName,
						UploadId:         initOutput.UploadId,
						PartNumber:       j + 1,
						CopySourceBucket: s.bucket.Name,
						CopySourceKey:    sourceKey,
					})
					if err != nil {
						su_logger.Error(egCtx, err, "复制分段失败", su_logger.E().String("key", sourceKey))
						return fmt.Errorf("复制分段失败: %w", err)
					}

					parts[j] = obs.Part{
						PartNumber: j + 1,
						ETag:       copyOutput.ETag,
					}
				}

				// 完成分段上传
				_, err = s.client.CompleteMultipartUpload(&obs.CompleteMultipartUploadInput{
					Bucket:   s.bucket.Name,
					Key:      tempName,
					UploadId: initOutput.UploadId,
					Parts:    parts,
				})
				if err != nil {
					su_logger.Error(egCtx, err, "完成分段上传失败", su_logger.E().String("objName", tempName))
					return fmt.Errorf("完成分段上传失败: %w", err)
				}

				// 保存临时对象
				tempObjects[batchIdx] = &struct {
					Bucket string
					Key    string
				}{
					Bucket: s.bucket.Name,
					Key:    tempName,
				}
				return nil
			})
		}

		// 等待所有合并完成，如果有错误则返回
		if err := eg.Wait(); err != nil {
			// 清理已创建的临时对象
			for _, obj := range tempObjects {
				if obj != nil {
					_, cleanErr := s.client.DeleteObject(&obs.DeleteObjectInput{
						Bucket: s.bucket.Name,
						Key:    obj.Key,
					})
					if cleanErr != nil {
						su_logger.Warn(ctx, "清理临时对象失败", su_logger.E().String("key", obj.Key).Error(cleanErr))
					}
				}
			}
			return nil, err
		}

		// 最终合并临时对象
		initOutput, err := s.client.InitiateMultipartUpload(&obs.InitiateMultipartUploadInput{
			ObjectOperationInput: obs.ObjectOperationInput{
				Bucket: s.bucket.Name,
				Key:    objectName,
			},
		})
		if err != nil {
			// 清理临时对象
			for _, obj := range tempObjects {
				_, cleanErr := s.client.DeleteObject(&obs.DeleteObjectInput{
					Bucket: s.bucket.Name,
					Key:    obj.Key,
				})
				if cleanErr != nil {
					su_logger.Warn(ctx, "清理临时对象失败", su_logger.E().String("key", obj.Key).Error(cleanErr))
				}
			}
			su_logger.Error(ctx, err, "初始化最终分段上传失败", su_logger.E().String("objName", objectName))
			return nil, fmt.Errorf("初始化最终分段上传失败: %w", err)
		}

		// 准备最终合并的部分
		finalParts := make([]obs.Part, len(tempObjects))
		var finalErr error
		for i, obj := range tempObjects {
			copyOutput, err := s.client.CopyPart(&obs.CopyPartInput{
				Bucket:           s.bucket.Name,
				Key:              objectName,
				UploadId:         initOutput.UploadId,
				PartNumber:       i + 1,
				CopySourceBucket: s.bucket.Name,
				CopySourceKey:    obj.Key,
			})
			if err != nil {
				finalErr = err
				su_logger.Error(ctx, err, "复制最终分段失败", su_logger.E().String("key", obj.Key))
				break
			}

			finalParts[i] = obs.Part{
				PartNumber: i + 1,
				ETag:       copyOutput.ETag,
			}
		}

		// 如果最终合并过程中出现错误，清理资源并返回错误
		if finalErr != nil {
			// 尝试中止分段上传
			_, abortErr := s.client.AbortMultipartUpload(&obs.AbortMultipartUploadInput{
				Bucket:   s.bucket.Name,
				Key:      objectName,
				UploadId: initOutput.UploadId,
			})
			if abortErr != nil {
				su_logger.Warn(ctx, "中止分段上传失败", su_logger.E().String("key", objectName).Error(abortErr))
			}

			// 清理临时对象
			for _, obj := range tempObjects {
				_, cleanErr := s.client.DeleteObject(&obs.DeleteObjectInput{
					Bucket: s.bucket.Name,
					Key:    obj.Key,
				})
				if cleanErr != nil {
					su_logger.Warn(ctx, "清理临时对象失败", su_logger.E().String("key", obj.Key).Error(cleanErr))
				}
			}

			return nil, fmt.Errorf("复制最终分段失败: %w", finalErr)
		}

		// 完成最终合并
		_, err = s.client.CompleteMultipartUpload(&obs.CompleteMultipartUploadInput{
			Bucket:   s.bucket.Name,
			Key:      objectName,
			UploadId: initOutput.UploadId,
			Parts:    finalParts,
		})
		if err != nil {
			su_logger.Error(ctx, err, "完成最终分段上传失败", su_logger.E().String("objName", objectName))

			// 清理临时对象
			for _, obj := range tempObjects {
				_, cleanErr := s.client.DeleteObject(&obs.DeleteObjectInput{
					Bucket: s.bucket.Name,
					Key:    obj.Key,
				})
				if cleanErr != nil {
					su_logger.Warn(ctx, "清理临时对象失败", su_logger.E().String("key", obj.Key).Error(cleanErr))
				}
			}

			return nil, fmt.Errorf("完成最终分段上传失败: %w", err)
		}

		// 清理临时对象
		for _, obj := range tempObjects {
			_, err := s.client.DeleteObject(&obs.DeleteObjectInput{
				Bucket: s.bucket.Name,
				Key:    obj.Key,
			})
			if err != nil {
				su_logger.Warn(ctx, "删除临时对象失败", su_logger.E().String("key", obj.Key).Error(err))
			}
		}

		// 获取最终对象的属性
		output, err := s.client.GetObjectMetadata(&obs.GetObjectMetadataInput{
			Bucket: s.bucket.Name,
			Key:    objectName,
		})
		if err != nil {
			su_logger.Error(ctx, err, "获取最终对象元数据失败", su_logger.E().String("key", objectName))
			return nil, fmt.Errorf("获取最终对象元数据失败: %w", err)
		}

		// 设置内容类型
		if len(attrs) > 0 && attrs[0].ContentType != "" {
			_, err = s.client.SetObjectMetadata(&obs.SetObjectMetadataInput{
				Bucket: s.bucket.Name,
				Key:    objectName,
				HttpHeader: obs.HttpHeader{
					ContentType: attrs[0].ContentType,
				},
			})
			if err != nil {
				su_logger.Warn(ctx, "设置对象内容类型失败", su_logger.E().String("key", objectName).Error(err))
			}
		}

		return &ObjectAttr{
			Name:        objectName,
			Size:        output.ContentLength,
			ContentType: output.ContentType,
			Metadata:    output.Metadata,
		}, nil
	}

	// 少于32个分片直接合并
	initOutput, err := s.client.InitiateMultipartUpload(&obs.InitiateMultipartUploadInput{
		ObjectOperationInput: obs.ObjectOperationInput{
			Bucket: s.bucket.Name,
			Key:    objectName,
		},
	})
	if err != nil {
		su_logger.Error(ctx, err, "初始化分段上传失败", su_logger.E().String("objName", objectName))
		return nil, fmt.Errorf("初始化分段上传失败: %w", err)
	}

	// 准备分段上传的部分
	parts := make([]obs.Part, len(objects))
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

		copyOutput, err := s.client.CopyPart(&obs.CopyPartInput{
			Bucket:           s.bucket.Name,
			Key:              objectName,
			UploadId:         initOutput.UploadId,
			PartNumber:       i + 1,
			CopySourceBucket: s.bucket.Name,
			CopySourceKey:    sourceKey,
		})
		if err != nil {
			copyErr = err
			su_logger.Error(ctx, err, "复制分段失败", su_logger.E().String("key", sourceKey))
			break
		}

		parts[i] = obs.Part{
			PartNumber: i + 1,
			ETag:       copyOutput.ETag,
		}
	}

	// 如果复制过程中出现错误，中止分段上传并返回错误
	if copyErr != nil {
		_, abortErr := s.client.AbortMultipartUpload(&obs.AbortMultipartUploadInput{
			Bucket:   s.bucket.Name,
			Key:      objectName,
			UploadId: initOutput.UploadId,
		})
		if abortErr != nil {
			su_logger.Warn(ctx, "中止分段上传失败", su_logger.E().String("key", objectName).Error(abortErr))
		}
		return nil, fmt.Errorf("复制分段失败: %w", copyErr)
	}

	// 完成分段上传
	_, err = s.client.CompleteMultipartUpload(&obs.CompleteMultipartUploadInput{
		Bucket:   s.bucket.Name,
		Key:      objectName,
		UploadId: initOutput.UploadId,
		Parts:    parts,
	})
	if err != nil {
		su_logger.Error(ctx, err, "完成分段上传失败", su_logger.E().String("objName", objectName))
		return nil, fmt.Errorf("完成分段上传失败: %w", err)
	}

	// 设置内容类型
	if len(attrs) > 0 && attrs[0].ContentType != "" {
		_, err = s.client.SetObjectMetadata(&obs.SetObjectMetadataInput{
			Bucket: s.bucket.Name,
			Key:    objectName,
			HttpHeader: obs.HttpHeader{
				ContentType: attrs[0].ContentType,
			},
		})
		if err != nil {
			su_logger.Warn(ctx, "设置对象内容类型失败", su_logger.E().String("key", objectName).Error(err))
		}
	}

	// 获取对象属性
	output, err := s.client.GetObjectMetadata(&obs.GetObjectMetadataInput{
		Bucket: s.bucket.Name,
		Key:    objectName,
	})
	if err != nil {
		su_logger.Error(ctx, err, "获取对象元数据失败", su_logger.E().String("key", objectName))
		return nil, fmt.Errorf("获取对象元数据失败: %w", err)
	}

	return &ObjectAttr{
		Name:        objectName,
		Size:        output.ContentLength,
		ContentType: output.ContentType,
		Metadata:    output.Metadata,
	}, nil
}

func (s *ObsStorage) GetReader(ctx context.Context, objName string) (io.ReadCloser, error) {
	output, err := s.client.GetObject(&obs.GetObjectInput{
		GetObjectMetadataInput: obs.GetObjectMetadataInput{
			Bucket: s.bucket.Name,
			Key:    objName,
		},
	})
	if err != nil {
		return nil, err
	}
	return output.Body, nil
}
