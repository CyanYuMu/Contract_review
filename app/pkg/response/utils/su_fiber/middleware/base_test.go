package middleware

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func doSomething() {
	panic("boom")
}

func TestPanicRecover_LogsStackAndReturns500(t *testing.T) {
	// 初始化日志到内存缓冲区，便于断言
	var buf bytes.Buffer
	//enc := su_logger.NewKeyValueEncoder(&su_logger.Config{Level: zapcore.DebugLevel, Colorful: false, CallerSkip: 0})
	//su_logger.Init(&su_logger.Config{Level: zapcore.DebugLevel, Colorful: false, Writer: zapcore.AddSync(&buf), CallerSkip: 1, Encoder: enc})

	app := fiber.New()
	app.Use(PanicRecover())
	app.Get("/panic", func(c *fiber.Ctx) error {
		doSomething()

		return nil
	})

	req := httptest.NewRequest("GET", "/panic", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}

	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", fiber.StatusInternalServerError, resp.StatusCode)
	}

	out := buf.String()
	if !strings.Contains(out, "panic_recover") {
		t.Fatalf("expected log to contain 'panic_recover', got: %s", out)
	}
	if !strings.Contains(out, "err=boom") {
		t.Fatalf("expected log to contain 'err=boom', got: %s", out)
	}
	if !strings.Contains(out, "stack_trace=") {
		t.Fatalf("expected log to contain 'stack_trace=', got: %s", out)
	}
}
