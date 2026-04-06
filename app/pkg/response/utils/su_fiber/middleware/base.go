package middleware

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/stack"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

// Cors 跨域中间件
func Cors() func(c *fiber.Ctx) error {
	return cors.New(cors.Config{
		//Next func(c *fiber.Ctx) bool ，返回值若为true，跳过此中间件
		Next: nil,
		//来源设定
		AllowOrigins: "*",
		//允许的方法
		AllowMethods: strings.Join([]string{
			fiber.MethodGet,
			fiber.MethodPost,
			fiber.MethodHead,
			fiber.MethodPut,
			fiber.MethodDelete,
			fiber.MethodPatch,
			fiber.MethodOptions,
		}, ","),
	})
}

// Logger 请求打印
func Logger() func(c *fiber.Ctx) error {
	return logger.New()
}

func PanicRecover() func(c *fiber.Ctx) error {
	return recover.New(recover.Config{
		EnableStackTrace: true,
		StackTraceHandler: func(ctx *fiber.Ctx, err interface{}) {
			stackTrace := stack.Get(&stack.Option{
				Levels:    10,
				Size:      2048,
				Separator: "\n",
				Skip:      5,
			})
			su_logger.Error(ctx.Context(), nil, "panic_recover", su_logger.E().Any("err", err).String("stack_trace", stackTrace))
			time.Sleep(time.Second)
			//fmt.Println(stackTrace)
		},
	})
}

// Healthy 给k8s健康检查探针使用
func Healthy() func(c *fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		auth := c.Get("X-Check-Auth")
		if auth == "health-check" {
			_, _ = c.WriteString("ok")
			return nil
		} else {
			c.Status(fiber.StatusNotFound)
			return nil
		}
	}
}
