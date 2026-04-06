package cloud_storage

import (
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"time"

	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/google"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
	"golang.org/x/sync/errgroup"
)

type GcpManager struct {
	GcpStorage
	ResourceSdk *ResourceSdk
}

type GcpManagerParam struct {
	Option       *GcpOption
	Region       Region
	ServiceName  string
	ResourceType ResourceType
	SecretKey    string
	UseProxy     bool
}

func NewGcpManager(bucket *google.Storage, param GcpManagerParam) CloudStorage {
	var preSignature GcsPreSignature
	var speedUps map[string]string
	if param.Option != nil {
		preSignature = param.Option.PreSignature
		speedUps = param.Option.SpeedUps
	}
	sdk := NewResourceSdk(param.Region, param.ServiceName, param.ResourceType, param.SecretKey, ResourceSdkOption{
		UseProxy: param.UseProxy,
	})
	return &GcpManager{
		GcpStorage: GcpStorage{
			bucket:       bucket,
			preSignature: preSignature,
			speedUps:     speedUps,
		},
		ResourceSdk: sdk,
	}
}

func (s *GcpManager) UploadObject(ctx context.Context, objectName string, file []byte, attrs ...ObjectAttr) error {
	return s.ResourceSdk.UploadObject(ctx, objectName, file, attrs...)
}

func (s *GcpManager) ReadObject(ctx context.Context, objectName string) ([]byte, error) {
	return s.ResourceSdk.ReadObject(ctx, objectName)
}

func (s *GcpManager) GetObjectAttrs(ctx context.Context, objectName string) (*ObjectAttr, error) {
	return s.ResourceSdk.GetObjectAttrs(ctx, objectName)
}

func (s *GcpManager) DeleteObject(ctx context.Context, objectName string) error {
	return s.ResourceSdk.DeleteObject(ctx, objectName)
}

func (s *GcpManager) IsObjectExists(ctx context.Context, objectName string) bool {
	_, err := s.GetObjectAttrs(ctx, objectName)
	if err != nil {
		return false
	}
	return true
}

func (s *GcpManager) GetObjectSize(ctx context.Context, objectName string) int64 {
	attrs, err := s.GetObjectAttrs(ctx, objectName)
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

func (s *GcpManager) BatchGetObjectAttrs(ctx context.Context, objectNames []string) ([]*ObjectAttr, error) {
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

func (s *GcpManager) SetObjectAttrs(ctx context.Context, objectName string, attrs map[string]string) error {
	return s.ResourceSdk.SetAttrs(ctx, objectName, attrs)
}

func (s *GcpManager) GetPreSignature(bucket Client, objectName string, file File) (string, error) {
	rsp, err := s.ResourceSdk.GetPreSign(context.Background(), PreParam{
		Parts: []PrePart{
			{
				FileName: objectName,
				FileSize: int(file.Size),
				FileOption: PreSignOption{
					ContentType: file.ContentType,
					Method:      file.Method,
					Expires:     file.Expires.Unix(),
					Category:    file.Category,
				},
			},
		},
	})
	if err != nil {
		return "", err
	}
	if len(rsp.Data) == 0 {
		return "", errors.New("pre signature is empty")
	}
	if file.Method == "GET" {
		return rsp.Data[0].Sign, nil
	}
	return s.toSpeedUp(rsp.Data[0].Sign), nil
}

func (s *GcpManager) UploadFromReader(ctx context.Context, objectName string, reader io.Reader, attrs ...ObjectAttr) (err error) {
	// todo
	return nil
}

func (s *GcpManager) BatchGetPreSignatures(prefix string, file File, items []ShardItem) (rs SignRs, err error) {
	rs.Items = make([]SignItem, len(items))
	for i, item := range items {
		objectName := prefix + GenPartName(file.Id, file.Ext, strconv.Itoa(i+1))
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

func (s *GcpManager) UploadObjectFromFile(ctx context.Context, objectName string, file *os.File, attrs ...ObjectAttr) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	return s.UploadObject(ctx, objectName, data, attrs...)
}

func (s *GcpManager) MergeFileParts(record *MergeFileParts, param MergeFileParam) (string, error) {
	str, err := s.GcpStorage.MergeFileParts(record, param)
	if err != nil {
		return "", err
	}
	_, _ = s.ResourceSdk.createTrace(context.Background(), createTraceParam{
		Path:     record.Url,
		CreateAt: time.Now().UnixMilli(),
		UpdateAt: time.Now().UnixMilli(),
		ByteSize: record.FileSize,
	})
	return str, nil
}

func (s *GcpManager) ComposerAndRun(ctx context.Context, objectName string, objects []interface{}, attrs ...ObjectAttr) (*ObjectAttr, error) {
	attr, err := s.GcpStorage.ComposerAndRun(ctx, objectName, objects, attrs...)
	if err != nil {
		return nil, err
	}
	_, _ = s.ResourceSdk.createTrace(context.Background(), createTraceParam{
		Path:     objectName,
		CreateAt: time.Now().UnixMilli(),
		UpdateAt: time.Now().UnixMilli(),
		ByteSize: attr.Size,
	})
	return attr, nil
}

func (s *GcpManager) MoveObject(ctx context.Context, objectName string, newBucket Client, newObjectName string) error {
	return s.ResourceSdk.MoveObject(ctx, s.bucket.Name(), objectName, newBucket.ToGcs().Name(), newObjectName)
}

func (s *GcpManager) GetReader(ctx context.Context, objName string) (io.ReadCloser, error) {
	return s.GcpStorage.GetReader(ctx, objName)
}
