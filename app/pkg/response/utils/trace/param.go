package trace

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/enum"
)

// ================ 埋点参数编码解码功能 ================
// TrackingParams 埋点服务基础参数结构体
type TrackingParams struct {
	AppId       string // 应用ID (header)
	AppVersion  string // 应用版本 (header)
	Refer       string // 来源页面 (header)
	PageLifeId  string // 页面生命周期ID (header), 前端主动上报
	CountryCode string // 国家代码 (IP转换)
	Ip          string // IP地址 (header)
	DeviceOs    string // 设备系统 (header)
	// SearchBot   string // 搜索引擎机器人 (header.From && header.User-Agent)
	Plat      string // 平台 (header)
	AccountId string // 账户ID
	DeviceId  string // 设备ID (header)
	From      string // 来源 (header.From), 比如识别爬虫
}

// EncodeTrackingParams 将埋点参数编码为字符串
// 使用紧凑键值对编码，格式：av=value&r=value&pl=value...
// 键名映射：av=AppVersion, r=Refer, pl=PageLifeId, cc=CountryCode, ai=AppId, do=DeviceOs, f=From, pt=Plat, ac=AccountId, di=DeviceId, ip=Ip
func EncodeTrackingParams(params TrackingParams) (string, error) {
	var parts []string

	// 只编码非空字段，使用简短键名以减少编码长度
	if params.AppVersion != "" {
		parts = append(parts, "av="+escapeValue(params.AppVersion))
	}
	if params.Refer != "" {
		parts = append(parts, "r="+escapeValue(params.Refer))
	}
	if params.PageLifeId != "" {
		parts = append(parts, "pl="+escapeValue(params.PageLifeId))
	}
	if params.CountryCode != "" {
		parts = append(parts, "cc="+escapeValue(params.CountryCode))
	}
	if params.AppId != "" {
		parts = append(parts, "ai="+escapeValue(params.AppId))
	}
	if params.DeviceOs != "" {
		parts = append(parts, "do="+escapeValue(params.DeviceOs))
	}
	if params.From != "" {
		parts = append(parts, "f="+escapeValue(params.From))
	}
	if params.Plat != "" {
		parts = append(parts, "pt="+escapeValue(params.Plat))
	}
	if params.AccountId != "" {
		parts = append(parts, "ac="+escapeValue(params.AccountId))
	}
	if params.DeviceId != "" {
		parts = append(parts, "di="+escapeValue(params.DeviceId))
	}
	if params.Ip != "" {
		parts = append(parts, "ip="+escapeValue(params.Ip))
	}

	if len(parts) == 0 {
		return "", nil
	}

	// 拼接并base64编码
	encoded := strings.Join(parts, "&")
	return base64.StdEncoding.EncodeToString([]byte(encoded)), nil
}

// DecodeTrackingParams 将编码字符串解码为埋点参数
func DecodeTrackingParams(encoded string) (*TrackingParams, error) {
	if encoded == "" {
		return &TrackingParams{}, nil
	}

	// base64解码
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode base64 failed: %w", err)
	}

	// 解析键值对
	params := &TrackingParams{}
	pairs := strings.Split(string(decoded), "&")

	for _, pair := range pairs {
		if pair == "" {
			continue
		}

		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := unescapeValue(parts[1])

		// 根据键名设置对应字段
		switch key {
		case "av":
			params.AppVersion = value
		case "r":
			params.Refer = value
		case "pl":
			params.PageLifeId = value
		case "cc":
			params.CountryCode = value
		case "ai":
			params.AppId = value
		case "do":
			params.DeviceOs = value
		case "f":
			params.From = value
		case "pt":
			params.Plat = value
		case "ac":
			params.AccountId = value
		case "di":
			params.DeviceId = value
		case "ip":
			params.Ip = value
		}
	}

	return params, nil
}

// SetTrackingParamsToContext 将埋点参数设置到context中
func SetTrackingParamsToContext(ctx context.Context, params TrackingParams) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	// 编码参数
	encoded, err := EncodeTrackingParams(params)
	if err != nil {
		// 编码失败时返回原context
		return ctx
	}

	// 存储到context中
	return AssignToContext(ctx, enum.CtxTrackingParams, encoded)
}

// GetTrackingParamsFromContext 从context中获取埋点参数
func GetTrackingParamsFromContext(ctx context.Context) TrackingParams {
	if ctx == nil {
		return TrackingParams{}
	}

	// 从context中获取编码字符串
	encoded := GetFromContext(ctx, enum.CtxTrackingParams)
	if encoded == "" {
		return TrackingParams{}
	}

	// 解码参数
	params, err := DecodeTrackingParams(encoded)
	if err != nil {
		// 解码失败时返回空参数
		return TrackingParams{}
	}

	return *params
}

