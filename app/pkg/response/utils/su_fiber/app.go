package su_fiber

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jinzhu/copier"
)

// 一不做二不休，直接在这里提供一个单例
var appIns *App
var appInsOnce = new(sync.Once)

func AppIns(conf ...interface{}) *App {
	appInsOnce.Do(
		//初始化
		func() {
			if len(conf) == 0 {
				panic("请传入配置参数")
			}
			appIns = &App{}
			appIns.InitApp(conf)
		},
	)
	return appIns
}

// 配置属性可能会更新
type Config struct {
	Name    string
	Addr    string
	TimeOut int
	// 默认 4 * 1024 * 1024 = 4MB
	BodyLimit         int
	EnablePrintRoutes bool
}

type App struct {
	a   *fiber.App
	ctf Config
}

func (t *App) checkInit() {
	if t.a == nil {
		panic("请先配置app")
	}
}

// InitApp 返回一个新的fiberApp。 conf 配置，传入值结构至少有 Config 里的属性信息
func (t *App) InitApp(conf interface{}) {
	var cf Config
	err := copier.Copy(&cf, conf)

	if err != nil {
		panic(err)
	}

	if cf.BodyLimit <= 0 {
		cf.BodyLimit = 4 * 1024 * 1024
	}

	t.ctf = cf
	//这里的配置很多，后续可自定义
	t.a = fiber.New(fiber.Config{
		CaseSensitive:           true,
		AppName:                 cf.Name,
		ReadTimeout:             time.Millisecond * time.Duration(cf.TimeOut),
		WriteTimeout:            time.Millisecond * time.Duration(cf.TimeOut),
		EnableTrustedProxyCheck: true,
		BodyLimit:               cf.BodyLimit,
		EnablePrintRoutes:       cf.EnablePrintRoutes,
	})
}

func (t *App) Fiber() *fiber.App {
	t.checkInit()
	return t.a
}

func (t *App) Run() <-chan error {
	t.checkInit()
	c := make(chan error)
	go func() {
		c <- t.a.Listen(t.ctf.Addr)
	}()
	return c
}
