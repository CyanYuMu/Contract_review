package middleware

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/utils"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/enum"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/trace"
)

type TracerParam struct {
	EnableLogger bool
	// 是否记录请求参数
	RecordRequestBody bool
	// 当RecordRequestBody为true时, 该值表示最大记录的请求参数长度, 超过这个长度就不打印日志了
	MaximumBodySize int
	// 仅记录延迟大于该值的请求, 当为0时, 打印所有请求的日志, 单位毫秒, > 0 的时候仅打印超过该值的请求
	ElapsedThreshold time.Duration
	// 忽略的请求, 针对流式接口，就不要打印日志了
	Ignore func(route string) bool
	// 自定义匹配更新的路由
	LogRoute func(route string) (log bool, logReq bool, logRsp bool)
	// 开启失败响应日志
	IsErrorResponse func(httpStatus int, rsp []byte) bool
	// 是否记录响应体
	RecordResponseBody bool
}

func Tracer(param TracerParam) func(c *fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		// 判断前缀/api/stream
		reqPath := utils.CopyString(c.Path())

		headers := c.GetReqHeaders()
		var reqId string
		var spanId string
		var appId string
		var deviceId string
		var trackingParams string
		var actId string

		if headers != nil {
			if actIds := headers[enum.HeaderAccountId]; len(actIds) > 0 {
				actId = actIds[0]
			}
			if reqIds := headers[enum.HeaderRequestId]; len(reqIds) > 0 {
				reqId = reqIds[0]
				c.Context().SetUserValue(enum.CtxRequestId, reqId)
			} else {
				// new trace id
				reqId = trace.NewRequestId()
				c.Context().SetUserValue(enum.CtxRequestId, reqId)
			}
			if spanIds := headers[enum.HeaderSpanId]; len(spanIds) > 0 {
				spanId = spanIds[0]
				c.Context().SetUserValue(enum.CtxSpanId, spanId)
			}
			if appIds := headers[enum.HeaderAppId]; len(appIds) > 0 {
				appId = appIds[0]
				c.Context().SetUserValue(enum.CtxAppId, appId)
			}
			if debugs := headers[enum.HeaderDebug]; len(debugs) > 0 {
				c.Context().SetUserValue(enum.CtxDebug, "true")
			}
			if trackingParamss := headers[enum.HeaderTrackingParams]; len(trackingParamss) > 0 {
				trackingParams = trackingParamss[0]
				c.Context().SetUserValue(enum.CtxTrackingParams, trackingParams)
			} else {
				traceParam := NewTrackingParams(c, headers)
				traceParamStr, _ := trace.EncodeTrackingParams(traceParam)
				if traceParamStr != "" {
					c.Context().SetUserValue(enum.CtxTrackingParams, traceParamStr)
				}
			}

			if deviceIds := headers[enum.HeaderDeviceId]; len(deviceIds) > 0 {
				deviceId = deviceIds[0]
				c.Context().SetUserValue(enum.CtxDeviceId, deviceId)
			}
		} else {
			c.Context().SetUserValue(enum.CtxRequestId, trace.NewRequestId())
		}

		if param.Ignore != nil && param.Ignore(reqPath) {
			return c.Next()
		}

		if !param.EnableLogger {
			return c.Next()
		}

		t1 := time.Now()
		reqErr := c.Next()
		escaped := time.Since(t1)
		logFlag := false
		msg := "reqNormal"

		if param.ElapsedThreshold > 0 && escaped >= param.ElapsedThreshold {
			logFlag = true
			msg = "reqTooLong"
		}

		routePath := utils.CopyString(c.Path())
		httpStatus := c.Response().StatusCode()
		ct := c.Response().Header.ContentType()
		if string(ct) != "application/json" {
			// trace response body is not json, skip
			return reqErr
		}
		var logRoute, logReq, logRsp bool
		if param.LogRoute != nil {
			logRoute, logReq, logRsp = param.LogRoute(routePath)
		}

		if !logFlag && logRoute {
			logFlag = true
		}

		if !logFlag && param.IsErrorResponse != nil && param.IsErrorResponse(httpStatus, c.Response().Body()) {
			logFlag = true
			// 记录错误日志
			if msg == "reqTooLong" {
				msg = "reqTooLongAndError"
			} else {
				msg = "reqError"
			}
		}

		var reqBody string
		var rspBody string

		if param.RecordRequestBody || logReq {
			reqBody = string(c.Request().Body())
			if param.MaximumBodySize > 0 && len(reqBody) > param.MaximumBodySize {
				reqBody = reqBody[:param.MaximumBodySize]
			}
		}

		if param.RecordResponseBody || logRsp {
			rspBody = string(c.Response().Body())
			if param.MaximumBodySize > 0 && len(rspBody) > param.MaximumBodySize {
				rspBody = rspBody[:param.MaximumBodySize]
			}
		}

		if logFlag {
			su_logger.Info(c.Context(), msg, su_logger.E().String("path", routePath).Int("status", httpStatus).Int64("elapsed", escaped.Milliseconds()).String("appId", appId).String("deviceId", deviceId).String("actId", actId).String("req", reqBody).String("rsp", rspBody))
		}

		return reqErr
	}
}

func GetIp(ctx *fiber.Ctx) (ip string) {
	for _, k := range []string{"X-Forwarded-For", "X-Real-Ip"} {
		ip = ctx.Get(k)
		if ip != "" {
			if strings.Contains(ip, ",") {
				ip = strings.Split(ip, ",")[0]
			}
			return ip
		}
	}
	return ""
}

func NewTrackingParams(ctx *fiber.Ctx, headers map[string][]string) trace.TrackingParams {
	if len(headers) == 0 {
		headers = ctx.GetReqHeaders()
	}
	// 基于服务的配置, 生成一个跟踪参数
	appId := getFirstValue(headers, enum.HeaderAppId)
	appVersion := getFirstValue(headers, enum.HeaderVersion)
	refer := getFirstValue(headers, enum.HeaderReferer)
	pageLifeId := getFirstValue(headers, enum.HeaderPageId)
	ip := GetIp(ctx)
	deviceOs := getFirstValue(headers, enum.HeaderDeviceOs)
	plat := getFirstValue(headers, enum.HeaderPlatform)
	accountId := getFirstValue(headers, enum.HeaderAccountId)
	deviceId := getFirstValue(headers, enum.HeaderDeviceId)
	from := getFirstValue(headers, enum.HeaderFrom)

	traceParam := trace.TrackingParams{
		AppId:       appId,
		AppVersion:  appVersion,
		Refer:       refer,
		PageLifeId:  pageLifeId,
		CountryCode: "",
		Ip:          ip,
		DeviceOs:    deviceOs,
		Plat:        plat,
		AccountId:   accountId,
		DeviceId:    deviceId,
		From:        from,
	}

	return traceParam
}

func getFirstValue(header map[string][]string, key string) string {
	if len(header) == 0 {
		return ""
	}

	for k := range header {
		if strings.EqualFold(k, key) {
			return header[k][0]
		}
	}

	return ""
}
