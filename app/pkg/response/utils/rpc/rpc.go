package rpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/imroc/req/v3"
	"github.com/tidwall/gjson"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/su_json"
	seagoReq "gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/http/req"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_error"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

// Request 请求对象，包含完整的请求信息
type RequestParam struct {
	Body    interface{}       // 请求体数据
	Headers map[string]string // 请求头
}

// Client RPC客户端
type Client struct {
	// 多环境的客户端链接, 管理
	lock        sync.Mutex
	client      *seagoReq.Client
	serviceName string // 服务名称
	Param       Param  // 参数
	baseHost    string
}

// ServiceName 获取服务名称
func (c *Client) ServiceName() string {
	return c.serviceName
}

func (c *Client) ReqTimeTooLongThreshold() time.Duration {
	if c.Param.ReqTimeTooLongThreshold == 0 {
		return 0
	}
	return c.Param.ReqTimeTooLongThreshold
}

// CallOptions 调用选项
type CallOptions struct {
	Timeout                    time.Duration // 超时时间
	RetryCount                 int           // 重试次数
	RetryInterval              time.Duration // 重试间隔
	RequestTooLongLogThreshold time.Duration // 请求耗时过长阈值, 超过该阈值则记录日志, 0 表示不记录
}

func (c *CallOptions) GetTimeout() time.Duration {
	var dft = 10 * time.Second
	if c == nil {
		return dft
	}
	if c.Timeout == 0 {
		return dft
	}
	return c.Timeout
}

func (c *CallOptions) GetRequestTooLongLogThreshold() time.Duration {
	if c == nil {
		return 0
	}
	return c.RequestTooLongLogThreshold
}

func (c *CallOptions) GetRetryCount() int {
	var dft = 0
	if c == nil {
		return dft
	}
	return c.RetryCount
}

func (c *CallOptions) GetRetryInterval() time.Duration {
	var dft = 1 * time.Second
	if c == nil {
		return dft
	}
	return c.RetryInterval
}

// Response 响应对象
type Response struct {
	Header     http.Header
	Body       io.ReadCloser
	Status     int
	StatusDesc string
}

func (r *Response) IsHttpError() error {
	if r == nil {
		return errors.New("response is nil")
	}
	if r.Status >= 200 && r.Status < 300 {
		return nil
	}
	return errors.New(r.StatusDesc)
}

func (r *Response) IsBodyError(body []byte) error {
	if body == nil {
		return errors.New("response body is nil")
	}
	// 成功路径：子串匹配，避免 gjson 解析（绝大多数 RPC 成功响应）
	// 虽然通过快速路径会有概率重复匹配, 但为了性能, 还是使用快速路径
	ok := strings.Contains(string(body), `"code":10000`) || strings.Contains(string(body), `"code": 10000`)
	if ok {
		return nil
	}
	status := gjson.GetBytes(body, "status.code").Int()
	msg := gjson.GetBytes(body, "status.msg").String()
	traceId := gjson.GetBytes(body, "status.trace_id").String()

	return fmt.Errorf("rpc server response error: status: %d, msg: %s, trace_id: %s", status, msg, traceId)
}

