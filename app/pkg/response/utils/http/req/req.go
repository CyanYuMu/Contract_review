package req

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/imroc/req/v3"
	jsoniter "github.com/json-iterator/go"
	"github.com/opentracing/opentracing-go"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/enum"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/trace"
	"golang.org/x/net/http2"
)

// SSEHandler SSE消息处理函数
type SSEHandler func(line string) error

// SSEResponse SSE响应结构
type SSEResponse struct {
	*http.Response
	scanner         *bufio.Scanner
	hasReceivedData bool // 标记是否已经接收到数据
}

// ReadLine 读取SSE消息行
func (s *SSEResponse) ReadLine() (string, error) {
	if s.scanner.Scan() {
		s.hasReceivedData = true // 标记已接收到数据
		return s.scanner.Text(), nil
	}
	return "", s.scanner.Err()
}

// HasReceivedData 检查是否已经接收到数据
func (s *SSEResponse) HasReceivedData() bool {
	return s.hasReceivedData
}

// ReadMessage 读取完整的SSE消息（包含data、event、id等）
func (s *SSEResponse) ReadMessage() (map[string]string, error) {
	message := make(map[string]string)

	for {
		line, err := s.ReadLine()
		if err != nil {
			if err == io.EOF && len(message) > 0 {
				return message, nil
			}
			return nil, err
		}

		// 空行表示消息结束
		if line == "" {
			if len(message) > 0 {
				return message, nil
			}
			continue
		}

		// 解析SSE格式
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			message[key] = value
		}
	}
}

// ForEach 遍历所有SSE消息
func (s *SSEResponse) ForEach(handler SSEHandler) error {
	defer s.Body.Close()

	for {
		line, err := s.ReadLine()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		if err := handler(line); err != nil {
			return err
		}
	}
}

type Client struct {
	ctx context.Context
	c   *req.Client
	r   *req.Request
	// 原生HTTP客户端，用于SSE请求
	httpClient *http.Client
	// SSE重试配置
	sseRetryConfig *SSERetryConfig
}

// New
// @description 获取client示例
// @description **默认会使用系统的proxy
// @description **默认启用追踪（如果 ytrace 已初始化）
func New(ctx context.Context) *Client {
	c := req.NewClient().SetJsonMarshal(jsoniter.Marshal).SetJsonUnmarshal(jsoniter.Unmarshal)
	r := c.NewRequest()
	// r.SetContext(ctx)

	// 初始化原生HTTP客户端
	httpClient := &http.Client{
		Timeout: 30 * time.Second, // 默认30秒超时
	}

	cli := &Client{
		ctx:        ctx,
		r:          r,
		c:          c,
		httpClient: httpClient,
	}

	cli.Context(ctx)

	// 默认启用追踪
	return cli.WithTracingRoundTripper()
}

func (c *Client) NewRequest() *req.Request {
	return c.c.NewRequest()
}

/*NoProxy
* @Description: 禁用proxy
* @return *Client
 */
func (c *Client) NoProxy() *Client {
	c.c.Transport.Options.Proxy = nil

	return c
}

func (c *Client) Proxy(proxy string) *Client {
	c.c.SetProxyURL(proxy)

	return c
}

// Timeout
// @description 设置超时时间
func (c *Client) Timeout(d time.Duration) *Client {
	c.c.SetTimeout(d)
	c.httpClient.Timeout = d
	return c
}

// TimeoutConfig SSE超时配置
type TimeoutConfig struct {
	// 总超时时间
	Timeout time.Duration
	// 连接超时时间
	DialTimeout time.Duration
	// 响应头读取超时时间
	ReadTimeout time.Duration
}

// SSERetryConfig SSE重试配置
type SSERetryConfig struct {
	// 重试次数
	MaxRetries int
	// 重试间隔
	RetryInterval time.Duration
	// 指数退避倍数 (可选，默认为1表示固定间隔)
	BackoffMultiplier float64
	// 最大重试间隔 (防止指数退避间隔过长)
	MaxRetryInterval time.Duration
	// 重试条件判断函数 (可选)
	ShouldRetry func(err error, statusCode int) bool
}

