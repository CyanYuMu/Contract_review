package adapter

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/sentinel/limiter"
)

func FiberTaker(input interface{}, key string) (v string) {
	ctx, ok := input.(*fiber.Ctx)
	if !ok {
		return ""
	}
	if strings.HasPrefix(key, "$") {
		switch key {
		case limiter.SourceKeyIp:
			if ip := ctx.Get("X-Real-IP"); ip != "" {
				v = ip
			} else if ip = ctx.Get("X-Forwarded-For"); ip != "" {
				v = ip
			}
			//本地ip
			if v == "::1" {
				v = "127.0.0.1"
			}
			return
		case limiter.SourceKeyUri:
			return string(ctx.Request().RequestURI())
		default:
			return
		}
	} else {
		v = ctx.Get(key)
		if v == "" {
			v = ctx.Query(key)
		}

		return
	}
}

/*FiberMiddleware
* @Description: fiber 限流中间件
* @param l * 限流器
* @param errorHandler ? 如果为nil会触发默认处理行为
* @return func(ctx *fiber.Ctx) error
 */
//func FiberMiddleware(l sentinel.Limiter, errorHandler func(ctx *fiber.Ctx, err error) error) func(ctx *fiber.Ctx) error {
//	return func(ctx *fiber.Ctx) error {
//		c := context.Background()
//		uri := string(ctx.Request().RequestURI())
//		entry, err := l.Take(c, ctx, uri)
//		if err != nil {
//			if errorHandler != nil {
//				return errorHandler(ctx, err)
//			} else {
//				if err == limiter.SourceLimiterNotDefined {
//					return ctx.Next()
//				} else if err == limiter.SourceUndefined {
//					ctx.Status(http.StatusNoContent)
//					return nil
//				} else {
//					ctx.Status(http.StatusTooManyRequests)
//					return nil
//				}
//			}
//		} else {
//			defer entry.Exit()
//			return ctx.Next()
//		}
//	}
//}
