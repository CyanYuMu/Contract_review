package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"contract_review/app/internal/global"

	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

// ClauseAgent 条款拆分 Agent
// 负责将合同文本智能拆分为有意义的条款单元，而非暴力按字数分块
type ClauseAgent struct {
	llmGenerate func(ctx context.Context, messages []*schema.Message) (*schema.Message, error)
	tools       []Tool
	reactConfig ReactConfig
}

// NewClauseAgent 创建条款拆分 Agent
func NewClauseAgent(
	llmGenerate func(ctx context.Context, messages []*schema.Message) (*schema.Message, error),
	tools []Tool,
) *ClauseAgent {
	return &ClauseAgent{
		llmGenerate: llmGenerate,
		tools:       tools,
		reactConfig: ReactConfig{
			MaxIterations:     3,
			MinIterations:     1,
			ObservationWindow: 3,
		},
	}
}

func (ca *ClauseAgent) Name() string { return "ClauseAgent" }

func (ca *ClauseAgent) AvailableTools() []Tool { return ca.tools }

// Execute 执行条款拆分
func (ca *ClauseAgent) Execute(ctx context.Context, input AgentInput) (AgentOutput, error) {
	contractText, _ := input.Context["contract_text"].(string)
	if contractText == "" {
		return AgentOutput{}, fmt.Errorf("合同文本不能为空")
	}

	global.Log.Info("ClauseAgent 开始条款拆分",
		zap.Int("textLength", len([]rune(contractText))))

	clauses := ca.structuralSplit(contractText)

	if len(clauses) <= 1 {
		clauses = ca.llmAssistedSplit(ctx, contractText)
	}

	clauses = ca.classifyClauses(ctx, clauses)

	global.Log.Info("ClauseAgent 条款拆分完成",
		zap.Int("clauseCount", len(clauses)))

	return AgentOutput{
		Result: clauses,
		Thinking: []ThinkStep{
			{
				Iteration: 1,
				Thought:   "对合同进行结构化条款拆分",
				Action:    "structural_split",
				Observation: fmt.Sprintf("拆分为 %d 个条款单元", len(clauses)),
			},
		},
	}, nil
}

// structuralSplit 基于文档结构的条款拆分
func (ca *ClauseAgent) structuralSplit(text string) []Clause {
	patterns := []struct {
		pattern *regexp.Regexp
		level   string
	}{
		{regexp.MustCompile(`(?m)^第[一二三四五六七八九十百千\d]+条[：:\s]`), "article"},
		{regexp.MustCompile(`(?m)^第[一二三四五六七八九十百千\d]+章[：:\s]`), "chapter"},
		{regexp.MustCompile(`(?m)^[一二三四五六七八九十]+[、.][^\n]+`), "section"},
		{regexp.MustCompile(`(?m)^\d+[.、]\d*[.、]?\s*[^\n]+`), "subsection"},
	}

	type splitPoint struct {
		index int
		title string
		level string
	}

	var points []splitPoint
	for _, p := range patterns {
		matches := p.pattern.FindAllStringIndex(text, -1)
		for _, m := range matches {
			lineEnd := strings.Index(text[m[0]:], "\n")
			title := text[m[0]:]
			if lineEnd != -1 {
				title = text[m[0] : m[0]+lineEnd]
			}
			points = append(points, splitPoint{
				index: m[0],
				title: strings.TrimSpace(title),
				level: p.level,
			})
		}
	}

	if len(points) < 2 {
		return []Clause{{
			ID:       "full-text",
			Title:    "合同全文",
			Content:  text,
			Category: "全文",
		}}
	}

	// 按位置排序并去重
	seen := make(map[int]bool)
	var uniquePoints []splitPoint
	for _, p := range points {
		if !seen[p.index] {
			seen[p.index] = true
			uniquePoints = append(uniquePoints, p)
		}
	}
	points = uniquePoints

	// 按 index 排序（插入排序）
	for i := 1; i < len(points); i++ {
		key := points[i]
		j := i - 1
		for j >= 0 && points[j].index > key.index {
			points[j+1] = points[j]
			j--
		}
		points[j+1] = key
	}

	var clauses []Clause
	for i, p := range points {
		var end int
		if i < len(points)-1 {
			end = points[i+1].index
		} else {
			end = len(text)
		}

		content := strings.TrimSpace(text[p.index:end])
		if content == "" {
			continue
		}

		clauseID := fmt.Sprintf("clause-%d", i+1)
		clauses = append(clauses, Clause{
			ID:      clauseID,
			Title:   p.title,
			Content: content,
		})
	}

	// 处理条款前的序言部分
	if len(points) > 0 && points[0].index > 0 {
		preamble := strings.TrimSpace(text[:points[0].index])
		if len([]rune(preamble)) > 50 {
			clauses = append([]Clause{{
				ID:       "preamble",
				Title:    "合同序言/首部",
				Content:  preamble,
				Category: "主体条款",
			}}, clauses...)
		}
	}

	return clauses
}


