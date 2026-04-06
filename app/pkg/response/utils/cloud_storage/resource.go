package cloud_storage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpproxy"
)

// Define custom errors
var (
	RegionNotFound = errors.New("region not found")
	UploadFailed   = errors.New("upload failed")
	ServerError    = errors.New("server error")
)

const SuccessStatusCode = 10000

// Define regions
type Region string

const (
	//可用区域
	AsiaEast2Region      Region = "asia-east2"      // 中国香港
	UsCentral1Region     Region = "us-central1"     // 美国爱荷华州
	AsiaNortheast3Region Region = "asia-northeast3" // 韩国

	//预留区域
	UsWest1Region        Region = "us-west1"        // 美国俄勒冈州
	UsWest2Region        Region = "us-west2"        // 美国加利福尼亚州
	AsiaNortheast1Region Region = "asia-northeast1" // 日本东京
	UsEast1Region        Region = "us-east1"        // 美国南卡罗来纳州
	UsEast4Region        Region = "us-east4"        // 美国北弗吉尼亚州
	UsWest3Region        Region = "us-west3"        // 美国俄勒冈州
	UsWest4Region        Region = "us-west4"        // 美国内华达州
	AsiaSoutheast1Region Region = "asia-southeast1" // 新加坡
)

// Region to URL mapping (生产环境)
var regionURLMap = map[Region]string{
	//可用区域
	UsCentral1Region:     "https://us-central1.resources.api.seaart.dev",     // 美国爱荷华州
	AsiaEast2Region:      "https://asia-east2.resources.api.seaart.dev",      // 中国香港
	AsiaNortheast3Region: "https://asia-northeast3.resources.api.seaart.dev", // 首尔

	//预留区域
	UsWest1Region:        "https://us-west1.resources.api.seaart.dev",        // 美国俄勒冈州
	UsWest2Region:        "https://us-west2.resources.api.seaart.dev",        // 美国加利福尼亚州
	AsiaNortheast1Region: "https://asia-northeast1.resources.api.seaart.dev", // 日本东京
	UsEast1Region:        "https://us-east1.resources.api.seaart.dev",        // 美国南卡罗来纳州
	UsEast4Region:        "https://us-east4.resources.api.seaart.dev",        // 美国北弗吉尼亚州
	UsWest3Region:        "https://us-west3.resources.api.seaart.dev",        // 美国俄勒冈州
	UsWest4Region:        "https://us-west4.resources.api.seaart.dev",        // 美国内华达州
	AsiaSoutheast1Region: "https://asia-southeast1.resources.api.seaart.dev", // 新加坡
}

type HTTPClient struct {
	client *fasthttp.Client
}

func NewHTTPClient(useProxy bool) *HTTPClient {
	cli := &HTTPClient{
		client: &fasthttp.Client{
			MaxIdleConnDuration: 30 * time.Second,
		},
	}
	if useProxy {
		cli.client.Dial = fasthttpproxy.FasthttpProxyHTTPDialer()
	}
	return cli
}

const (
	maxRetries = 3
	minBackoff = 100 * time.Millisecond
	maxBackoff = 300 * time.Millisecond
)

// DoRequest sends an HTTP request and returns the response body
func (hc *HTTPClient) DoRequest(method, url string, headers map[string]string, body []byte) ([]byte, int, error) {
	rand.Seed(time.Now().UnixNano())
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()

		defer fasthttp.ReleaseRequest(req)
		defer fasthttp.ReleaseResponse(resp)

		req.SetRequestURI(url)
		req.Header.SetMethod(method)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		if body != nil {
			req.SetBody(body)
		}

		lastErr = hc.client.Do(req, resp)
		if lastErr == nil {
			rspBody := resp.Body()
			statusCode := resp.StatusCode()
			return rspBody, statusCode, nil
		}
		if attempt < maxRetries {
			backoff := minBackoff + time.Duration(rand.Int63n(int64(maxBackoff-minBackoff)))
			time.Sleep(backoff)
			continue
		}
		return nil, 0, lastErr
	}
	if lastErr != nil {
		return nil, 0, lastErr
	}
	return nil, 0, net.ErrClosed
}

