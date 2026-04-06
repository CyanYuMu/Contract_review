package google

import (
	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	tasks "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"context"
	"fmt"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/googleapis/gax-go/v2"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/encrypt"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/types/known/durationpb"
	"net/http"
	"time"
)

type CloudTask struct {
	// 平台参数
	ProjectId string
	Region    string
	inst      *cloudtasks.Client
}

type CloudTaskConfig struct {
	CredentialsJson string
	Region          string
	// 平台参数
	ProjectId string
}

func NewCloudTask(ctx context.Context, cnf CloudTaskConfig, opts ...option.ClientOption) (cli *CloudTask, err error) {
	cli = &CloudTask{}
	byteData, err := encrypt.Base64Decode([]byte(cnf.CredentialsJson))
	if err != nil {
		return nil, err
	}
	opts = append(opts, option.WithCredentialsJSON(byteData))
	client, err := cloudtasks.NewClient(ctx, opts...)
	if err != nil {
		return
	}
	cli.inst = client
	cli.ProjectId = cnf.ProjectId
	cli.Region = cnf.Region

	return cli, nil
}

type HttpRequest struct {
	// ? default post
	Method string
	// * 发送的地址
	Url string
	// ? 头信息
	Headers map[string]string
	// * 发送的数据
	Data []byte
}

type AppEnginRequest struct {
	// ? default post
	Method string
	// ? 头信息
	Headers map[string]string
	// * 发送的数据
	Data []byte
}

type Backoff struct {
	// Initial is the initial value of the retry period, defaults to 1 second.
	Initial time.Duration
	// Max is the maximum value of the retry period, defaults to 30 seconds.
	Max time.Duration
	// Multiplier is the factor by which the retry period increases.
	// It should be greater than 1 and defaults to 2.
	Multiplier float64
}

type Task struct {
	// * task name
	ID string
	// * 队列名
	Queue string
	// ? 延迟执行时间
	DelaySec int
	// ? 过期时间
	ExpireSec int
	// 常规 http 模式, 与 AppEnginRequest 二选一, 优先级更高
	HttpRequest *HttpRequest
	// google appengin 模式
	AppEnginRequest *AppEnginRequest
	Backoff         *Backoff
}

func (c *CloudTask) GetTaskName(queue, taskId string) string {
	// "projects/<PROJECT_ID>/locations/<LOCATION_ID>/queues/<QUEUE_ID>/tasks/<TASK_ID>".
	return fmt.Sprintf("projects/%s/locations/%s/queues/%s/tasks/%s", c.ProjectId, c.Region, queue, taskId)
}

func (c *CloudTask) getParent(projectId, region, queue string) string {
	return fmt.Sprintf("projects/%s/locations/%s/queues/%s", projectId, region, queue)
}

func headerMerge(h1, h2 map[string]string) map[string]string {
	if h2 == nil {
		return h1
	}
	for k, v := range h2 {
		h1[k] = v
	}

	return h1
}

func (c *CloudTask) GetInst() *cloudtasks.Client {
	return c.inst
}

/*DeleteTask
* @Description: 删除任务
* @param ctx
* @param name
* @return err
 */
func (c *CloudTask) DeleteTask(ctx context.Context, name string) (err error) {
	err = c.inst.DeleteTask(ctx, &tasks.DeleteTaskRequest{
		Name: name,
	})

	return
}

/*AddTask
* @Description: 添加任务
* @param ctx
* @param t
* @return tsk
* @return err
 */