// llmAssistedSplit LLM 辅助拆分（当结构化拆分失败时）
func (ca *ClauseAgent) llmAssistedSplit(ctx context.Context, text string) []Clause {
	if len([]rune(text)) > 8000 {
		text = string([]rune(text)[:8000])
	}

	result, err := RunSimple(ctx, clauseSplitSystemPrompt, fmt.Sprintf(`请对以下合同文本进行条款拆分：

%s

请以JSON数组格式输出，每个元素包含:
- "id": 条款编号如"clause-1"
- "title": 条款标题
- "content": 条款内容（完整原文）

输出格式: [{"id":"clause-1","title":"...","content":"..."}, ...]`, text), ca.llmGenerate)

	if err != nil {
		global.Log.Error("LLM 辅助拆分失败", zap.Error(err))
		return ca.fallbackSplit(text)
	}

	var clauses []Clause
	jsonStr := extractJSONArray(result)
	if jsonStr != "" {
		if err := json.Unmarshal([]byte(jsonStr), &clauses); err == nil && len(clauses) > 0 {
			return clauses
		}
	}

	return ca.fallbackSplit(text)
}

// fallbackSplit 兜底拆分 — 按段落分割
func (ca *ClauseAgent) fallbackSplit(text string) []Clause {
	paragraphs := regexp.MustCompile(`\n\s*\n`).Split(text, -1)

	var clauses []Clause
	var currentContent strings.Builder
	clauseIdx := 1

	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		currentContent.WriteString(p)
		currentContent.WriteString("\n")

		if currentContent.Len() >= 2000 || clauseIdx == 1 {
			clauses = append(clauses, Clause{
				ID:      fmt.Sprintf("block-%d", clauseIdx),
				Title:   fmt.Sprintf("段落%d", clauseIdx),
				Content: currentContent.String(),
			})
			currentContent.Reset()
			clauseIdx++
		}
	}

	if currentContent.Len() > 0 {
		clauses = append(clauses, Clause{
			ID:      fmt.Sprintf("block-%d", clauseIdx),
			Title:   fmt.Sprintf("段落%d", clauseIdx),
			Content: currentContent.String(),
		})
	}

	return clauses
}

// classifyClauses 为条款分类标注
func (ca *ClauseAgent) classifyClauses(ctx context.Context, clauses []Clause) []Clause {
	categoryKeywords := map[string][]string{
		"主体条款":   {"甲方", "乙方", "合同主体", "签约方", "委托方", "受托方"},
		"权利义务":   {"应当", "有权", "义务", "负责", "承担", "交付", "验收"},
		"违约责任":   {"违约", "赔偿", "损失", "罚款", "违反"},
		"争议解决":   {"争议", "仲裁", "诉讼", "管辖", "调解"},
		"保密条款":   {"保密", "商业秘密", "不得泄露", "机密"},
		"知识产权":   {"知识产权", "著作权", "专利", "商标", "版权"},
		"合同生效与终止": {"生效", "终止", "解除", "期限", "有效期"},
		"付款条款":   {"付款", "支付", "价款", "费用", "结算", "金额"},
		"附则":     {"附则", "附件", "补充", "其他约定"},
	}

	for i := range clauses {
		if clauses[i].Category != "" {
			continue
		}

		contentLower := strings.ToLower(clauses[i].Content + clauses[i].Title)
		bestCategory := "其他"
		bestScore := 0

		for category, keywords := range categoryKeywords {
			score := 0
			for _, kw := range keywords {
				if strings.Contains(contentLower, kw) {
					score++
				}
			}
			if score > bestScore {
				bestScore = score
				bestCategory = category
			}
		}

		clauses[i].Category = bestCategory
	}

	return clauses
}

func extractJSONArray(text string) string {
	start := strings.Index(text, "[")
	if start == -1 {
		return ""
	}
	depth := 0
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return text[start : i+1]
			}
		}
	}
	return ""
}

const clauseSplitSystemPrompt = `你是一个合同结构化分析专家。你的任务是将合同文本按照条款结构进行智能拆分。

## 拆分规则
1. 按合同条款的自然边界拆分，不要在条款中间截断
2. 保留每个条款的完整性，包括子条款
3. 识别条款标题（如"第X条"、"X、"等）
4. 序言/首部作为独立段落处理
5. 附则和附件信息作为独立段落处理`