// DoJSONRequest sends an HTTP request with JSON body and decodes the JSON response
func (hc *HTTPClient) DoJSONRequest(ctx context.Context, secretKey string, url string, requestBody map[string]interface{}, responseBody interface{}) (int, error) {
	var body []byte
	var err error
	if requestBody != nil {
		body, err = jsoniter.Marshal(requestBody)
		if err != nil {
			return 0, fmt.Errorf("failed to marshal request body: %w", err)
		}
	}

	var timestamp = time.Now().UTC().Unix()

	headers := map[string]string{
		"Content-Type": "application/json",
		"X-Signature":  generateSignature(secretKey, timestamp),
		"X-Timestamp":  strconv.FormatInt(timestamp, 10),
	}

	respBody, statusCode, err := hc.DoRequest(fasthttp.MethodPost, url, headers, body)
	if err != nil {
		return statusCode, err
	}

	if responseBody != nil {
		if err := jsoniter.Unmarshal(respBody, responseBody); err != nil {
			return statusCode, fmt.Errorf("failed to unmarshal response body: %w", err)
		}
	}

	return statusCode, nil
}

// ResourceSdk encapsulates resource operations
type ResourceSdk struct {
	region       Region
	serviceName  string
	httpClient   *HTTPClient
	isDev        bool
	isLocal      bool
	resourceType ResourceType
	secretKey    string
	config       *Config
	lock         sync.Mutex
}

type ResourceType int

const (
	FileType  ResourceType = 1
	ModelType ResourceType = 2
)

type ResourceSdkParam struct {
	Region      Region
	ServiceName string
}

type ResourceSdkOption struct {
	UseProxy bool
}

// NewResourceSdk initializes and returns a ResourceSdk
func NewResourceSdk(rg Region, serviceName string, resourceType ResourceType, secretKey string, options ...ResourceSdkOption) *ResourceSdk {
	var useProxy bool
	if len(options) > 0 {
		useProxy = options[0].UseProxy
	}
	return &ResourceSdk{
		region:       rg,
		serviceName:  serviceName,
		httpClient:   NewHTTPClient(useProxy),
		resourceType: resourceType,
		secretKey:    secretKey,
	}
}

func (res *ResourceSdk) Dev() {
	res.isDev = true
}

func (res *ResourceSdk) Local() {
	res.isLocal = true
}

// getUrl retrieves the URL based on the region
func (res *ResourceSdk) getUrl() (string, error) {
	var urlStr string
	var exists bool
	if res.isDev {
		exists = true
		urlStr = "https://us-central1.resources.api.dev.seaart.dev"
	} else if res.isLocal {
		exists = true
		urlStr = "http://127.0.0.1:8080"
	} else {
		urlStr, exists = regionURLMap[res.region]
	}
	if !exists {
		return "", RegionNotFound
	}
	return urlStr, nil
}

// Config represents the configuration structure
type Config struct {
	CdnHost      string `json:"cdn_host"`
	ProxyCdnHost string `json:"proxy_cdn_host"`
}

// Status represents the status structure
type Status struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// configRsp represents the response structure for config
type configRsp struct {
	Data   *Config `json:"data"`
	Status Status  `json:"status"`
}

func (res *ResourceSdk) getConfig(ctx context.Context) (*Config, error) {
	if res.config != nil {
		return res.config, nil
	}

	res.lock.Lock()
	defer res.lock.Unlock()

	if res.config != nil {
		return res.config, nil
	}

	urlStr, err := res.getUrl()
	if err != nil {
		return nil, err
	}

	requestPayload := map[string]interface{}{
		"server_region": res.region,
		"server_name":   res.serviceName,
		"type":          res.resourceType,
	}

	var rsp configRsp
	fullURL := fmt.Sprintf("%s/v1/config", urlStr)
	statusCode, err := res.httpClient.DoJSONRequest(ctx, res.secretKey, fullURL, requestPayload, &rsp)
	if err != nil {
		return nil, err
	}

	if statusCode != fasthttp.StatusOK || rsp.Status.Code != SuccessStatusCode {
		return nil, ServerError
	}

	res.config = rsp.Data
	return res.config, nil
}