// SSETimeout 设置SSE请求的详细超时配置
func (c *Client) SSETimeout(config *TimeoutConfig) *Client {
	transport := &http.Transport{}

	if config.DialTimeout > 0 {
		transport.DialContext = (&net.Dialer{
			Timeout: config.DialTimeout,
		}).DialContext
	}

	if config.ReadTimeout > 0 {
		transport.ResponseHeaderTimeout = config.ReadTimeout
	}

	// WriteTimeout 在 http.Transport 中没有直接对应的字段
	// 对于 SSE 长连接，通常不需要设置写超时

	c.httpClient.Transport = transport

	if config.Timeout > 0 {
		c.httpClient.Timeout = config.Timeout
	}

	return c
}

// SSERetry 设置SSE重试配置
func (c *Client) SSERetry(config *SSERetryConfig) *Client {
	c.sseRetryConfig = config
	return c
}

// TryTimes
// @description 请求失败后的重试次数
func (c *Client) TryTimes(n int) *Client {
	c.r.SetRetryCount(n)

	return c
}

// Retry Config
type RetryConfig struct {
	// 重试次数, 可选
	Count int
	// 重试间隔, 可选
	Interval time.Duration
	// 重试判断的条件, 比如基于响应内容code判断是否服务繁忙, 可选
	Condition func(resp *req.Response, err error) bool
}

// 自定义重试方案
func (c *Client) Retry(cnf *RetryConfig) *Client {
	if cnf == nil {
		return c
	}
	if cnf.Count > 0 {
		c.r.SetRetryCount(cnf.Count)
	}
	if cnf.Interval > 0 {
		c.r.SetRetryFixedInterval(cnf.Interval)
	}

	if cnf.Condition != nil {
		c.r.SetRetryCondition(cnf.Condition)
	}

	return c
}

type TLSConfig struct {
	CertFile string
	KeyFile  string
}

// TLS
// @description 设置证书
// 参考示例:
func (c *Client) TLS(cnf *TLSConfig) *Client {
	cert, err := tls.LoadX509KeyPair(cnf.CertFile, cnf.KeyFile)
	if err != nil {
		fmt.Println(err, "密钥对不合法")
		return c
	}
	ssl := &tls.Config{
		Certificates: []tls.Certificate{cert},
		//InsecureSkipVerify: true,
	}
	// 有了证书, 选择使用http2进行通信
	c.c.GetClient().Transport = &http2.Transport{
		TLSClientConfig: ssl,
	}

	return c
}

// Insecure
// @description 涉及到证书校验时, 忽略校验
func (c *Client) Insecure() *Client {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	c.c.GetClient().Transport = tr

	return c
}

func (c *Client) SetJsonMarshal(marshal func(v interface{}) ([]byte, error)) *Client {
	c.c.SetJsonMarshal(marshal)

	return c
}

func (c *Client) SetJsonUnmarshal(unmarshal func(data []byte, v interface{}) error) *Client {
	c.c.SetJsonUnmarshal(unmarshal)

	return c
}

// Debug
// @description 开启调试模式, 收到响应后, 会将响应的原始数据全量打印出来
func (c *Client) Debug() *Client {
	c.c.EnableDumpAll()

	return c
}

// Context
// @description 中途修改context
func (c *Client) Context(ctx context.Context) *Client {
	c.ctx = ctx
	c.r.SetContext(ctx)
	reqId, spanId, traceParam, debug := trace.ParseCore(ctx)
	if reqId != "" {
		c.r.SetHeader(enum.HeaderRequestId, reqId)
	}
	if spanId != "" {
		c.r.SetHeader(enum.HeaderSpanId, spanId)
	}
	// track param
	if traceParam != "" {
		c.r.SetHeader(enum.HeaderTrackingParams, traceParam)
	}
	if debug != "" {
		c.r.SetHeader(enum.HeaderDebug, debug)
	}

	return c
}

func AssignRequestCtx(r *req.Request, ctx context.Context) {
	reqId, spanId, traceParam, debug := trace.ParseCore(ctx)
	if reqId != "" {
		r.SetHeader(enum.HeaderRequestId, reqId)
	}
	if spanId != "" {
		r.SetHeader(enum.HeaderSpanId, spanId)
	}
	// track param
	if traceParam != "" {
		r.SetHeader(enum.HeaderTrackingParams, traceParam)
	}
	if debug != "" {
		r.SetHeader(enum.HeaderDebug, debug)
	}
}