func (r *Response) UnmarshalData(data interface{}) (err error) {
	if err = r.IsHttpError(); err != nil {
		return err
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if err = r.IsBodyError(body); err != nil {
		return err
	}

	jsonStr := gjson.GetBytes(body, "data").String()
	su_json.MustUnmarshalFromString(jsonStr, data)

	return nil
}

func (r *Response) Unmarshal(data interface{}) (err error) {
	if err = r.IsHttpError(); err != nil {
		return err
	}
	byteData, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	if err = r.IsBodyError(byteData); err != nil {
		return err
	}

	su_json.MustUnmarshal(byteData, data)

	return nil
}

type Param struct {
	seagoReq.Options
	ReqTimeTooLongThreshold time.Duration // 请求耗时过长阈值, 超过该阈值则记录日志

	Client *seagoReq.Client
}

// New 创建新的RPC客户端
func New(serviceName string, baseHost string, param Param) *Client {
	var cli *seagoReq.Client
	if param.Client != nil {
		cli = param.Client
	} else {
		cli = seagoReq.GetClient(baseHost, &param.Options)
	}
	return &Client{
		client:      cli,
		serviceName: serviceName,
		Param:       param,
		lock:        sync.Mutex{},
		baseHost:    baseHost,
	}
}

// Call 执行HTTP调用
func (c *Client) Call(ctx context.Context, reqPath string, request *RequestParam, opts *CallOptions) (*Response, error) {
	// 获取客户端
	if opts == nil {
		opts = &CallOptions{}
	}
	if opts.GetRequestTooLongLogThreshold() == 0 {
		opts.RequestTooLongLogThreshold = c.ReqTimeTooLongThreshold()
	}

	method := "POST"

	fullUrl := concatUrl(c.baseHost, reqPath)

	response, err := c.DoCall(ctx, c.client, method, fullUrl, request, opts)
	if err != nil {
		su_logger.Error(ctx, err, "rpcCallFailed",
			su_logger.E().String("service", c.serviceName).String("method", method).String("url", fullUrl).Any("request", request))
		return nil, err
	}

	return response, nil
}

func concatUrl(host string, reqPath string) string {
	aFlag := strings.HasSuffix(host, "/")
	bFlag := strings.HasPrefix(reqPath, "/")
	if aFlag && bFlag {
		return host + reqPath[1:]
	} else if aFlag {
		return host + reqPath
	} else if bFlag {
		return host + reqPath
	} else {
		return host + "/" + reqPath
	}
}

// DoCall 执行实际的HTTP调用
func (c *Client) DoCall(ctx context.Context, client *seagoReq.Client, method string, url string, request *RequestParam, opts *CallOptions) (*Response, error) {
	r := client.NewRequest()
	// 设置请求头
	if request != nil && request.Headers != nil {
		for k, v := range request.Headers {
			r.SetHeader(k, v)
		}
	}

	// 设置请求体
	if request != nil && request.Body != nil {
		r.SetContentType("application/json")
		r.SetBody(request.Body)
	}

	// 设置重试
	if opts.RetryCount > 0 {
		if opts.RetryCount > 0 {
			r.SetRetryCount(opts.RetryCount)
		}
		if opts.RetryInterval > 0 {
			r.SetRetryFixedInterval(opts.RetryInterval)
		}
	}

	seagoReq.AssignRequestCtx(r, ctx)

	// 执行请求
	var resp *req.Response
	var err error

	t1 := time.Now()
	switch strings.ToUpper(method) {
	case "GET":
		resp, err = r.SetContext(ctx).Get(url)
	case "POST":
		resp, err = r.SetContext(ctx).Post(url)
	case "PUT":
		resp, err = r.SetContext(ctx).Put(url)
	case "DELETE":
		resp, err = r.SetContext(ctx).Delete(url)
	case "PATCH":
		resp, err = r.SetContext(ctx).Patch(url)
	default:
		err = ErrInvalidMethodV2
		su_logger.Error(ctx, err, "invalid method",
			su_logger.E().String("method", method).String("url", url))
		return nil, err
	}
	reqTooLongThreshold := opts.GetRequestTooLongLogThreshold()
	extra := su_logger.E().String("service", c.serviceName).String("method", method).String("url", url)
	timeUse := time.Since(t1).String()
	var isReqTooLong = false
	if reqTooLongThreshold > 0 && time.Since(t1) > reqTooLongThreshold {
		extra = extra.String("cost", timeUse)
		isReqTooLong = true
	}

	if err != nil {
		su_logger.Error(ctx, err, "rpcCallErr", extra)
		return nil, err
	}

	if !resp.IsSuccessState() {
		su_logger.Error(ctx, err, "rpcCallFailed", extra.String("status", resp.Status))
		return &Response{
			Header:     resp.Header,
			Status:     resp.StatusCode,
			Body:       resp.Body,
			StatusDesc: resp.Status,
		}, err
	}

	// 记录成功日志
	if isReqTooLong {
		su_logger.Warn(ctx, "httpReqTooLong", extra)
	} else {
		su_logger.Debug(ctx, "httpReqSuccess", extra)
	}

	// 构建响应对象
	response := &Response{
		Header:     resp.Header,
		Status:     resp.StatusCode,
		Body:       resp.Body,
		StatusDesc: resp.Status,
	}

	return response, nil
}

// 错误定义
var (
	ErrInvalidMethodV2 = su_error.NewF(400, "invalid method")
)
