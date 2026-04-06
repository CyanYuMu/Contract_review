package qwrobot

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/panjf2000/ants/v2"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/http/req"
)

const DefaultFreqLimit = 100

const (
	UnitHour   = "H"
	UnitMinute = "M"
	UnitSecond = "S"
)

const DefaultFreqUnit = UnitHour

const (
	// 常规信息
	LevelInfo = "info"
	// 警告信息
	LevelWarn = "warn"
	// 错误信息
	LevelError = "error"
	// 严重错误
	LevelFatal = "fatal"
)

type Config struct {
	// 机器人地址, 缺省值, 当未指定特定级别的机器人地址时使用
	Webhook string
	// ?
	WarnWebhook string
	// ?
	ErrorWebhook string
	// ?
	FatalWebhook string
	// 常规频率限制, 支持 n/S n/M n/H
	InfoFreqLimit string
	// 警告消息频率限制, 支持 n/S n/M n/H
	WarnFreqLimit string
	// 错误消息频率限制, 支持 n/S n/M n/H
	ErrorFreqLimit string
	// 严重错误消息频率限制, 支持 n/S n/M n/H
	FatalFreqLimit string
	// 消息类型, 自定义, 区分消息类型
	MessageType string
	// key前缀, 区分业务域,非必填
	Prefix string
	// 自定义限流器
	Limiter  Limiter
	HostName string
	Ip       string
	Stage    string
	// 用户id映射关系
	Users Users
}

type Users []User

type User struct {
	Name string
	Id   string
}

func (u Users) GetIdByName(name string) string {
	if len(u) == 0 {
		return name
	}
	for i := range u {
		if u[i].Name == name {
			return u[i].Id
		}
	}

	return name
}

func (c *Config) Apply(cnf *Config) {
	if cnf == nil {
		return
	}

}

// type Topic string

type Message struct {
	// 标题
	Title string
	// 内容
	Content string
	// 通知的成员列表, 可不填
	UserList []string
	level    string
	// ? 指定发送到哪个topic
	// Topic string
}

func New(conf *Config) *QWRobot {
	robot := &QWRobot{
		config:  conf,
		limiter: conf.Limiter,
	}
	robot.infoFreqLimit, robot.infoFreqTTL = robot.parseFreqLimit(conf.InfoFreqLimit)
	robot.warnFreqLimit, robot.warnFreqTTL = robot.parseFreqLimit(conf.WarnFreqLimit)
	robot.errFreqLimit, robot.errFreqTTL = robot.parseFreqLimit(conf.ErrorFreqLimit)
	robot.fatalFreqLimit, robot.fatalFreqTTL = robot.parseFreqLimit(conf.FatalFreqLimit)
	// 清理可能已有的脏数据
	if robot.limiter != nil {
		robot.limiter.Del(context.Background(), robot.getCacheKey(LevelInfo))
		robot.limiter.Del(context.Background(), robot.getCacheKey(LevelWarn))
		robot.limiter.Del(context.Background(), robot.getCacheKey(LevelError))
	}

	if conf.Limiter != nil && conf.Webhook != "" {
		robot.ready = true
	}

	opt := ants.Options{
		ExpiryDuration:   time.Hour,
		PreAlloc:         false,
		MaxBlockingTasks: 1000,
		Nonblocking:      false,
		PanicHandler:     nil,
		Logger:           nil,
	}
	options := ants.WithOptions(opt)
	pool, _ := ants.NewPoolWithFunc(100, robot.send, options)
	robot.pool = pool

	return robot
}

type QWRobot struct {
	config        *Config
	infoFreqLimit int
	infoFreqTTL   int

	warnFreqLimit int
	warnFreqTTL   int

	errFreqLimit int
	errFreqTTL   int

	fatalFreqLimit int
	fatalFreqTTL   int

	msgCh   chan Message
	ready   bool
	pool    *ants.PoolWithFunc
	limiter Limiter
}

func (q *QWRobot) getCacheKey(level string) string {
	return q.config.Prefix + "_" + q.config.MessageType + "_" + level
}

func (q *QWRobot) Incr(key string, ttl int) int {
	var cnt int
	var err error

	cnt, err = q.limiter.Count(context.Background(), key, ttl)

	if err != nil {
		fmt.Printf("redis lua error key:%s ttl:%d err:%s\n", key, ttl, err.Error())
		return 0
	}

	return cnt
}