// UpdateTrackingParamsInContext 更新context中的埋点参数 (合并模式)
func UpdateTrackingParamsInContext(ctx context.Context, updater func(*TrackingParams)) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	// 获取现有参数
	params := GetTrackingParamsFromContext(ctx)

	// 执行更新函数
	if updater != nil {
		updater(&params)
	}

	// 设置回context
	return SetTrackingParamsToContext(ctx, params)
}

// ================ 快速字段获取功能 ================

// GetTrackingFieldFromContext 从context中快速获取指定字段值，无需完整解码
// fieldKey: av, r, pl, cc, ai, do, f, pt, ac, di, ip
func GetTrackingFieldFromContext(ctx context.Context, fieldKey string) string {
	if ctx == nil {
		return ""
	}

	// 从context中获取编码字符串
	encoded := GetFromContext(ctx, enum.CtxTrackingParams)
	if encoded == "" {
		return ""
	}

	return GetTrackingFieldFromEncoded(encoded, fieldKey)
}

// GetTrackingFieldFromEncoded 从编码字符串中快速获取指定字段值
// fieldKey: av, r, pl, cc, ai, do, f, pt, ac, di, ip
func GetTrackingFieldFromEncoded(encoded, fieldKey string) string {
	if encoded == "" || fieldKey == "" {
		return ""
	}

	// base64解码
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return ""
	}

	// 查找目标字段，使用字符串搜索避免完整解析
	decodedStr := string(decoded)
	searchKey := fieldKey + "="

	// 查找键的位置
	startIdx := strings.Index(decodedStr, searchKey)
	if startIdx == -1 {
		return ""
	}

	// 确保找到的是完整的键名（不是子字符串）
	if startIdx > 0 && decodedStr[startIdx-1] != '&' {
		return ""
	}

	// 获取值的起始位置
	valueStart := startIdx + len(searchKey)
	if valueStart >= len(decodedStr) {
		return ""
	}

	// 查找值的结束位置（到下一个&或字符串末尾）
	valueEnd := strings.Index(decodedStr[valueStart:], "&")
	if valueEnd == -1 {
		valueEnd = len(decodedStr) - valueStart
	}

	// 提取并反转义值
	value := decodedStr[valueStart : valueStart+valueEnd]
	return unescapeValue(value)
}

// 便捷函数：获取常用字段
func GetAppIdFromContext(ctx context.Context) string {
	return GetTrackingFieldFromContext(ctx, "ai")
}

func GetAccountIdFromContext(ctx context.Context) string {
	return GetTrackingFieldFromContext(ctx, "ac")
}

func GetDeviceIdFromContext(ctx context.Context) string {
	return GetTrackingFieldFromContext(ctx, "di")
}

func GetAppVersionFromContext(ctx context.Context) string {
	return GetTrackingFieldFromContext(ctx, "av")
}

func GetCountryCodeFromContext(ctx context.Context) string {
	return GetTrackingFieldFromContext(ctx, "cc")
}

func GetPlatFromContext(ctx context.Context) string {
	return GetTrackingFieldFromContext(ctx, "pt")
}

func GetFromFromContext(ctx context.Context) string {
	return GetTrackingFieldFromContext(ctx, "f")
}

// escapeValue 转义字段值中的特殊字符
// 将 & 转义为 \&，将 = 转义为 \=，将 \ 转义为 \\
func escapeValue(value string) string {
	if value == "" {
		return ""
	}

	// 检查是否包含需要转义的字符，如果不包含则直接返回
	if !strings.ContainsAny(value, "\\&=") {
		return value
	}

	// 先转义反斜杠，再转义其他特殊字符
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "&", "\\&")
	escaped = strings.ReplaceAll(escaped, "=", "\\=")
	return escaped
}

// unescapeValue 反转义字段值
// 将 \& 转义为 &，将 \= 转义为 =，将 \\ 转义为 \
func unescapeValue(value string) string {
	if value == "" {
		return ""
	}

	// 检查是否包含转义字符，如果不包含则直接返回
	if !strings.Contains(value, "\\") {
		return value
	}

	// 先反转义特殊字符，再反转义反斜杠
	unescaped := strings.ReplaceAll(value, "\\&", "&")
	unescaped = strings.ReplaceAll(unescaped, "\\=", "=")
	unescaped = strings.ReplaceAll(unescaped, "\\\\", "\\")
	return unescaped
}
