package su_id

import (
	"fmt"
	"hash/fnv"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/yitter/idgenerator-go/idgen"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/snowflake"
)

type Generator interface {
	NextId() int64
	GetWorkId() uint16
}

// SnowflakeM1 漂移算法实现, 遇到时间回拨会进行等待
type SnowflakeM1 struct {
	generator *idgen.DefaultIdGenerator
	workId    uint16
}

// 全局变量用于跟踪已使用的workId
var (
	usedWorkIds = make(map[uint16]bool)
	workIdMutex sync.RWMutex
)

type GenOptions struct {
	BitLength      int
	BaseTimeMs     int64
	WorkerIdBitLen int
}

func NewSnowflake(workId uint16, options *GenOptions) Generator {
	// 检查workId是否已被使用
	workIdMutex.Lock()
	if usedWorkIds[workId] {
		workIdMutex.Unlock()
		panic(fmt.Sprintf("workId %d has already been used, please use a different workId", workId))
	}
	usedWorkIds[workId] = true
	workIdMutex.Unlock()

	opt := idgen.NewIdGeneratorOptions(workId)
	var setBaseTimeFlag bool
	if options != nil {
		if options.BaseTimeMs > 0 {
			opt.BaseTime = options.BaseTimeMs
			setBaseTimeFlag = true
		}
		if options.BitLength > 0 {
			opt.SeqBitLength = byte(options.BitLength)
		}
		if options.WorkerIdBitLen > 0 {
			opt.WorkerIdBitLength = byte(options.WorkerIdBitLen)
		}
	}

	if !setBaseTimeFlag {
		opt.BaseTime = time.Date(2023, 3, 8, 12, 13, 14, 15, time.UTC).UnixNano() / 1e6
	}

	generator := idgen.NewDefaultIdGenerator(opt)

	return &SnowflakeM1{
		generator: generator,
		workId:    workId,
	}
}

func (s *SnowflakeM1) GetWorkId() uint16 {
	return s.workId
}

func (s *SnowflakeM1) NextId() int64 {
	return s.generator.SnowWorker.NextId()
}

// IsWorkIdUsed 检查workId是否已被使用
func IsWorkIdUsed(workId uint16) bool {
	workIdMutex.RLock()
	defer workIdMutex.RUnlock()
	return usedWorkIds[workId]
}

// GetUsedWorkIds 获取所有已使用的workId列表
func GetUsedWorkIds() []uint16 {
	workIdMutex.RLock()
	defer workIdMutex.RUnlock()

	ids := make([]uint16, 0, len(usedWorkIds))
	for id := range usedWorkIds {
		ids = append(ids, id)
	}
	return ids
}

// ClearUsedWorkIds 清除所有已使用的workId记录（仅用于测试）
func ClearUsedWorkIds() {
	workIdMutex.Lock()
	defer workIdMutex.Unlock()
	usedWorkIds = make(map[uint16]bool)
}

// ReleaseWorkId 释放指定的workId（允许重新使用）
func ReleaseWorkId(workId uint16) {
	workIdMutex.Lock()
	defer workIdMutex.Unlock()
	delete(usedWorkIds, workId)
}

var nodeMap = make(map[int64]*snowflake.Node, 16)
var nodeMapLock = sync.Mutex{}

func getSnowflakeNode(node int64) (n *snowflake.Node, err error) {
	if _, ok := nodeMap[node]; !ok {
		nodeMapLock.Lock()
		defer nodeMapLock.Unlock()
		// 双重锁判断
		if _, ok = nodeMap[node]; !ok {
			nodeMap[node], err = snowflake.NewNode(node)
		}
	}

	return nodeMap[node], err
}

// GetStringWithSnowflake
// @description获取string类型的雪花id
func GetStringWithSnowflake(node int64) (string, error) {
	newNode, err := getSnowflakeNode(node)
	if err != nil {
		return "", err
	}
	s := newNode.Generate().String()

	return s, nil
}

// GetIntWithSnowflake
// @description 获取int64类型的雪花id
func GetIntWithSnowflake(node int64) (int64, error) {
	newNode, err := getSnowflakeNode(node)
	if err != nil {
		return 0, err
	}
	s := newNode.Generate().Int64()

	return s, nil
}

// InitIdGen
// options.WorkerIdBitLength = 6  // 默认值6，限定 WorkerId 最大值为2^6-1，即默认最多支持64个节点。
// options.SeqBitLength = 6; // 默认值6，限制每毫秒生成的ID个数。若生成速度超过5万个/秒，建议加大 SeqBitLength 到 10。
// options.BaseTime = Your_Base_Time // 如果要兼容老系统的雪花算法，此处应设置为老系统的BaseTime。
type GenParam struct {
	BitLength      int
	BaseTimeMs     int64
	WorkerIdBitLen int
}

func InitIdGen(workId uint16, param ...GenParam) {
	// NewIdGeneratorOptions
	opt := idgen.NewIdGeneratorOptions(workId)
	opt.WorkerIdBitLength = 6
	opt.SeqBitLength = 6
	setBaseTimeFlag := false
	if len(param) > 0 {
		if param[0].BitLength > 0 {
			opt.SeqBitLength = byte(param[0].BitLength)
		}
		if param[0].WorkerIdBitLen > 0 {
			opt.WorkerIdBitLength = byte(param[0].WorkerIdBitLen)
		}

		if param[0].BaseTimeMs > 0 {
			opt.BaseTime = param[0].BaseTimeMs
			setBaseTimeFlag = true
		}
	}

	if !setBaseTimeFlag {
		opt.BaseTime = time.Date(2023, 3, 8, 12, 13, 14, 15, time.UTC).UnixNano() / 1e6
	}

	idgen.SetIdGenerator(opt)
}

// GetIdByIdGen 之前需要调用 InitIdGen() 进行初始化
func GetIdByIdGen() int64 {
	return idgen.NextId()
}

// DefaultGetWorkId 优先从环境变量 HOSTNAME 读取（K8s Pod 等场景通常已注入），否则使用 os.Hostname()，
// 再将主机名字符串哈希为 uint16 作为 worker id。
func DefaultGetWorkId() uint16 {
	hostname := strings.TrimSpace(os.Getenv("HOSTNAME"))
	if hostname == "" {
		var err error
		hostname, err = os.Hostname()
		if err != nil || hostname == "" {
			hostname = "unknown"
		}
	}
	return hostnameToWorkID(hostname)
}

func hostnameToWorkID(s string) uint16 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return uint16(h.Sum32())
}