// RequestId
// @description 使用脚本触发, 自定义requestId
func (c *Client) RequestId(id string) *Client {
	c.r.SetHeader(enum.HeaderRequestId, id)

	return c
}

// Header
// @description 设置header
func (c *Client) Header(header map[string]string) *Client {
	if header == nil {
		return c
	}
	for k, v := range header {
		c.r.SetHeader(k, v)
	}

	return c
}

// SetBasicToken
// @description 设置basic认证的token, 会在token前追加 Basic
func (c *Client) SetBasicToken(token, psd string) *Client {
	c.r.SetBasicAuth(token, psd)

	return c
}

// SetBearerToken
// @description 设置bearer认证的token, 会在token前追加 Bearer
func (c *Client) SetBearerToken(token string) *Client {
	c.r.SetBearerAuthToken(token)

	return c
}

func (c *Client) SetCertFromFile(clientPemFile, keyPemFile string) *Client {
	c.c.SetCertFromFile(clientPemFile, keyPemFile)

	return c
}

func (c *Client) SetCertificate(cert tls.Certificate) *Client {
	c.c.SetCerts(cert)

	return c
}

// SetRootCertsFromFile
// @description 设置根证书
func (c *Client) SetRootCertsFromFile(pemFiles ...string) *Client {
	c.c.SetRootCertsFromFile(pemFiles...)

	return c
}

// SetRootCertFromString
// @description 设置根证书
func (c *Client) SetRootCertFromString(certs ...string) *Client {
	c.c.SetRootCertsFromFile(certs...)

	return c
}

// SetToken
// @description 自定义token
func (c *Client) SetToken(token string) *Client {
	c.r.Headers.Set("Authorization", token)

	return c
}

func (c *Client) ForceHttp1() *Client {
	c.c.EnableForceHTTP1()
	return c
}

func (c *Client) ForceHttp2() *Client {
	c.c.EnableForceHTTP2()

	return c
}

// createHTTPRequest 创建原生HTTP请求
func (c *Client) createHTTPRequest(method, url string) (*http.Request, error) {
	var body io.Reader

	// 获取req.Request中的body
	// req/v3的Body字段是[]byte类型
	if len(c.r.Body) > 0 {
		body = strings.NewReader(string(c.r.Body))
	}

	req, err := http.NewRequestWithContext(c.ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	// 复制请求头 - 从req.Request的Headers中获取
	for k, values := range c.r.Headers {
		for _, v := range values {
			req.Header.Add(k, v)
		}
	}

	// 添加链路追踪头
	reqId, spanId := trace.ParseFromContext(c.ctx)
	if reqId != "" {
		req.Header.Set(enum.HeaderRequestId, reqId)
	}
	if spanId != "" {
		req.Header.Set(enum.HeaderSpanId, spanId)
	}

	return req, nil
}

// DoSSERequest 执行SSE请求，支持重试机制
func (c *Client) DoSSERequest(method, url string) (*SSEResponse, error) {
	var lastErr error
	var resp *http.Response

	// 默认重试配置
	retryConfig := c.sseRetryConfig
	if retryConfig == nil {
		retryConfig = &SSERetryConfig{
			MaxRetries:        3,
			RetryInterval:     time.Second,
			BackoffMultiplier: 1.0,
		}
	}

	// 默认重试条件：网络错误或5xx状态码
	shouldRetry := retryConfig.ShouldRetry
	if shouldRetry == nil {
		shouldRetry = func(err error, statusCode int) bool {
			// 网络错误重试
			if err != nil {
				return true
			}
			// 5xx服务器错误重试
			return statusCode >= 500 && statusCode < 600
		}
	}

	maxAttempts := retryConfig.MaxRetries + 1 // 包含初始请求

	for attempt := 0; attempt < maxAttempts; attempt++ {
		req, err := c.createHTTPRequest(method, url)
		if err != nil {
			return nil, err
		}

		resp, err = c.httpClient.Do(req)
		lastErr = err

		// 请求成功，返回带有数据接收状态跟踪的SSEResponse
		if err == nil && resp.StatusCode < 500 {
			return &SSEResponse{
				Response:        resp,
				scanner:         bufio.NewScanner(resp.Body),
				hasReceivedData: false, // 初始化为未接收数据状态
			}, nil
		}

		// 判断是否需要重试
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
			resp.Body.Close() // 关闭失败的响应体
		}

		if attempt < maxAttempts-1 && shouldRetry(err, statusCode) {
			// 计算重试间隔
			interval := retryConfig.RetryInterval
			if retryConfig.BackoffMultiplier > 1.0 {
				multiplier := 1.0
				for i := 0; i < attempt; i++ {
					multiplier *= retryConfig.BackoffMultiplier
				}
				interval = time.Duration(float64(interval) * multiplier)

				// 限制最大重试间隔
				if retryConfig.MaxRetryInterval > 0 && interval > retryConfig.MaxRetryInterval {
					interval = retryConfig.MaxRetryInterval
				}
			}

			// 等待重试
			select {
			case <-c.ctx.Done():
				return nil, c.ctx.Err()
			case <-time.After(interval):
				continue
			}
		}

		break
	}

	// 所有重试都失败了
	if lastErr != nil {
		return nil, lastErr
	}
	if resp != nil && resp.StatusCode >= 400 {
		return nil, fmt.Errorf("SSE request failed with status: %d", resp.StatusCode)
	}

	return nil, fmt.Errorf("SSE request failed after %d attempts", maxAttempts)
}