// UploadObject uploads an object with optional attributes
func (res *ResourceSdk) UploadObject(ctx context.Context, objectName string, file []byte, attrs ...ObjectAttr) error {
	contentType := "application/octet-stream"
	if len(attrs) > 0 {
		contentType = attrs[0].ContentType
	}

	pre, err := res.GetPreSign(ctx, PreParam{
		Parts: []PrePart{
			{
				FileName: objectName,
				FileSize: len(file),
				FileOption: PreSignOption{
					ContentType: contentType,
				},
			},
		},
	})
	if err != nil {
		return err
	}

	if len(pre.Data) == 0 {
		return UploadFailed
	}

	return res.uploadToSignedURL(ctx, pre.Data[0].Sign, file, attrs...)
}

func (res *ResourceSdk) UploadPreSign(ctx context.Context, objectName string, file []byte, attrs ...ObjectAttr) (string, error) {
	contentType := "application/octet-stream"
	if len(attrs) > 0 {
		contentType = attrs[0].ContentType
	}

	pre, err := res.GetPreSign(ctx, PreParam{
		Parts: []PrePart{
			{
				FileName: objectName,
				FileSize: len(file),
				FileOption: PreSignOption{
					ContentType: contentType,
				},
			},
		},
	})
	if err != nil {
		return "", err
	}

	if len(pre.Data) == 0 {
		return "", UploadFailed
	}

	return pre.Data[0].Sign, nil
}

type UploadObject struct {
	ObjectName string
	File       []byte
	Attr       *ObjectAttr
}

// BatchUploadResult represents the result of a single object upload.
type BatchUploadResult struct {
	ObjectName string
	Error      error
}

func (res *ResourceSdk) UploadObjects(ctx context.Context, objects []UploadObject) []BatchUploadResult {
	results := make([]BatchUploadResult, len(objects))
	if len(objects) == 0 {
		return results
	}

	preParts := make([]PrePart, 0, len(objects))
	for _, obj := range objects {
		contentType := "application/octet-stream"
		if obj.Attr != nil && obj.Attr.ContentType != "" {
			contentType = obj.Attr.ContentType
		}
		preParts = append(preParts, PrePart{
			FileName: obj.ObjectName,
			FileSize: len(obj.File),
			FileOption: PreSignOption{
				ContentType: contentType,
			},
		})
	}

	pre, err := res.GetPreSign(ctx, PreParam{
		Parts: preParts,
	})
	if err != nil {
		for i, obj := range objects {
			results[i] = BatchUploadResult{
				ObjectName: obj.ObjectName,
				Error:      err,
			}
		}
		return results
	}

	if len(pre.Data) != len(objects) {
		for i, obj := range objects {
			results[i] = BatchUploadResult{
				ObjectName: obj.ObjectName,
				Error:      ServerError,
			}
		}
		return results
	}

	var wg sync.WaitGroup
	wg.Add(len(objects))

	var mu sync.Mutex

	for i, obj := range objects {
		go func(i int, obj UploadObject, sign string) {
			defer wg.Done()
			contentType := "application/octet-stream"
			if obj.Attr != nil && obj.Attr.ContentType != "" {
				contentType = obj.Attr.ContentType
			}
			contentDisposition := ""
			if obj.Attr != nil && obj.Attr.ContentDisposition != "" {
				contentDisposition = obj.Attr.ContentDisposition
			}
			err := res.uploadToSignedURL(ctx, sign, obj.File, ObjectAttr{
				ContentType:        contentType,
				ContentDisposition: contentDisposition,
			})
			mu.Lock()
			defer mu.Unlock()
			results[i] = BatchUploadResult{
				ObjectName: obj.ObjectName,
				Error:      err,
			}
		}(i, obj, pre.Data[i].Sign)
	}
	wg.Wait()
	return results
}

type BatchSignResult struct {
	ObjectName string
	Sign       string
	Error      error
}