type qwMarkdownMessage struct {
	MsgType    string        `json:"msgtype"`
	Markdown   *MarkdownData `json:"markdown,omitempty"`
	MarkdownV2 *MarkdownData `json:"markdown_v2,omitempty"`
}

type MarkdownData struct {
	Content string `json:"content"`
}

type qwTextMessage struct {
	MsgType string `json:"msgtype"`
	Text    struct {
		Content       *string  `json:"content"`
		MentionedList []string `json:"mentioned_list"`
	} `json:"text"`
}

var userPattern = regexp.MustCompile(`@([a-zA-Z0-9]{3,32})`)

// ExtractUsernames 从输入字符串中提取 @ 开头的用户名
func ExtractUsernamesAndRemaining(input string) ([]string, string) {
	// 使用正则表达式匹配 @ 开头的单词
	matches := userPattern.FindAllStringSubmatch(input, -1)

	var usernames []string
	var remainingString string = input

	for _, match := range matches {
		// match[1] 是捕获的用户名部分
		usernames = append(usernames, match[1])
		// 将匹配到的部分从原字符串中移除
		remainingString = strings.Replace(remainingString, match[0], "", 1)
	}

	// 去除剩余字符串的多余空格
	remainingString = strings.TrimSpace(remainingString)

	return usernames, remainingString
}

func (q *QWRobot) getMessage(msg Message, level string) []byte {
	// 主动拼接环境信息
	// hostName := q.config.HostName
	// ip := q.config.Ip

	if len(msg.UserList) == 0 && strings.HasPrefix(msg.Title, "@") {
		usernames, remain := ExtractUsernamesAndRemaining(msg.Title)
		for i := range usernames {
			usernames[i] = q.config.Users.GetIdByName(usernames[i])
		}
		msg.UserList = usernames
		msg.Title = remain
	}

	var hostItem string
	var ipItem string
	var stageItem string

	if q.config.HostName != "" {
		hostItem = fmt.Sprintf("[host: %s] ", q.config.HostName)
	}
	if q.config.Ip != "" {
		ipItem = fmt.Sprintf("[ip: %s] ", q.config.Ip)
	}
	if q.config.Stage != "" {
		stageItem = fmt.Sprintf("[stage: %s] ", q.config.Stage)
	}

	msg.Content += fmt.Sprintf("\n%s%s%s", hostItem, ipItem, stageItem)

	var qwMsg interface{}
	var content = new(string)

	if len(msg.UserList) > 0 {
		*content = "[" + level + "] " + msg.Title + "\n" + msg.Content
		qwMsg = qwTextMessage{
			MsgType: "text",
			Text: struct {
				Content       *string  `json:"content"`
				MentionedList []string `json:"mentioned_list"`
			}{Content: content, MentionedList: msg.UserList},
		}
	} else {
		var color string
		if level == LevelInfo {
			color = "blue"
		} else if level == LevelWarn {
			color = "yellow"
		} else if level == LevelError {
			color = "red"
		} else {
			color = "purple"
		}
		containsImg := strings.Contains(msg.Content, "![")

		// https://developer.work.weixin.qq.com/document/path/91770
		// 判断 content 中是否包含 ![](), 企微支持图片
		if containsImg {
			*content = `[` + level + "] " + msg.Title + "]\n" + msg.Content
			qwMsg = qwMarkdownMessage{
				MsgType: "markdown_v2",
				MarkdownV2: &MarkdownData{
					Content: *content,
				},
			}
		} else {
			*content = `<font color="` + color + `">[` + level + "] " + msg.Title + "</font>\n>" + msg.Content
			qwMsg = qwMarkdownMessage{
				MsgType: "markdown",
				Markdown: &MarkdownData{
					Content: *content,
				},
			}
		}
	}
	if len(*content) > 2048 {
		*content = (*content)[0:2048]
	}

	data, _ := jsoniter.Marshal(qwMsg)

	return data
}

