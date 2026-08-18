package agent

import (
	"context"
	"time"
)

// Agent 统一接口 — 所有 Worker Agent 实现此接口
type Agent interface {
	Name() string
	Execute(ctx context.Context, input AgentInput) (AgentOutput, error)
	AvailableTools() []Tool
}

// Tool 工具统一接口 — 对应 Function Calling
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]interface{}
	Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error)
}

// ToolResult 工具执行结果
type ToolResult struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
	Error   string      `json:"error,omitempty"`
}

// AgentInput Agent 执行输入
type AgentInput struct {
	Task        string                 `json:"task"`
	Context     map[string]interface{} `json:"context"`
	Constraints AgentConstraints       `json:"constraints"`
}

// AgentConstraints Agent 约束条件
type AgentConstraints struct {
	MaxIterations int           `json:"max_iterations"`
	TokenBudget   int           `json:"token_budget"`
	Timeout       time.Duration `json:"timeout"`
}

// AgentOutput Agent 执行输出
type AgentOutput struct {
	Result     interface{}   `json:"result"`
	Thinking   []ThinkStep   `json:"thinking"`
	TokensUsed int           `json:"tokens_used"`
	Duration   time.Duration `json:"duration"`
}

// ThinkStep ReAct 循环中的一步思考记录（可观测性）
type ThinkStep struct {
	Iteration   int       `json:"iteration"`
	Thought     string    `json:"thought"`
	Action      string    `json:"action"`
	ActionInput string    `json:"action_input"`
	Observation string    `json:"observation"`
	Timestamp   time.Time `json:"timestamp"`
}

