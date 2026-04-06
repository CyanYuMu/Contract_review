package su_fiber

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/utils"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/enum"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/geo"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_error"
)

type BaseController struct {
}

func (b *BaseController) CountryCode(ctx *fiber.Ctx) string {
	// 优先从 header 头中读取 X-Gateway-Country, 这个值是从网关主动透传过来, 这一次运维很主动
	countryCode := ctx.Get(enum.HeaderXGatewayCountry)
	if countryCode != "" {
		return countryCode
	}
	ip := b.GetIP(ctx)
	region, err := geo.DefaultGeo.Ip2Region(ctx.Context(), ip)
	if err != nil {
		return ""
	}

	return region.CountryCode
}

func (b *BaseController) ParseIP(ctx context.Context, ip string) (geo.Region, error) {
	region, err := geo.DefaultGeo.Ip2Region(ctx, ip)
	if err != nil {
		return geo.Region{}, err
	}
	return region, nil
}

func (b *BaseController) GetIP(ctx *fiber.Ctx) (ip string) {
	defer func() {
		if ip != "" {
			ip = strings.TrimSpace(ip)
		}
	}()
	for _, k := range []string{"X-Forwarded-For", "X-Real-Ip"} {
		ip = ctx.Get(k)
		if ip != "" {
			if strings.Contains(ip, ",") {
				ip = strings.Split(ip, ",")[0]
			}
			break
		}
	}

	if ip == "" {
		ip = "0.0.0.0"
	}

	return ip
}

type Status struct {
	// 业务code
	Code int64 `json:"code" validate:"required"`
	// 业务msg
	Msg string `json:"msg" validate:"required"`
	// RequestId
	RequestId string `json:"request_id" validate:"required"`
}

type Rsp struct {
	Data   interface{} `json:"data"`
	Status Status      `json:"status"`
}

// GetRequestId 获取request id
func GetRequestId(ctx *fiber.Ctx) string {
	var requestId string
	if val := ctx.Context().UserValue(enum.CtxRequestId); val != nil {
		requestId = val.(string)
	}
	return requestId
}

// GetTrackingParams 获取跟踪参数
func GetTrackingParams(ctx *fiber.Ctx) string {
	var trackingParams string
	if val := ctx.Context().UserValue(enum.CtxTrackingParams); val != nil {
		trackingParams = val.(string)
	}
	return trackingParams
}

// SuccessJSON 响应成功数据
func (b *BaseController) SuccessJSON(ctx *fiber.Ctx, data ...interface{}) error {
	if len(data) > 0 {
		return ctx.JSON(Rsp{
			Data: data[0],
			Status: Status{
				Code:      enum.SystemSuccessCode,
				Msg:       enum.SystemSuccessMsg,
				RequestId: GetRequestId(ctx),
			},
		})
	}
	return ctx.JSON(Rsp{
		Status: Status{
			Code:      enum.SystemSuccessCode,
			Msg:       enum.SystemSuccessMsg,
			RequestId: GetRequestId(ctx),
		},
	})
}

func (b *BaseController) ErrorResponse(ctx *fiber.Ctx, err error) error {
	code, msg := su_error.Parse(err)
	if code == 0 {
		code = enum.SystemServerErrorCode
	}

	if msg == "" {
		msg = enum.SystemServerErrorMsg
	}

	return b.FailJSON(ctx, int64(code), msg)
}

func (b *BaseController) ParamError(ctx *fiber.Ctx) error {
	return b.FailJSON(ctx, enum.SystemParamsErrorCode, enum.SystemParamsErrorMsg)
}

func (b *BaseController) NotFound(ctx *fiber.Ctx) error {
	return b.FailJSON(ctx, enum.SystemSourceNotFound, enum.SystemSourceNotFoundMsg)
}

func (b *BaseController) ServerError(ctx *fiber.Ctx) error {
	return b.FailJSON(ctx, enum.SystemServerErrorCode, enum.SystemServerErrorMsg)
}

// FailJSON 响应失败数据
func (b *BaseController) FailJSON(ctx *fiber.Ctx, code int64, msg string, data ...interface{}) error {
	if code == 0 {
		code = enum.SystemServerErrorCode
	}
	if msg == "" {
		msg = enum.SystemServerErrorMsg
	}
	if len(data) > 0 {
		return ctx.JSON(Rsp{
			Data: data[0],
			Status: Status{
				Code:      code,
				Msg:       msg,
				RequestId: GetRequestId(ctx),
			},
		})
	}
	return ctx.JSON(Rsp{
		Status: Status{
			Code:      code,
			Msg:       msg,
			RequestId: GetRequestId(ctx),
		},
	})
}

func (b *BaseController) AppId(ctx *fiber.Ctx) string {
	return getFirstValue(ctx.GetReqHeaders(), enum.HeaderAppId)
}

// SafeHeaderGet 安全获取header的值
func (b *BaseController) SafeHeaderGet(ctx *fiber.Ctx, key string) string {
	if ctx == nil {
		return ""
	}
	return utils.CopyString(ctx.Get(key))
}

func (b *BaseController) UnsafeAccountId(ctx *fiber.Ctx) string {
	if ctx == nil {
		return ""
	}

	return ctx.Get(enum.HeaderAccountId)
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

func (b *BaseController) ProjectId(ctx *fiber.Ctx) string {
	return getFirstValue(ctx.GetReqHeaders(), enum.HeaderXProjectId)
}

func (b *BaseController) AppVersion(ctx *fiber.Ctx) string {
	return getFirstValue(ctx.GetReqHeaders(), enum.HeaderVersion)
}

func (b *BaseController) DeviceId(ctx *fiber.Ctx) string {
	return getFirstValue(ctx.GetReqHeaders(), enum.HeaderDeviceId)
}

func (b *BaseController) DeviceOs(ctx *fiber.Ctx) string {
	return getFirstValue(ctx.GetReqHeaders(), enum.HeaderDeviceOs)
}

func (b *BaseController) Plat(ctx *fiber.Ctx) string {
	return getFirstValue(ctx.GetReqHeaders(), enum.HeaderPlatform)
}

func (b *BaseController) PageId(ctx *fiber.Ctx) string {
	return getFirstValue(ctx.GetReqHeaders(), enum.HeaderPageId)
}