func (res *ResourceSdk) UploadPreSigns(ctx context.Context, objects []UploadObject) []BatchSignResult {
	results := make([]BatchSignResult, len(objects))
	if len(objects) == 0 {
		return results
	}

	preParts := make([]PrePart, 0, len(objects))
	for _, obj := range objects {
		contentType := "application/octet-stream"
		if obj.Attr != nil && obj.Attr.ContentType != "" {
			contentType = obj.Attr.ContentType
		}
		preParts = append(preParts, PrePart{
			FileName: obj.ObjectName,
			FileSize: len(obj.File),
			FileOption: PreSignOption{
				ContentType: contentType,
			},
		})
	}

	pre, err := res.GetPreSign(ctx, PreParam{
		Parts: preParts,
	})
	if err != nil {
		for i, obj := range objects {
			results[i] = BatchSignResult{
				ObjectName: obj.ObjectName,
				Error:      err,
			}
		}
		return results
	}

	if len(pre.Data) != len(objects) {
		for i, obj := range objects {
			results[i] = BatchSignResult{
				ObjectName: obj.ObjectName,
				Error:      ServerError,
			}
		}
		return results
	}

	for i, obj := range objects {
		results[i] = BatchSignResult{
			ObjectName: obj.ObjectName,
			Sign:       pre.Data[i].Sign,
			Error:      nil,
		}
	}
	return results
}

// PreSign represents a pre-signed URL
type PreSign struct {
	FileName string `json:"file_name"`
	Sign     string `json:"sign"`
}

// PreRsp represents the response structure for pre-sign
type PreRsp struct {
	Data   []PreSign `json:"data"`
	Status Status    `json:"status"`
}

type baseRsp struct {
	Data   interface{} `json:"data"`
	Status Status      `json:"status"`
}

// PreSignOption represents options for pre-signing
type PreSignOption struct {
	ContentType string `json:"content_type"`
	Method      string `json:"method"`
	Expires     int64  `json:"expires"`
	Category    int    `json:"category"`
}

// PrePart represents a part for pre-signing
type PrePart struct {
	FileName   string        `json:"file_name"`
	FileSize   int           `json:"file_size"`
	FileOption PreSignOption `json:"file_option,omitempty"`
}

type PreParam struct {
	Parts []PrePart
}

// GetPreSign retrieves pre-signed URLs
func (res *ResourceSdk) GetPreSign(ctx context.Context, param PreParam) (PreRsp, error) {
	urlStr, err := res.getUrl()
	if err != nil {
		return PreRsp{}, err
	}

	requestPayload := map[string]interface{}{
		"server_name":   res.serviceName,
		"server_region": res.region,
		"type":          res.resourceType,
		"parts":         param.Parts,
	}

	var rsp PreRsp
	fullURL := fmt.Sprintf("%s/v1/preSign", urlStr)
	statusCode, err := res.httpClient.DoJSONRequest(ctx, res.secretKey, fullURL, requestPayload, &rsp)
	if err != nil {
		return PreRsp{}, err
	}

	if statusCode != fasthttp.StatusOK || rsp.Status.Code != SuccessStatusCode {
		return PreRsp{}, ServerError
	}

	if len(rsp.Data) == 0 {
		return PreRsp{}, ServerError
	}

	return rsp, nil
}

// uploadToSignedURL uploads data to a pre-signed URL
func (res *ResourceSdk) uploadToSignedURL(ctx context.Context, url string, data []byte, attrs ...ObjectAttr) error {
	var contentType string
	var contentDisposition string
	if len(attrs) != 0 {
		contentType = attrs[0].ContentType
		contentDisposition = attrs[0].ContentDisposition
	}

	headers := map[string]string{
		"Content-Type": contentType,
	}
	if contentDisposition != "" {
		headers["Content-Disposition"] = contentDisposition
	}

	_, statusCode, err := res.httpClient.DoRequest(fasthttp.MethodPut, url, headers, data)
	if err != nil {
		return err
	}

	if statusCode != fasthttp.StatusOK && statusCode != fasthttp.StatusCreated && statusCode != fasthttp.StatusNoContent {
		return UploadFailed
	}

	return nil
}

