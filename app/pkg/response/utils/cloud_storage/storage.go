package cloud_storage

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/huaweicloud/huaweicloud-sdk-go-obs/obs"
	"github.com/minio/minio-go/v7"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/google"
	"golang.org/x/crypto/ed25519"
	"google.golang.org/api/googleapi"
)

type CloudStorage interface {
	// Client 获取客户端
	Client() *StorageClient

	// UploadObject 上传对象
	UploadObject(ctx context.Context, objectName string, file []byte, attrs ...ObjectAttr) error

	// UploadObjectFromFile 上传对象通过文件
	UploadObjectFromFile(ctx context.Context, objectName string, file *os.File, attrs ...ObjectAttr) error

	// ReadObject 读取对象
	ReadObject(ctx context.Context, objectName string) ([]byte, error)
	//ReadObjectToBuff(ctx context.Context, objectName string, buff *bytes.Buffer) (err error)
	//ReadObjectToWriter(ctx context.Context, objName string, writer io.Writer) (err error)
	//UploadObjectFromBuff(ctx context.Context, objName string, buff *bytes.Buffer, attrs ...ObjectAttr) (err error)
	UploadFromReader(ctx context.Context, objName string, reader io.Reader, attrs ...ObjectAttr) (err error)

	// IsObjectExists 对象是否存在
	IsObjectExists(ctx context.Context, objectName string) bool

	// GetObjectAttrs 获取对象属性
	GetObjectAttrs(ctx context.Context, objectName string) (*ObjectAttr, error)

	// BatchGetObjectAttrs 批量获取对象属性
	BatchGetObjectAttrs(ctx context.Context, objectNames []string) ([]*ObjectAttr, error)

	// SetObjectAttrs 设置对象属性
	SetObjectAttrs(ctx context.Context, objectName string, attrs map[string]string) error

	// GetObjectSize 获取对象size
	GetObjectSize(ctx context.Context, objectName string) int64

	// BatchGetPreSignatures 批量获取预签名
	BatchGetPreSignatures(prefix string, file File, items []ShardItem) (rs SignRs, err error)

	// GetPreSignature 获取预签名
	GetPreSignature(bucket Client, objectName string, file File) (string, error)

	// MergeFileParts 合并文件分片
	MergeFileParts(record *MergeFileParts, param MergeFileParam) (string, error)

	// MoveObject 移动对象
	MoveObject(ctx context.Context, objectName string, newBucket Client, newObjectName string) error

	// DeleteObject 删除对象
	DeleteObject(ctx context.Context, objectName string) error
	//SignedURl(ctx context.Context, bucket Client, objectName string, opt *SignedOptions) (string, error)

	// ComposerAndRun 分片打包
	ComposerAndRun(ctx context.Context, objectName string, objects []interface{}, attrs ...ObjectAttr) (*ObjectAttr, error)
	GetReader(ctx context.Context, objName string) (io.ReadCloser, error)
}

type SignedOptions struct {
	ContentType string
	ExpireAt    time.Time
	// 签名的http方法 default: PUT
	Method string
}

func (s *SignedOptions) GetMethod() string {
	if s.Method == "" {
		return "PUT"
	}
	return s.Method
}

type Client interface {
	ToGcs() *google.Storage
	ToObs() *obs.ObsClient
	ToObsBucket() *obs.Bucket
	ToMinio() *minio.Client
}

type StorageClient struct {
	client interface{}
	bucket interface{}
}

func (s *StorageClient) ToGcs() *google.Storage {
	return s.bucket.(*google.Storage)
}

func (s *StorageClient) ToObs() *obs.ObsClient {
	return s.client.(*obs.ObsClient)
}

func (s *StorageClient) ToObsBucket() *obs.Bucket {
	return s.bucket.(*obs.Bucket)
}

func (s *StorageClient) ToMinio() *minio.Client {
	return s.bucket.(*minio.Client)
}

const (
	MaxChunk         = 32            // 最大分片数量
	PresignedExpires = 3 * time.Hour // 签名有效期 3 小时
)

