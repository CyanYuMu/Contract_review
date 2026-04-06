package csrf

import (
	"github.com/gofiber/fiber/v2"
)

func NewFiberCsrf(ctx *fiber.Ctx) {
	ctx.Response().Header.Set("X-CSRF-Token", "1")
}