// afterRequest
// @description 回收
func (c *Client) afterRequest(resp *req.Response) {
}

// ContentType
// @description 设置编码方式
func (c *Client) ContentType(t string) *Client {
	c.r.SetContentType(t)

	return c
}

// Cookie
// @description 设置cookie
func (c *Client) Cookie(data []*http.Cookie) *Client {
	c.r.SetCookies(data...)
	return c
}

// FormDataByValue
// @description 设置表单数据
func (c *Client) FormDataByValue(val url.Values) *Client {
	c.r.SetContentType("application/x-www-from-urlencoded")
	c.r.SetFormDataFromValues(val)

	return c
}

// FormData
// @description 以表单形式请求
func (c *Client) FormData(data map[string]string) *Client {
	c.r.SetContentType("application/x-www-from-urlencoded")
	c.r.SetFormData(data)

	return c
}

// File
// @description 表单形式上传文件
func (c *Client) File(name, path string) *Client {
	c.r.SetContentType("multipart/form-data")
	c.r.SetFile(name, path)

	return c
}

// BodyJson
// @description json编码格式, data 支持 string, []byte, io.Reader, map, struct, slice 类型
func (c *Client) BodyJson(data interface{}) *Client {
	c.r.SetContentType("application/json")
	c.r.SetBody(data)

	return c
}

// Body
// @description 设置body内容, data 支持 string, []byte, io.Reader, map, struct, slice 类型
func (c *Client) Body(data interface{}) *Client {
	c.r.SetBody(data)
	return c
}

func (c *Client) Post(url string) (resp *req.Response, err error) {
	span := c.Span(url)
	defer func() {
		if span != nil {
			span.Finish()
		}
	}()
	resp, err = c.r.Post(url)
	if err != nil {
		return resp, err
	}
	c.afterRequest(resp)
	return
}

// PostSSE SSE流式POST请求
func (c *Client) PostSSE(url string) (*SSEResponse, error) {
	return c.DoSSERequest("POST", url)
}

func (c *Client) Span(uri string) opentracing.Span {
	//if config2.AppEnvConfig.Tracing.Jaeger.JaegerOn {
	//
	//	parse, err := url.Parse(uri)
	//	if err == nil {
	//		traceId, pSpanId := trace.ParseFromContext(c.ctx)
	//		if traceId == "" {
	//			traceId = trace.NewRequestId()
	//		}
	//		spanId := trace.NewSpanId()
	//		return trace.NewSpan("remote", cast.ToUint64(traceId), cast.ToUint64(spanId), cast.ToUint64(pSpanId), map[string]interface{}{"host": parse.Host}, nil)
	//	}
	//}

	return nil
}