func (c *CloudTask) AddTask(ctx context.Context, t Task) (tsk *tasks.Task, err error) {
	// 创建一个任务
	parent := c.getParent(c.ProjectId, c.Region, t.Queue)
	header := map[string]string{
		"Content-Type": "application/json",
	}

	header = headerMerge(header, t.HttpRequest.Headers)

	task := &tasks.Task{
		Name: c.GetTaskName(t.Queue, t.ID),
		View: tasks.Task_BASIC,
	}

	if t.HttpRequest != nil {
		if t.HttpRequest.Method == "" {
			t.HttpRequest.Method = "POST"
		}
		method := tasks.HttpMethod_value[t.HttpRequest.Method]
		task.MessageType = &tasks.Task_HttpRequest{
			HttpRequest: &tasks.HttpRequest{
				HttpMethod: tasks.HttpMethod(method),
				Url:        t.HttpRequest.Url,
				Headers:    header,
				Body:       t.HttpRequest.Data,
			},
		}
	} else if t.AppEnginRequest != nil {
		if t.AppEnginRequest.Method == "" {
			t.AppEnginRequest.Method = "POST"
		}
		method := tasks.HttpMethod_value[t.HttpRequest.Method]

		task.MessageType = &tasks.Task_AppEngineHttpRequest{
			AppEngineHttpRequest: &tasks.AppEngineHttpRequest{
				HttpMethod: tasks.HttpMethod(method),
				Headers:    header,
				Body:       t.AppEnginRequest.Data,
			},
		}
	}

	if t.DelaySec > 0 {
		task.ScheduleTime = &timestamp.Timestamp{
			Seconds: time.Now().Add(time.Second * time.Duration(t.DelaySec)).Unix(),
		}
	}

	if t.ExpireSec > 0 {
		task.DispatchDeadline = &durationpb.Duration{
			Seconds: time.Now().Add(time.Second * time.Duration(t.ExpireSec)).Unix(),
		}
	}

	backoff := gax.Backoff{
		Initial:    time.Second,
		Max:        time.Minute,
		Multiplier: 1.30,
	}

	if t.Backoff != nil {
		if t.Backoff.Initial > 0 {
			backoff.Initial = t.Backoff.Initial
		}
		if t.Backoff.Max > 0 {
			backoff.Max = t.Backoff.Max
		}
		if t.Backoff.Multiplier > 0 {
			backoff.Multiplier = t.Backoff.Multiplier
		}
	}

	var opts = []gax.CallOption{
		gax.WithRetry(func() gax.Retryer {
			return gax.OnHTTPCodes(backoff,
				// 针对超时, 服务不可用
				http.StatusGatewayTimeout,
				http.StatusServiceUnavailable,
				http.StatusInternalServerError,
			)
		}),
	}

	// 将任务添加到队列中
	resp, err := c.inst.CreateTask(ctx, &tasks.CreateTaskRequest{
		Parent:       parent,
		Task:         task,
		ResponseView: tasks.Task_BASIC,
	}, opts...)

	return resp, err
}

type ReceiveParam struct {
	Queue   string
	TaskId  string
	Handler func(ctx context.Context, data []byte, header map[string]string) error
}

func (c *CloudTask) DoGetTaskAndRun(ctx context.Context, param ReceiveParam) error {
	taskName := c.GetTaskName(param.Queue, param.TaskId)
	task, err := c.inst.RunTask(ctx, &tasks.RunTaskRequest{
		Name:         taskName,
		ResponseView: tasks.Task_FULL,
	})

	if err != nil {
		return err
	}

	taskData := task.GetHttpRequest().Body
	header := task.GetHttpRequest().Headers
	if len(taskData) == 0 {
		taskData = task.GetAppEngineHttpRequest().Body
		header = task.GetAppEngineHttpRequest().Headers
	}
	err = param.Handler(ctx, taskData, header)
	if err != nil {
		return err
	}

	return nil
}

func (c *CloudTask) GetTaskAndRun(ctx context.Context, param ReceiveParam) error {
	taskName := c.GetTaskName(param.Queue, param.TaskId)
	task, err := c.inst.GetTask(ctx, &tasks.GetTaskRequest{
		Name:         taskName,
		ResponseView: tasks.Task_FULL,
	})

	if err != nil {
		return err
	}

	taskData := task.GetHttpRequest().Body
	header := task.GetHttpRequest().Headers
	if len(taskData) == 0 {
		taskData = task.GetAppEngineHttpRequest().Body
		header = task.GetAppEngineHttpRequest().Headers
	}
	err = param.Handler(ctx, taskData, header)
	if err != nil {
		return err
	}

	return nil
}