// ReadObject 下载文件
func (res *ResourceSdk) ReadObject(ctx context.Context, objectName string) ([]byte, error) {
	config, err := res.getConfig(ctx)
	if err != nil {
		return nil, err
	}
	objectName = getPathWithoutSlash(objectName)

	//优先使用代理CDN
	if config.ProxyCdnHost != "" {
		byt, _, _ := res.httpClient.DoRequest(fasthttp.MethodGet, config.ProxyCdnHost+"/"+objectName, nil, nil)
		if len(byt) > 0 {
			return byt, nil
		}
	}

	//使用CDN
	byt, _, err := res.httpClient.DoRequest(fasthttp.MethodGet, config.CdnHost+"/"+objectName, nil, nil)
	return byt, err
}

// GetObjectAttrs 获取对象属性
func (res *ResourceSdk) GetObjectAttrs(ctx context.Context, objectName string) (*ObjectAttr, error) {
	rsp, err := res.getAttrs(ctx, objectName)
	if err != nil {
		return nil, err
	}
	return &ObjectAttr{
		Bucket:      rsp.Data.Bucket,
		Name:        rsp.Data.Name,
		ContentType: rsp.Data.ContentType,
		CRC32C:      rsp.Data.CRC32C,
		Size:        rsp.Data.Size,
		Etag:        rsp.Data.Etag,
		Metadata:    rsp.Data.Metadata,
	}, nil
}

type Attrs struct {
	Bucket          string            `json:"bucket"`
	Name            string            `json:"name"`
	ContentType     string            `json:"content_type"`
	ContentLanguage string            `json:"content_language"`
	CacheControl    string            `json:"cache_control"`
	Size            int64             `json:"size"`
	CRC32C          uint32            `json:"crc_32_c"`
	Metadata        map[string]string `json:"metadata"`
	Prefix          string            `json:"prefix"`
	Etag            string            `json:"etag"`
}

type attrsRsp struct {
	Data   Attrs  `json:"data"`
	Status Status `json:"status"`
}

func (res *ResourceSdk) getAttrs(ctx context.Context, path string) (*attrsRsp, error) {
	urlStr, err := res.getUrl()
	if err != nil {
		return nil, err
	}

	requestPayload := map[string]interface{}{
		"server_name":   res.serviceName,
		"server_region": res.region,
		"type":          res.resourceType,
		"path":          path,
	}

	var rsp *attrsRsp
	fullURL := fmt.Sprintf("%s/v1/attrs", urlStr)
	statusCode, err := res.httpClient.DoJSONRequest(ctx, res.secretKey, fullURL, requestPayload, &rsp)
	if err != nil {
		return nil, err
	}

	if statusCode != fasthttp.StatusOK || rsp.Status.Code != SuccessStatusCode {
		return nil, ServerError
	}

	return rsp, nil
}

func (res *ResourceSdk) DeleteObject(ctx context.Context, objectName string) error {
	return res.delete(ctx, objectName)
}

func (res *ResourceSdk) delete(ctx context.Context, path string) error {
	urlStr, err := res.getUrl()
	if err != nil {
		return err
	}

	requestPayload := map[string]interface{}{
		"server_name":   res.serviceName,
		"server_region": res.region,
		"type":          res.resourceType,
		"path":          path,
	}

	var rsp *attrsRsp
	fullURL := fmt.Sprintf("%s/v1/delete", urlStr)
	statusCode, err := res.httpClient.DoJSONRequest(ctx, res.secretKey, fullURL, requestPayload, &rsp)
	if err != nil {
		return err
	}

	if statusCode != fasthttp.StatusOK || rsp.Status.Code != SuccessStatusCode {
		return ServerError
	}

	return nil
}

func getPathWithoutSlash(urlStr string) string {
	if urlStr == "" {
		return ""
	}
	u, err := url.Parse(urlStr)
	if err != nil {
		return ""
	}
	urlStr = u.Path
	if strings.HasPrefix(urlStr, "/") {
		return urlStr[1:]
	}
	return urlStr
}