func (c *Client) Get(url string) (resp *req.Response, err error) {
	span := c.Span(url)
	defer func() {
		if span != nil {
			span.Finish()
		}
	}()
	resp, err = c.r.Get(url)
	if err != nil {
		return resp, err
	}
	c.afterRequest(resp)

	return
}

// GetSSE SSE流式GET请求
func (c *Client) GetSSE(url string) (*SSEResponse, error) {
	return c.DoSSERequest("GET", url)
}

func (c *Client) Delete(url string) (resp *req.Response, err error) {
	span := c.Span(url)
	defer func() {
		if span != nil {
			span.Finish()
		}
	}()
	resp, err = c.r.Delete(url)
	if err != nil {
		return resp, err
	}
	c.afterRequest(resp)

	return
}

func (c *Client) Put(url string) (resp *req.Response, err error) {
	span := c.Span(url)
	defer func() {
		if span != nil {
			span.Finish()
		}
	}()
	resp, err = c.r.Put(url)
	if err != nil {
		return resp, err
	}
	c.afterRequest(resp)

	return
}

func (c *Client) Patch(url string) (resp *req.Response, err error) {
	span := c.Span(url)
	defer func() {
		if span != nil {
			span.Finish()
		}
	}()
	resp, err = c.r.Patch(url)
	if err != nil {
		return resp, err
	}
	c.afterRequest(resp)

	return
}

// SSEStream 便捷的SSE流式处理方法
// 自动设置Accept头并执行GET请求，然后调用处理函数处理每行数据
func (c *Client) SSEStream(url string, handler SSEHandler) error {
	// 自动设置SSE请求头
	c.Header(map[string]string{
		"Accept":        "text/event-stream",
		"Cache-Control": "no-cache",
	})

	sseResp, err := c.GetSSE(url)
	if err != nil {
		return err
	}

	return sseResp.ForEach(handler)
}

// SSEStreamWithRetry 带重试的SSE流式处理方法
// 在连接断开时会自动重试，但只在未接收到数据时才重试
func (c *Client) SSEStreamWithRetry(url string, handler SSEHandler) error {
	// 自动设置SSE请求头
	c.Header(map[string]string{
		"Accept":        "text/event-stream",
		"Cache-Control": "no-cache",
	})

	retryConfig := c.sseRetryConfig
	if retryConfig == nil {
		retryConfig = &SSERetryConfig{
			MaxRetries:        3,
			RetryInterval:     time.Second,
			BackoffMultiplier: 1.0,
		}
	}

	maxAttempts := retryConfig.MaxRetries + 1
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		sseResp, err := c.GetSSE(url)
		if err != nil {
			lastErr = err

			// 计算重试间隔
			if attempt < maxAttempts-1 {
				interval := retryConfig.RetryInterval
				if retryConfig.BackoffMultiplier > 1.0 {
					multiplier := 1.0
					for i := 0; i < attempt; i++ {
						multiplier *= retryConfig.BackoffMultiplier
					}
					interval = time.Duration(float64(interval) * multiplier)

					if retryConfig.MaxRetryInterval > 0 && interval > retryConfig.MaxRetryInterval {
						interval = retryConfig.MaxRetryInterval
					}
				}

				select {
				case <-c.ctx.Done():
					return c.ctx.Err()
				case <-time.After(interval):
					continue
				}
			}
			continue
		}

		// 开始处理SSE流
		err = sseResp.ForEach(handler)

		// 如果已经接收到数据，不再重试
		if sseResp.HasReceivedData() {
			return err
		}

		// 如果没有接收到数据且满足重试条件，继续重试
		lastErr = err
		if attempt < maxAttempts-1 {
			interval := retryConfig.RetryInterval
			if retryConfig.BackoffMultiplier > 1.0 {
				multiplier := 1.0
				for i := 0; i < attempt; i++ {
					multiplier *= retryConfig.BackoffMultiplier
				}
				interval = time.Duration(float64(interval) * multiplier)

				if retryConfig.MaxRetryInterval > 0 && interval > retryConfig.MaxRetryInterval {
					interval = retryConfig.MaxRetryInterval
				}
			}

			select {
			case <-c.ctx.Done():
				return c.ctx.Err()
			case <-time.After(interval):
				continue
			}
		}
	}

	return lastErr
}