func (q *QWRobot) parseFreqLimit(freq string) (times int, ttl int) {
	items := strings.SplitN(freq, "/", 2)
	var limit = DefaultFreqLimit
	var unit = DefaultFreqUnit

	if len(items) == 2 {
		limit, _ = strconv.Atoi(items[0])
		if limit < 1 {
			limit = DefaultFreqLimit
		}
		unit = strings.ToUpper(items[1])
		if unit != UnitMinute && unit != UnitSecond && unit != UnitHour {
			unit = DefaultFreqUnit
		}
	}

	if unit == UnitMinute {
		ttl = 60
	} else if unit == UnitSecond {
		ttl = 1
	} else {
		ttl = 3600
	}

	return limit, ttl
}

func (q *QWRobot) checkLimit(key string, ttl, limit int) bool {
	curN := q.Incr(key, ttl)
	if curN <= limit {
		return true
	}

	return false
}

func (q *QWRobot) getLimitAndTTL(level string) (freq int, ttl int) {
	if level == LevelInfo {
		freq, ttl = q.infoFreqLimit, q.infoFreqTTL
	} else if level == LevelWarn {
		freq, ttl = q.warnFreqLimit, q.warnFreqTTL
	} else if level == LevelFatal {
		freq, ttl = q.fatalFreqLimit, q.fatalFreqTTL
	} else {
		freq, ttl = q.errFreqLimit, q.errFreqTTL
	}
	if freq == 0 {
		freq = 1
	}
	if ttl == 0 {
		ttl = 60
	}

	return
}

func (q *QWRobot) getWebhook(level string) (webhook string) {
	if level == LevelError {
		webhook = q.config.ErrorWebhook
	} else if level == LevelWarn {
		webhook = q.config.WarnWebhook
	} else if level == LevelFatal {
		webhook = q.config.FatalWebhook
	}

	if webhook == "" {
		webhook = q.config.Webhook
	}

	return webhook
}

func (q *QWRobot) send(in interface{}) {
	msg := in.(Message)
	if q.config.Limiter == nil {
		fmt.Println("qw robot limiter not defined ignored!!!")
		return
	}

	webhook := q.getWebhook(msg.level)
	if webhook == "" {
		fmt.Println("qw robot webhook undefined ignored!!!")
		return
	}

	defer func() {
		if err := recover(); err != nil {
			fmt.Println("qw robot send error", err)
		}
	}()
	// 1. 检查频率
	key := q.getCacheKey(msg.level)
	limit, ttl := q.getLimitAndTTL(msg.level)
	content := q.getMessage(msg, msg.level)

	if !q.checkLimit(key, ttl, limit) {
		fmt.Println("frequency limit exceeded level:"+msg.level+" type:"+q.config.MessageType, "content:"+string(content))
		return
	}

	err := q.sendToQW(webhook, content)
	if err != nil {
		fmt.Println("webhook=" + webhook + " content:" + string(content) + " err:" + err.Error())
	}
}

func (q *QWRobot) sendToQW(webhook string, data []byte) error {
	response, err := req.New(context.Background()).Header(map[string]string{"Content-Type": "application/json"}).Body(data).Post(webhook)
	if err != nil {
		return err
	}
	if response != nil && response.IsSuccess() {
		return nil
	} else {
		if response != nil {
			return fmt.Errorf("code:%d", response.StatusCode)
		}
		return errors.New("request error")
	}
}

func (q *QWRobot) Info(msg Message) {
	if q.ready {
		msg.level = LevelInfo
		_ = q.pool.Invoke(msg)
	}
}

func (q *QWRobot) Warn(msg Message) {
	if q.ready {
		msg.level = LevelWarn
		_ = q.pool.Invoke(msg)
	}
}

func (q *QWRobot) Error(msg Message) {
	if q.ready {
		msg.level = LevelError
		_ = q.pool.Invoke(msg)
	}
}

func (q *QWRobot) Fatal(msg Message) {
	if q.ready {
		msg.level = LevelFatal
		_ = q.pool.Invoke(msg)
	}
}

var qwRobotInst *QWRobot
var qwRobotOnce sync.Once

func Get() *QWRobot {
	return qwRobotInst
}

func Init(conf *Config) error {
	if qwRobotInst == nil {
		qwRobotOnce.Do(func() {
			qwRobotInst = New(conf)
		})
	}

	return nil
}