// 文件元信息 Key
const (
	ContentType   = "Content-Type"
	ContentLength = "Content-Length"
)

var (
	NotFoundErr = errors.New("not found")
)

type File struct {
	Id          string
	Ext         string
	Size        int64
	ContentType string
	Method      string
	Expires     time.Time
	Category    int
}

type Resource struct {
	B   []byte
	Err error
}

type BatchReadRef struct {
	data []Resource
}

func (ref *BatchReadRef) Get() []Resource {
	return ref.data
}

func NewBatchReadRef(data []Resource) *BatchReadRef {
	return &BatchReadRef{data: data}
}

type ObjectAttr struct {
	Bucket             string
	Name               string
	ContentType        string
	ContentDisposition string
	CRC32C             uint32
	CRC64C             string
	Size               int64
	Etag               string
	Metadata           map[string]string
}

func GcpToAttr(attrs *storage.ObjectAttrs) *ObjectAttr {
	return &ObjectAttr{
		Bucket:             attrs.Bucket,
		Name:               attrs.Name,
		ContentType:        attrs.ContentType,
		ContentDisposition: attrs.ContentDisposition,
		CRC32C:             attrs.CRC32C,
		Size:               attrs.Size,
		Metadata:           attrs.Metadata,
		Etag:               attrs.Etag,
	}
}

func (atr ObjectAttr) SetGcpWriter(w *storage.Writer) *storage.Writer {
	w.ContentType = atr.ContentType
	w.ContentDisposition = atr.ContentDisposition
	return w
}

type BatchReadOption struct {
	// 此参数目前仅对jpg类型的图片生效
	TryRemoveExif bool
}

type ShardItem struct {
	Idx     int
	Size    int64
	ObjName string
}

type SignRs struct {
	Items []SignItem `json:"items"`
}

func (s SignRs) Objs() []string {
	objs := make([]string, len(s.Items))
	for i, item := range s.Items {
		objs[i] = item.ObjName
	}
	return objs
}

func (s SignRs) Addrs() []string {
	signAddrs := make([]string, len(s.Items))
	for i, item := range s.Items {
		signAddrs[i] = item.Url
	}
	return signAddrs
}

type SignItem struct {
	Url     string `json:"url"`
	ObjName string `json:"obj_name"`
	Idx     int    `json:"idx"`
	Size    int64  `json:"size"`
}

// GenPartName 生成分片名称
func GenPartName(fileId, fileExt string, partNumber ...string) string {
	if len(partNumber) == 0 {
		return fileId + fileExt
	}
	// 按目录规划分片
	return fileId + "_/" + partNumber[0] + fileExt
}

// IsNotFound 判断是否为不存在错误
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	if strings.Contains(err.Error(), "object doesn't exist") {
		return true
	}
	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		if gerr.Code == http.StatusNotFound {
			return true
		}
	}
	return false
}

// MediaCdnSignURL media cdn签名
func MediaCdnSignURL(inputURL, keyName, base64Key string, expirationTime time.Time) (string, error) {
	urlToSign := fmt.Sprintf("%s?Expires=%d&KeyName=%s", inputURL, expirationTime.Unix(), keyName)
	keySeed, err := base64.URLEncoding.DecodeString(base64Key)
	if err != nil {
		return "", err
	}
	if len(keySeed) != 32 {
		return "", errors.New("base64Key must decode to 32 bytes")
	}
	privateKey := ed25519.NewKeyFromSeed(keySeed)
	signature := ed25519.Sign(privateKey, []byte(urlToSign))
	encodedSignature := base64.URLEncoding.EncodeToString(signature)
	signedURL := fmt.Sprintf("%s&Signature=%s", urlToSign, encodedSignature)
	return signedURL, nil
}

// MergeFileParts MergeFileParts
type MergeFileParts struct {
	// 文件 ID
	FileId string
	// 文件扩展名
	FileExt string
	// 文件大小
	FileSize int64
	// 文件分片记录
	Parts  map[string]*Part
	Prefix string
	Url    string
}

type Part struct {
	Number    string
	Url       string
	Confirmed bool
}

type MergeFileParam struct {
	SizeLimit int64
}
