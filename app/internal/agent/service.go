package agent

import (
	"context"
	"contract_review/app/internal/global"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/arkbot"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

// ContractAgent 合同解析智能体
type ContractAgent struct {
	llm            *arkbot.ChatModel
	promptTemplate string
}

// NewContractAgent 创建合同解析智能体
func NewContractAgent(ctx context.Context) (*ContractAgent, error) {
	// 初始化LLM
	llm, err := arkbot.NewChatModel(ctx,
		&arkbot.Config{
			Model:  global.Config.LLMConfig.Model,
			APIKey: global.Config.LLMConfig.APIKey,
		})
	if err != nil {
		global.Log.Error("init arkbot model failed", zap.Error(err))
		return nil, err
	}

	// 加载提示词模板
	promptPath := getPromptPath()
	promptTemplate, err := LoadPrompt(promptPath)
	if err != nil {
		global.Log.Error("load prompt template failed", zap.Error(err))
		return nil, err
	}

	return &ContractAgent{
		llm:            llm,
		promptTemplate: promptTemplate,
	}, nil
}

// getPromptPath 获取提示词文件路径
func getPromptPath() string {
	// 获取当前文件所在目录
	_, filename, _, _ := runtime.Caller(0)
	dir := filepath.Dir(filename)
	return filepath.Join(dir, "prompts", "contract_extract.prompt")
}

// LoadPrompt 从文件加载提示词
func LoadPrompt(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// CreateContractExtractTemplate 创建合同信息提取的提示词模板
func CreateContractExtractTemplate(promptText string) *prompt.DefaultChatTemplate {
	template := prompt.FromMessages(schema.GoTemplate,
		&schema.Message{
			Role:    schema.User,
			Content: promptText,
		},
	)
	return template
}

// ParseContract 解析合同文本，提取甲方、乙方、金额等信息
func (ca *ContractAgent) ParseContract(ctx context.Context, contractText string) (string, error) {
	// 创建提示词模板
	template := CreateContractExtractTemplate(ca.promptTemplate)

	// 格式化提示词，将合同文本填入模板
	messages, err := template.Format(ctx, map[string]any{
		"input": contractText,
	})
	if err != nil {
		global.Log.Error("format prompt template failed", zap.Error(err))
		return "", err
	}

	// 调用LLM
	response, err := ca.llm.Generate(ctx, messages)
	if err != nil {
		global.Log.Error("llm generate failed", zap.Error(err))
		return "", err
	}

	// 提取响应内容
	result := response.Content
	global.Log.Info("contract parse result", zap.String("result", result))

	return result, nil
}

// 全局智能体实例
var contractAgent *ContractAgent

// InitContractAgent 初始化全局合同解析智能体
func InitContractAgent(ctx context.Context) error {
	agent, err := NewContractAgent(ctx)
	if err != nil {
		return err
	}
	contractAgent = agent
	return nil
}

// GetContractAgent 获取全局合同解析智能体
func GetContractAgent() *ContractAgent {
	return contractAgent
}

// LLMContractParse 对外提供的合同解析函数
func LLMContractParse(ctx context.Context, content string) (string, error) {
	agent := GetContractAgent()
	if agent == nil {
		// 如果未初始化，则初始化
		if err := InitContractAgent(ctx); err != nil {
			return "", err
		}
		agent = GetContractAgent()
	}
	return agent.ParseContract(ctx, content)
}

// ExtractJSONFromResponse 从LLM响应中提取JSON
func ExtractJSONFromResponse(response string) string {
	// 尝试找到JSON的开始和结束位置
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")

	if start != -1 && end != -1 && end > start {
		return response[start : end+1]
	}

	return response
}