func generateSignature(key string, timestamp int64) string {
	h := hmac.New(sha256.New, []byte(key))
	h.Write([]byte(strconv.FormatInt(timestamp, 10)))
	return hex.EncodeToString(h.Sum(nil))
}

func (res *ResourceSdk) setAttrs(ctx context.Context, objectName string, attrs map[string]string) error {
	urlStr, err := res.getUrl()
	if err != nil {
		return err
	}

	requestPayload := map[string]interface{}{
		"server_name":   res.serviceName,
		"server_region": res.region,
		"type":          res.resourceType,
		"path":          objectName,
		"attrs":         attrs,
	}

	var rsp *attrsRsp
	fullURL := fmt.Sprintf("%s/v1/set-attrs", urlStr)
	statusCode, err := res.httpClient.DoJSONRequest(ctx, res.secretKey, fullURL, requestPayload, &rsp)
	if err != nil {
		return err
	}

	if statusCode != fasthttp.StatusOK || rsp.Status.Code != SuccessStatusCode {
		return ServerError
	}

	return nil
}

func (res *ResourceSdk) SetAttrs(ctx context.Context, objectName string, attrs map[string]string) error {
	return res.setAttrs(ctx, objectName, attrs)
}

type createTraceParam struct {
	Path     string
	CreateAt int64
	UpdateAt int64
	ByteSize int64
}

func (res *ResourceSdk) createTrace(ctx context.Context, param createTraceParam) (baseRsp, error) {
	urlStr, err := res.getUrl()
	if err != nil {
		return baseRsp{}, err
	}

	requestPayload := map[string]interface{}{
		"server_name":   res.serviceName,
		"server_region": res.region,
		"type":          res.resourceType,
		"action":        "CREATE",
		"path":          param.Path,
		"create_at":     param.CreateAt,
		"update_at":     param.UpdateAt,
		"byte_size":     param.ByteSize,
	}

	var rsp baseRsp
	fullURL := fmt.Sprintf("%s/v1/trace", urlStr)
	statusCode, err := res.httpClient.DoJSONRequest(ctx, res.secretKey, fullURL, requestPayload, &rsp)
	if err != nil {
		return baseRsp{}, err
	}

	if statusCode != fasthttp.StatusOK || rsp.Status.Code != SuccessStatusCode {
		return baseRsp{}, ServerError
	}

	return rsp, nil
}

func (res *ResourceSdk) MoveObject(ctx context.Context, oldBuket string, oldPath string, newBuket string, newPath string) error {
	_, err := res.moveObject(ctx, moveObjectParam{
		OldBuket: oldBuket,
		OldPath:  oldPath,
		NewBuket: newBuket,
		NewPath:  newPath,
	})
	return err
}

type moveObjectParam struct {
	OldBuket string
	OldPath  string
	NewBuket string
	NewPath  string
}

func (res *ResourceSdk) moveObject(ctx context.Context, param moveObjectParam) (baseRsp, error) {
	urlStr, err := res.getUrl()
	if err != nil {
		return baseRsp{}, err
	}

	requestPayload := map[string]interface{}{
		"server_name":   res.serviceName,
		"server_region": res.region,
		"type":          res.resourceType,
		"old_buket":     param.OldBuket,
		"old_path":      param.OldPath,
		"new_buket":     param.NewBuket,
		"new_path":      param.NewPath,
	}

	var rsp baseRsp
	fullURL := fmt.Sprintf("%s/v1/move", urlStr)
	statusCode, err := res.httpClient.DoJSONRequest(ctx, res.secretKey, fullURL, requestPayload, &rsp)
	if err != nil {
		return baseRsp{}, err
	}

	if statusCode != fasthttp.StatusOK || rsp.Status.Code != SuccessStatusCode {
		return baseRsp{}, ServerError
	}

	return rsp, nil
}

func (res *ResourceSdk) GetReader(ctx context.Context, objectName string) (io.ReadCloser, error) {
	byt, err := res.ReadObject(ctx, objectName)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(byt)), nil
}