// Clause 合同条款单元
type Clause struct {
	ID           string            `json:"id"`
	Title        string            `json:"title"`
	Content      string            `json:"content"`
	Category     string            `json:"category"`
	ParentID     string            `json:"parent_id,omitempty"`
	Dependencies []string          `json:"dependencies,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// RiskFinding 经过验证的风险发现
type RiskFinding struct {
	FindingID           string       `json:"finding_id,omitempty"`
	ClauseID            string       `json:"clause_id"`
	ClauseIDs           []string     `json:"clause_ids,omitempty"` // 跨条款合并后命中的所有条款 ID（含 ClauseID）
	CandidateIDs        []string     `json:"candidate_ids,omitempty"`
	RiskType            string       `json:"risk_type"`
	RiskLevel           string       `json:"risk_level"`
	RiskDescription     string       `json:"risk_description"`
	OriginalText        string       `json:"original_text"`
	LegalBasis          []LegalBasis `json:"legal_basis"`
	Verified            bool         `json:"verified"`
	RequiresHumanReview bool         `json:"requires_human_review,omitempty"`
	Confidence          float64      `json:"confidence"`
	SuggestedText       string       `json:"suggested_text,omitempty"`
	SuggestionReason    string       `json:"suggestion_reason,omitempty"`
	Priority            string       `json:"priority,omitempty"`
}

// LegalBasis 法律依据（来自 RAG 检索）
type LegalBasis struct {
	Source    string  `json:"source"`
	Article   string  `json:"article"`
	Content   string  `json:"content"`
	Relevance float64 `json:"relevance"`
}

// Suggestion 修改建议
type Suggestion struct {
	RiskFindingID  string `json:"risk_finding_id"`
	OriginalText   string `json:"original_text"`
	SuggestedText  string `json:"suggested_text"`
	Reason         string `json:"reason"`
	LegalReference string `json:"legal_reference"`
	Impact         string `json:"impact"`
	Priority       string `json:"priority"` // 必须修改/建议修改/可选修改
}

// ReviewReport 审阅报告
type ReviewReport struct {
	TaskID          uint64        `json:"task_id"`
	Clauses         []Clause      `json:"clauses"`
	Findings        []RiskFinding `json:"findings"`
	Suggestions     []Suggestion  `json:"suggestions"`
	Summary         string        `json:"summary"`
	OverallRisk     string        `json:"overall_risk"`
	WordCount       int           `json:"word_count"`
	ReflectionCount int           `json:"reflection_count"`
	QualityScore    float64       `json:"quality_score"`
	TokensUsed      int           `json:"tokens_used"`
	Duration        time.Duration `json:"duration"`
}

// OrchestratorConfig 编排器配置
type OrchestratorConfig struct {
	MaxIterations        int           `json:"max_iterations"`
	MinIterations        int           `json:"min_iterations"`
	ReflectionEnabled    bool          `json:"reflection_enabled"`
	ReflectionThreshold  float64       `json:"reflection_threshold"`
	MaxReflectionRetries int           `json:"max_reflection_retries"`
	TokenBudget          int           `json:"token_budget"`
	Timeout              time.Duration `json:"timeout"`
	MaxConcurrentAgents  int           `json:"max_concurrent_agents"`
	RiskCandidateTopK    int           `json:"risk_candidate_top_k"`
	RiskReviewBatchSize  int           `json:"risk_review_batch_size"`
}

// DefaultOrchestratorConfig 默认配置
func DefaultOrchestratorConfig() OrchestratorConfig {
	return OrchestratorConfig{
		MaxIterations:        10,
		MinIterations:        1,
		ReflectionEnabled:    true,
		ReflectionThreshold:  0.7,
		MaxReflectionRetries: 0,
		TokenBudget:          100000,
		Timeout:              10 * time.Minute,
		MaxConcurrentAgents:  5,
		RiskCandidateTopK:    8,
		RiskReviewBatchSize:  3,
	}
}

// ReactConfig ReAct 循环配置
type ReactConfig struct {
	MaxIterations     int `json:"max_iterations"`
	MinIterations     int `json:"min_iterations"`
	ObservationWindow int `json:"observation_window"`
}

// DefaultReactConfig 默认 ReAct 配置
func DefaultReactConfig() ReactConfig {
	return ReactConfig{
		MaxIterations:     5,
		MinIterations:     1,
		ObservationWindow: 3,
	}
}

// ReflectionConfig Reflection 质量评估配置
type ReflectionConfig struct {
	Enabled             bool     `json:"enabled"`
	MaxRetries          int      `json:"max_retries"`
	ConfidenceThreshold float64  `json:"confidence_threshold"`
	Criteria            []string `json:"criteria"`
}

// DefaultReflectionConfig 默认 Reflection 配置
func DefaultReflectionConfig() ReflectionConfig {
	return ReflectionConfig{
		Enabled:             true,
		MaxRetries:          2,
		ConfidenceThreshold: 0.7,
		Criteria: []string{
			"completeness",
			"legal_accuracy",
			"risk_coverage",
			"suggestion_quality",
			"consistency",
		},
	}
}

// QualityEvaluation 质量评估结果
type QualityEvaluation struct {
	OverallScore   float64            `json:"overall_score"`
	CriteriaScores map[string]float64 `json:"criteria_scores"`
	CriticalGaps   []string           `json:"critical_gaps"`
	Feedback       string             `json:"feedback"`
	ShouldRetry    bool               `json:"should_retry"`
}

// RetrievalResult RAG 检索结果
type RetrievalResult struct {
	ID       string            `json:"id"`
	Content  string            `json:"content"`
	Score    float64           `json:"score"`
	Source   string            `json:"source"`
	Metadata map[string]string `json:"metadata"`
}

// ContractMeta 合同元信息
type ContractMeta struct {
	PartyA       string `json:"party_a"`
	PartyB       string `json:"party_b"`
	ContractType string `json:"contract_type"`
	Stance       string `json:"stance"`
	Intensity    string `json:"intensity"`
	Amount       string `json:"amount"`
	// Overview 合同整体结构摘要（全部条款标题+分类），条款拆分后由编排器回填，
	// 注入批量审阅提示词，帮助 LLM 结合全合同上下文判断跨条款风险。
	Overview string `json:"overview,omitempty"`
}
