package req

import (
	"crypto/tls"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/imroc/req/v3"
	jsoniter "github.com/json-iterator/go"
)

type ClientManager struct {
	clients map[string]*req.Client
	mu      sync.Mutex
}

var dftClientManager = &ClientManager{
	clients: make(map[string]*req.Client),
}

type Options struct {
	// MaxIdleConns controls the maximum number of idle (keep-alive)
	// connections across all hosts. Zero means no limit.
	MaxIdleConns int

	// MaxConnsPerHost optionally limits the total number of
	// connections per host, including connections in the dialing,
	// active, and idle states. On limit violation, dials will block.
	//
	// Zero means no limit.
	MaxConnsPerHost int

	// IdleConnTimeout is the maximum amount of time an idle
	// (keep-alive) connection will remain idle before closing
	// itself.
	// Zero means no limit.
	IdleConnTimeout time.Duration

	// ResponseHeaderTimeout, if non-zero, specifies the amount of
	// time to wait for a server's response headers after fully
	// writing the request (including its body, if any). This
	// time does not include the time to read the response body.
	ResponseHeaderTimeout time.Duration

	// ExpectContinueTimeout, if non-zero, specifies the amount of
	// time to wait for a server's first response headers after fully
	// writing the request headers if the request has an
	// "Expect: 100-continue" header. Zero means no timeout and
	// causes the body to be sent immediately, without
	// waiting for the server to approve.
	// This time does not include the time to send the request header.
	ExpectContinueTimeout time.Duration
	// This time does not include the time to send the request header.
	DisableAutoReadBody bool
	// hook after request
	AfterRequest func(resp *req.Response, err error)
}

/*GetHostClient
* @Description: 获取host的client对象
* @param host http://host:port
* @param opt
* @return *Client
 */
func GetHostClient(host string, opt *Options) *Client {
	client := dftClientManager.GetHostClient(host, opt)
	// 默认启用追踪
	return client.WithTracingRoundTripper()
}

/*GetHostClientWithTracing
* @Description: 获取带追踪的host的client对象
* @param host http://host:port
* @param opt
* @return *Client
 */
func GetHostClientWithTracing(host string, opt *Options) *Client {
	client := dftClientManager.GetHostClient(host, opt)
	return client.WithTracingRoundTripper()
}

/*
* @Description: 获取url的client对象
* @param url eg: http://host:port/path/to/api
* @param opt
* @return *Client
 */
func GetClient(url string, opt *Options) *Client {
	host := GetHost(url)
	client := dftClientManager.GetHostClient(host, opt)
	// 默认启用追踪
	return client.WithTracingRoundTripper()
}

/*GetClientWithTracing
* @Description: 获取带追踪的url的client对象
* @param url eg: http://host:port/path/to/api
* @param opt
* @return *Client
 */
func GetClientWithTracing(url string, opt *Options) *Client {
	client := GetClient(url, opt)
	return client.WithTracingRoundTripper()
}

// NewClientManager initializes a new ClientManager
func NewClientManager() *ClientManager {
	return &ClientManager{
		clients: make(map[string]*req.Client),
	}
}

func GetHost(urlStr string) string {
	// 如果已经是 host:port 格式，直接返回
	if strings.Contains(urlStr, ":") && !strings.HasPrefix(urlStr, "http") {
		return urlStr
	}

	// 尝试解析为 URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		// 如果解析失败，可能是纯 host，直接返回
		return urlStr
	}

	// 构建 host:port 格式
	host := parsedURL.Hostname()
	if host == "" {
		return urlStr
	}

	port := parsedURL.Port()
	if port == "" {
		// 根据 schema 设置默认端口
		switch parsedURL.Scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		default:
			// 如果没有 schema 或未知 schema，不添加端口
			return host
		}
	}

	return host + ":" + port
}

// GetHostClient retrieves or creates a client for the specified host
func (cm *ClientManager) GetHostClient(host string, opt *Options) *Client {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if client, exists := cm.clients[host]; exists {
		return &Client{
			c:          client,
			r:          client.NewRequest(),
			httpClient: client.GetClient(),
		}
	}

	client := req.NewClient().
		SetJsonMarshal(jsoniter.Marshal).
		SetJsonUnmarshal(jsoniter.Unmarshal).
		SetCommonRetryCount(0).
		SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true}).
		SetTimeout(60*time.Second).
		SetCommonHeader("User-Agent", "req/v3").
		SetCommonHeader("Accept", "application/json").
		SetCommonHeader("Content-Type", "application/json")

	trans := &req.Transport{}
	if opt != nil {
		trans.SetMaxIdleConns(opt.MaxIdleConns).SetMaxConnsPerHost(opt.MaxConnsPerHost).SetIdleConnTimeout(opt.IdleConnTimeout).SetResponseHeaderTimeout(opt.ResponseHeaderTimeout).SetExpectContinueTimeout(opt.ExpectContinueTimeout)
	}
	client.Transport = trans

	cm.clients[host] = client

	reqInst := client.NewRequest()

	if opt != nil && opt.DisableAutoReadBody {
		//reqInst.DisableAutoReadResponse()
		client.DisableAutoReadResponse()
	}

	return &Client{
		c:          client,
		r:          reqInst,
		httpClient: client.GetClient(),
	}
}
