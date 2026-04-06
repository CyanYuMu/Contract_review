package server

import (
	"context"
	"errors"
	"flag"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/config"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/geo"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

// ========== App 结构体 ==========
type Server struct {
	// **** 必填项 ****
	// * server 整体配置
	config config.Config
	// **** 可选项 ****
	// fiber 中间件
	// middlewares []fiber.Handler
	// fiber 启动之前钩子
	beforeStart []HookFunc
	// 关闭之前钩子
	beforeShutdown []HookFunc
	// 关闭之后钩子
	afterShutdown []HookFunc
	// 关闭超时时间, 默认 10s
	shutdownTimeout time.Duration
	// fiber 应用
	app *fiber.App
	// 上下文
	ctx context.Context
	// 取消函数
	cancelFun          context.CancelFunc
	promethusCollector []prometheus.Collector
	// ? default /metrics
	promethusRoute string
}

func (s *Server) SetPromethusRoute(route string) *Server {
	s.promethusRoute = route
	return s
}

func (s *Server) GetPromethusRoute() string {
	if s == nil || s.promethusRoute == "" {
		return "/metrics"
	}
	if strings.HasPrefix(s.promethusRoute, "/") {
		return s.promethusRoute
	}
	return "/" + s.promethusRoute
}

func (s *Server) GetConfig() config.Config {
	return s.config
}

func (s *Server) GetFiberApp() *fiber.App {
	return s.app
}

func (s *Server) GetContext() context.Context {
	return s.ctx
}

type (
	// 生命周期钩子
	HookFunc func(ctx context.Context, server *Server) error
)

func New(ctx context.Context, cancelFun context.CancelFunc, cnf config.Config) *Server {
	s := &Server{
		config:          cnf,
		ctx:             ctx,
		cancelFun:       cancelFun,
		shutdownTimeout: time.Second * 10,
	}
	if cnf == nil {
		su_logger.Error(s.ctx, errors.New("config is nil"), "config is nil")
		panic("config is nil")
	}
	appCnf := cnf.GetApp()

	s.config = cnf
	if appCnf != nil && appCnf.Addr != "" {
		fiberCnf := getFiberConfig(appCnf)
		s.app = fiber.New(fiberCnf)
	}

	return s
}

func LoadConfig(cnf config.Config, cnfPath ...string) (err error) {
	ctx := context.Background()
	var confDir string
	flag.StringVar(&confDir, "c", "", "config dir")
	flag.Parse()
	if len(confDir) == 0 {
		var curDir string = "."
		if len(cnfPath) > 0 {
			curDir = cnfPath[0]
		}
		// 主动寻找配置文件
		confDir, err = config.TryFindConfigFile(curDir)
		if err != nil {
			return err
		}
	}
	su_logger.Info(ctx, "------ config dir: "+confDir)
	if err = config.Load(ctx, confDir, cnf); err != nil {
		su_logger.Error(ctx, err, "init config failed")
		return err
	}
	if strings.HasSuffix(confDir, ".yaml") {
		cnf.GetApp().ConfigDir = filepath.Dir(confDir)
	} else {
		cnf.GetApp().ConfigDir = confDir
	}
	su_logger.Info(ctx, "------ config loaded success")
	return nil
}

func getFiberConfig(app *config.App) fiber.Config {
	cnf := fiber.Config{
		BodyLimit:          4 * 1024 * 1024,
		CaseSensitive:      true,
		AppName:            "",
		ReadTimeout:        time.Minute,
		WriteTimeout:       time.Second * 30,
		IdleTimeout:        time.Minute * 5,
		ErrorHandler:       nil,
		DisableKeepalive:   false,
		DisableDefaultDate: false,
		ReadBufferSize:     4096 * 2,
		JSONEncoder:        sonic.Marshal,
		JSONDecoder:        sonic.Unmarshal,
		ETag:               false,
		EnablePrintRoutes:  true,
	}

	if app == nil {
		return cnf
	}

	if app.BodyLimit > 0 {
		cnf.BodyLimit = app.BodyLimit
	}

	if app.ReadTimeoutSec > 0 {
		cnf.ReadTimeout = time.Second * time.Duration(app.ReadTimeoutSec)
	}
	if app.WriteTimeoutSec > 0 {
		cnf.WriteTimeout = time.Second * time.Duration(app.WriteTimeoutSec)
	}

	if app.IdleTimeoutSec > 0 {
		cnf.IdleTimeout = time.Second * time.Duration(app.IdleTimeoutSec)
	}
	if app.ReadBufferSize > 0 {
		cnf.ReadBufferSize = app.ReadBufferSize
	}
	if app.WriteBufferSize > 0 {
		cnf.WriteBufferSize = app.WriteBufferSize
	}

	return cnf
}

func (s *Server) WithShutdownTimeout(timeout time.Duration) *Server {
	s.shutdownTimeout = timeout
	return s
}

// func (s *Server) Use(mw ...fiber.Handler) *Server {
// 	s.middlewares = append(s.middlewares, mw...)
// 	return s
// }

func (s *Server) EnableGeo() *Server {
	// 触发初始化
	geo.DefaultGeo.Ip2Region(s.ctx, "127.0.0.1")
	return s
}

func (s *Server) UsePromethusCollector(collector ...prometheus.Collector) *Server {
	s.promethusCollector = append(s.promethusCollector, collector...)
	return s
}

func (s *Server) BeforeStart(hook HookFunc) *Server {
	s.beforeStart = append(s.beforeStart, hook)
	return s
}

func (s *Server) BeforeShutdown(hook HookFunc) *Server {
	s.beforeShutdown = append(s.beforeShutdown, hook)
	return s
}

func (s *Server) AfterShutdown(hook HookFunc) *Server {
	s.afterShutdown = append(s.afterShutdown, hook)
	return s
}

// 启动主流程
func (s *Server) Run() error {
	defer s.cancelFun()
	if s.app != nil {
		s.registerBaseRoutes()
	}

	// fiber before start hooks
	for _, hook := range s.beforeStart {
		if err := hook(s.ctx, s); err != nil {
			return err
		}
	}
	var err error
	var errCh = make(chan error)
	if s.config.GetApp().Pprof != "" {
		go func() {
			errCh <- PprofHandle(s.config.GetApp().Pprof)
		}()
	}
	go func() {
		// 启动 fiber
		if s.app == nil {
			return
		}
		if len(s.promethusCollector) > 0 {
			if err := PrometheusHandle(s.app, s.promethusCollector, s.config.GetApp().Name, s.config.GetApp().Stage, s.GetPromethusRoute()); err != nil {
				su_logger.Error(s.ctx, err, "promethus handle failed")
			}
		}

		errCh <- s.app.Listen(s.config.GetApp().Addr)
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM)
	// 监听退出信号（可扩展）
	select {
	case err = <-errCh:
		su_logger.Error(s.ctx, err, "server error")
		for _, hook := range s.beforeShutdown {
			if err := hook(s.ctx, s); err != nil {
				return err
			}
		}
		if s.app != nil {
			_ = s.app.ShutdownWithTimeout(s.shutdownTimeout)
		}
		s.cancelFun()
		for _, hook := range s.afterShutdown {
			if err := hook(s.ctx, s); err != nil {
				return err
			}
		}
	case <-c:
		su_logger.Info(s.ctx, "api server shutdown by signal")
		for _, hook := range s.beforeShutdown {
			if err := hook(s.ctx, s); err != nil {
				return err
			}
		}
		_ = s.app.ShutdownWithTimeout(s.shutdownTimeout)
		s.cancelFun()
		for _, hook := range s.afterShutdown {
			if err := hook(s.ctx, s); err != nil {
				return err
			}
		}
	}

	return nil
}

// baseRouteHandler 基础路由处理器类型
type baseRouteHandler func(ctx *fiber.Ctx) error

// baseRoutes 基础路由配置
var baseRoutes = map[string]map[string]baseRouteHandler{
	"GET": {
		"/system/health": func(ctx *fiber.Ctx) error {
			auth := ctx.Get("X-Check-Auth")
			if auth == "health-check" {
				_, _ = ctx.WriteString("ok")
				return nil
			} else {
				ctx.Status(fiber.StatusNotFound)
				return nil
			}
		},
	},
	"POST": {
		// 可以在这里添加 POST 方法的基础路由
	},
	"PUT": {
		// 可以在这里添加 PUT 方法的基础路由
	},
	"DELETE": {
		// 可以在这里添加 DELETE 方法的基础路由
	},
}

// registerBaseRoutes 注册所有基础路由
func (s *Server) registerBaseRoutes() *Server {
	if s.app == nil {
		su_logger.Warn(s.ctx, "fiber app is nil, cannot register base routes")
		return s
	}

	registeredCount := 0
	for method, routes := range baseRoutes {
		for path, handler := range routes {
			// 检查路由是否已存在
			if s.isRouteRegistered(method, path) {
				su_logger.Debug(s.ctx, "base route already registered",
					su_logger.E().String("method", method).String("path", path))
				continue
			}

			// 注册路由
			switch method {
			case "GET":
				s.app.Get(path, handler)
			case "POST":
				s.app.Post(path, handler)
			case "PUT":
				s.app.Put(path, handler)
			case "DELETE":
				s.app.Delete(path, handler)
			case "PATCH":
				s.app.Patch(path, handler)
			default:
				su_logger.Warn(s.ctx, "unsupported HTTP method for base route",
					su_logger.E().String("method", method).String("path", path))
				continue
			}

			su_logger.Info(s.ctx, "base route registered",
				su_logger.E().String("method", method).String("path", path))
			registeredCount++
		}
	}

	if registeredCount > 0 {
		su_logger.Info(s.ctx, "base routes registration completed",
			su_logger.E().Int("count", registeredCount))
	}

	return s
}

// isRouteRegistered 检查指定路由是否已注册
func (s *Server) isRouteRegistered(method, path string) bool {
	if s.app == nil {
		return false
	}

	stack := s.app.Stack()
	for _, route := range stack {
		for _, handler := range route {
			if handler.Path == path && handler.Method == method {
				return true
			}
		}
	}
	return false
}
