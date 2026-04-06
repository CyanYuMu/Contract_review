# 合同审阅 Agent 架构优化方案

> 基于 [AI Agent 架构：从单体到企业级多智能体](https://www.waylandz.com/ai-agent-book/) 设计范式  
> 版本: v1.0 | 日期: 2026-04-06  
> 状态: Phase 2-4 核心代码已实现

---

## 一、现状分析与问题诊断

### 1.1 现有架构

当前合同审阅流程是一个**纯 Prompt 驱动的单次调用模式（L0-L1）**：

```
用户上传合同 → 提取文本 → 按4000字分块 → 并发调LLM → 正则解析输出 → 存库返回
```

**核心问题：**


| 问题           | 现状                        | 影响              |
| ------------ | ------------------------- | --------------- |
| 无知识检索        | 风险点凭 LLM 自有知识"编造"，无法律法规依据 | 审阅结果不可信、不可追溯    |
| 无 Agent 自主性  | 每个分块独立调一次 LLM，无思考-行动-观察循环 | 无法根据上下文动态调整审阅策略 |
| 无质量反思        | LLM 输出直接使用，无评估与迭代         | 输出质量不稳定，存在遗漏和误判 |
| 分块割裂         | 按固定字数暴力切割，忽视合同条款边界        | 跨块条款被截断，上下文丢失   |
| Prompt 未接 DB | 代码中有三级 Prompt 模型但审阅未接入    | 无法按机构/个人定制审阅规则  |
| 无工具调用        | LLM 不具备主动检索审阅规范的能力        | 风险点缺乏法规条例支撑     |


### 1.2 目标架构等级

从当前的 **L0-L1（Chatbot/Tool Agent）** 升级到 **L3-L4（Planning Agent + Multi-Agent）**。

---

## 二、整体架构设计

### 2.1 三层架构总览

参考 Shannon 的三层架构思想，结合本项目 Go + Eino 技术栈，设计如下分层：

```
┌─────────────────────────────────────────────────────────────────┐
│                    Orchestrator 编排层                           │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐   │
│  │ ReviewOrch   │  │ TaskPlanner  │  │  QualityGate         │   │
│  │ (主Agent/    │  │ (任务分解/   │  │  (Reflection质量评估) │   │
│  │  Supervisor) │  │  DAG编排)    │  │                      │   │
│  └──────┬───────┘  └──────────────┘  └──────────────────────┘   │
│         │                                                        │
├─────────┼────────────────────────────────────────────────────────┤
│         │           Worker Agent 执行层                          │
│  ┌──────┴──────────────────────────────────────────────────┐     │
│  │                                                         │     │
│  │  ┌─────────────┐ ┌──────────────┐ ┌─────────────────┐  │     │
│  │  │ ClauseAgent  │ │ RiskAgent    │ │ SuggestionAgent │  │     │
│  │  │ (条款拆分    │ │ (风险识别    │ │ (修改建议       │  │     │
│  │  │  与分类)     │ │  与验证)     │ │  生成)          │  │     │
│  │  └──────────────┘ └──────┬───────┘ └─────────────────┘  │     │
│  │                          │                               │     │
│  └──────────────────────────┼───────────────────────────────┘     │
│                             │                                     │
├─────────────────────────────┼─────────────────────────────────────┤
│                             │      Tool & RAG 工具层              │
│  ┌──────────────┐  ┌───────┴──────┐  ┌────────────────────────┐  │
│  │ VectorStore  │  │ RAGRetriever │  │ RuleVerifier           │  │
│  │ (Milvus/     │  │ (混合检索:   │  │ (规则验证工具:          │  │
│  │  Weaviate)   │  │  向量+关键词)│  │  确认风险有据可依)     │  │
│  └──────────────┘  └──────────────┘  └────────────────────────┘  │
│  ┌──────────────┐  ┌──────────────┐  ┌────────────────────────┐  │
│  │ DocParser    │  │ PromptMgr    │  │ ContractContext        │  │
│  │ (智能文档    │  │ (三级Prompt  │  │ (合同上下文            │  │
│  │  解析工具)   │  │  管理工具)   │  │  管理工具)             │  │
│  └──────────────┘  └──────────────┘  └────────────────────────┘  │
└──────────────────────────────────────────────────────────────────┘
```

### 2.2 核心设计理念

**模式优先，框架其次**。本方案采用以下 Agent 设计模式的组合：


| 模式               | 应用场景                         | 对应书中章节    |
| ---------------- | ---------------------------- | --------- |
| **ReAct**        | 每个 Worker Agent 的思考-行动-观察循环  | 第 2 章     |
| **Planning**     | Orchestrator 对合同审阅任务的分解与编排   | 第 10 章    |
| **Reflection**   | 审阅结果的质量评估与迭代改进               | 第 11 章    |
| **Supervisor**   | 主 Agent 监督和协调多个 Worker Agent | 第 13-15 章 |
| **Tool Calling** | Agent 调用 RAG 检索、规则验证等工具      | 第 3 章     |


---

## 三、多 Agent 详细设计

### 3.1 Agent 角色定义

#### 3.1.1 ReviewOrchestrator（审阅编排主 Agent / Supervisor）

```go
// 角色：指挥家，不亲自审阅，负责任务分解、Agent 调度、结果综合
type ReviewOrchestrator struct {
    llm             ChatModel
    clauseAgent     *ClauseAgent
    riskAgent       *RiskAgent
    suggestionAgent *SuggestionAgent
    qualityGate     *QualityGate
    ragRetriever    *RAGRetriever
    promptMgr       *PromptManager
    config          OrchestratorConfig
}

type OrchestratorConfig struct {
    MaxIterations       int     // ReAct 最大轮数，默认 10
    MinIterations       int     // 最小轮数，防偷懒，默认 1
    ReflectionEnabled   bool    // 是否启用 Reflection
    ReflectionThreshold float64 // 质量阈值，默认 0.7
    MaxReflectionRetries int    // 最大反思重试次数，默认 2
    TokenBudget         int     // Token 预算上限
    Timeout             time.Duration // 超时时间
}
```

**职责：**

1. 接收审阅任务，解析合同元信息（类型、甲方/乙方、合同金额等）
2. 根据合同类型从 PromptManager 加载对应的审阅规则
3. 调用 ClauseAgent 进行智能条款拆分
4. 将条款分发给 RiskAgent 并行进行风险识别
5. 汇总风险点，调用 SuggestionAgent 生成修改建议
6. 通过 QualityGate 进行 Reflection 质量评估
7. 综合生成最终审阅报告

#### 3.1.2 ClauseAgent（条款拆分 Agent）

```go
// 角色：合同结构化专家，负责将合同拆分为有意义的条款单元
type ClauseAgent struct {
    llm          ChatModel
    tools        []Tool  // DocParser
    reactConfig  ReactConfig
}

type ReactConfig struct {
    MaxIterations     int // 默认 5
    MinIterations     int // 默认 1
    ObservationWindow int // 观察窗口，默认保留最近 3 轮
}

// Clause 表示一个合同条款单元
type Clause struct {
    ID           string   // 条款唯一标识 e.g. "article-3-2"
    Title        string   // 条款标题 e.g. "第三条 付款方式 第二款"
    Content      string   // 条款内容
    Category     string   // 条款分类: 主体条款/权利义务/违约责任/争议解决/附则
    ParentID     string   // 父条款ID（支持多级嵌套）
    Dependencies []string // 依赖的其他条款ID（如"按第X条约定"）
    Metadata     map[string]string // 额外元数据
}
```

**ReAct 循环示例：**

```
[第 1 轮]
思考：需要先识别合同的整体结构，找到所有一级标题
行动：调用 DocParser 工具，提取文档段落结构
观察：识别到 12 个一级条款，包含标题和正文

[第 2 轮]
思考：需要进一步拆分多级子条款，并标记条款间引用关系
行动：对每个一级条款进行子条款拆分，标记"按第X条"的引用关系
观察：共拆分出 47 个条款单元，发现 8 处跨条款引用

[第 3 轮]
思考：条款拆分完成，对条款进行分类标注
行动：为每个条款标注 Category
观察：主体条款 3 个、权利义务 15 个、违约责任 8 个、争议解决 3 个...

[终止：任务完成]
```

#### 3.1.3 RiskAgent（风险识别与验证 Agent）

```go
// 角色：风险猎人，识别条款风险并通过 RAG 验证风险有据可依
type RiskAgent struct {
    llm          ChatModel
    tools        []Tool  // RAGRetriever, RuleVerifier
    reactConfig  ReactConfig
}

// RiskFinding 表示一个经过验证的风险发现
type RiskFinding struct {
    ClauseID        string          // 关联条款ID
    RiskType        string          // 风险类型
    RiskLevel       string          // 风险等级: 高/中/低
    RiskDescription string          // 风险描述
    OriginalText    string          // 原文摘录
    LegalBasis      []LegalBasis    // 法律依据（来自 RAG）
    Verified        bool            // 是否经过 RAG 验证
    Confidence      float64         // 置信度 0.0-1.0
}

// LegalBasis 法律依据
type LegalBasis struct {
    Source     string  // 来源文档名
    Article   string  // 条款编号
    Content   string  // 相关内容摘录
    Relevance float64 // 相关度评分 0.0-1.0
}
```

**关键创新 — 风险验证必须有据可依：**

```
[第 1 轮]
思考：分析该条款"甲方有权随时单方面解除合同且无需承担违约责任"，初步判断存在
     权利不对等风险
行动：调用 RAG 检索工具，查询"单方面解除合同 免除违约责任 审阅规范"
观察：检索到 3 条相关审阅条例:
     1. 《合同审查要点-服务类》第4.2条: "单方解约条款需明确触发条件"
     2. 《民法典》第563条: "合同解除的法定事由"
     3. 《风险审查标准》: "免责条款需对等约定"

[第 2 轮]
思考：RAG 返回了相关规范，需要验证当前条款是否确实违反这些规范
行动：调用 RuleVerifier 工具，传入条款内容和检索到的规范，验证风险是否成立
观察：验证结果:
     - 违反《合同审查要点》第4.2条 → 确认风险 (confidence: 0.92)
     - 与《民法典》第563条不符 → 确认风险 (confidence: 0.88)
     → 综合判定：高风险，已验证

[第 3 轮]
思考：风险已确认并有法律依据支撑，构建 RiskFinding 输出
行动：生成结构化风险报告
观察：风险报告生成完成

[终止：已验证风险，有法规依据]
```

**核心规则：未经 RAG 验证的风险不进入最终报告（或标记为"待验证"降级处理）。**

#### 3.1.4 SuggestionAgent（修改建议 Agent）

```go
// 角色：法律顾问，基于风险发现和审阅规范生成修改建议
type SuggestionAgent struct {
    llm          ChatModel
    tools        []Tool  // RAGRetriever, ContractContext
    reactConfig  ReactConfig
}

// Suggestion 修改建议
type Suggestion struct {
    RiskFindingID   string   // 关联的风险发现ID
    OriginalText    string   // 原文
    SuggestedText   string   // 建议修改后的文本
    Reason          string   // 修改理由
    LegalReference  string   // 法律依据引用
    Impact          string   // 修改影响评估
    Priority        string   // 优先级: 必须修改/建议修改/可选修改
}
```

**ReAct 循环：**

```
[第 1 轮]
思考：收到风险发现 — 单方解约条款权利不对等。需要检索类似条款的标准表述
行动：调用 RAG 检索"服务合同 单方解约 标准条款 示范文本"
观察：检索到 2 个示范文本片段，包含对等解约条件的标准表述

[第 2 轮]
思考：结合检索到的标准表述和当前合同上下文，生成针对性修改建议
行动：调用 ContractContext 获取合同甲乙方信息、合同类型
观察：甲方=XX公司，乙方=YY公司，合同类型=服务类合同

[第 3 轮]
思考：已有足够信息，生成修改建议
行动：生成修改建议，包含修改后文本、修改理由、法律依据
观察：建议生成完成

[终止：建议生成完成]
```

### 3.2 QualityGate（质量评估 - Reflection 模式）

```go
type QualityGate struct {
    llm    ChatModel
    config ReflectionConfig
}

type ReflectionConfig struct {
    Enabled             bool
    MaxRetries          int     // 默认 2
    ConfidenceThreshold float64 // 默认 0.7
    Criteria            []string
}

type QualityEvaluation struct {
    OverallScore     float64              // 0.0-1.0 综合评分
    CriteriaScores   map[string]float64   // 各维度评分
    CriticalGaps     []string             // 关键缺口
    Feedback         string               // 改进建议
    ShouldRetry      bool                 // 是否需要重试
}
```

**评估维度：**


| 维度                 | 说明                 | 权重   |
| ------------------ | ------------------ | ---- |
| completeness       | 是否覆盖所有条款类别的审阅      | 0.25 |
| legal_accuracy     | 法律依据是否准确、是否经过RAG验证 | 0.30 |
| risk_coverage      | 高中低风险覆盖是否合理        | 0.20 |
| suggestion_quality | 修改建议是否具体可行         | 0.15 |
| consistency        | 审阅风格和术语是否一致        | 0.10 |


**确定性护栏（硬规则）：**

```go
func (qg *QualityGate) applyGuardrails(eval *QualityEvaluation, report *ReviewReport) {
    // 规则 1: 没有任何高风险发现但声称覆盖完整 → 降低置信度
    if len(report.HighRiskFindings) == 0 && eval.OverallScore > 0.9 {
        eval.OverallScore = 0.6
        eval.Feedback += "未发现高风险点但评分过高，需要复查。"
        eval.ShouldRetry = true
    }

    // 规则 2: 存在未验证的风险发现 → 必须重试
    unverifiedCount := countUnverified(report.Findings)
    if unverifiedCount > 0 {
        eval.CriticalGaps = append(eval.CriticalGaps,
            fmt.Sprintf("%d 个风险点未经 RAG 验证", unverifiedCount))
        eval.ShouldRetry = true
    }

    // 规则 3: 审阅结果太少（每万字少于 2 个风险点）→ 可能遗漏
    riskDensity := float64(len(report.Findings)) / float64(report.WordCount/10000)
    if riskDensity < 2.0 && eval.OverallScore > 0.8 {
        eval.OverallScore = 0.5
        eval.Feedback += "风险点密度过低，可能存在遗漏。"
        eval.ShouldRetry = true
    }

    // 规则 4: 达到最大重试次数 → 强制停止
    if report.ReflectionCount >= qg.config.MaxRetries {
        eval.ShouldRetry = false
    }
}
```

---

## 四、RAG 系统详细设计

### 4.1 知识库结构

```
审阅知识库 (Vector Store)
├── 审阅规范层
│   ├── 通用合同审查要点           # 适用所有合同类型的基础规范
│   ├── 服务类合同审查要点         # 按合同类型细分
│   ├── 货物类合同审查要点
│   ├── 基建类合同审查要点
│   └── ...
├── 法律法规层
│   ├── 民法典-合同编               # 核心法律条文
│   ├── 招标投标法
│   ├── 政府采购法
│   └── 行业监管规定
├── 风险案例层
│   ├── 典型风险条款案例库          # 历史审阅中发现的典型风险
│   ├── 争议裁判案例摘要
│   └── 常见合同纠纷分析
└── 示范文本层
    ├── 标准合同示范文本            # 各类合同的示范条款
    ├── 推荐条款表述库
    └── 行业标准模板
```

### 4.2 文档处理与向量化 Pipeline

```go
// Document 知识库文档
type Document struct {
    ID         string            `json:"id"`
    Title      string            `json:"title"`
    Content    string            `json:"content"`
    Category   string            `json:"category"`   // 规范/法规/案例/示范
    SubCategory string           `json:"sub_category"` // 服务类/货物类/基建类/通用
    Source     string            `json:"source"`     // 来源
    Metadata   map[string]string `json:"metadata"`
}

// Chunk 文档分块
type Chunk struct {
    ID         string            `json:"id"`
    DocID      string            `json:"doc_id"`
    Content    string            `json:"content"`
    Embedding  []float32         `json:"embedding"`
    Metadata   map[string]string `json:"metadata"`
}
```

**文档处理流程：**

```
原始文档(PDF/DOCX/TXT)
    ↓
文本提取 (DocParser)
    ↓
智能分块 (按条款/章节边界，非固定长度)
    ↓
元数据标注 (合同类型、法规编号、条款编号)
    ↓
Embedding 生成 (调用 Embedding API)
    ↓
存入向量数据库 (Milvus/Weaviate)
    ↓
建立倒排索引 (关键词检索)
```

### 4.3 混合检索策略（Hybrid Retrieval）

```go
// RAGRetriever 混合检索器
type RAGRetriever struct {
    vectorStore    VectorStore    // 向量数据库客户端
    keywordIndex   KeywordIndex   // 关键词索引（倒排）
    embeddingModel EmbeddingModel // Embedding 模型
    reranker       Reranker       // 重排序模型
    config         RetrieverConfig
}

type RetrieverConfig struct {
    TopK           int     // 每路检索返回数，默认 10
    FinalTopK      int     // 重排序后最终返回数，默认 5
    VectorWeight   float64 // 向量检索权重，默认 0.6
    KeywordWeight  float64 // 关键词检索权重，默认 0.4
    MinRelevance   float64 // 最低相关度阈值，默认 0.5
}

// Retrieve 混合检索方法
func (r *RAGRetriever) Retrieve(ctx context.Context, query string, filters map[string]string) ([]RetrievalResult, error) {
    // 1. 向量检索 —— 语义相似
    vectorResults := r.vectorStore.Search(ctx, r.embeddingModel.Embed(query), r.config.TopK, filters)

    // 2. 关键词检索 —— 精确匹配法规编号、条款名
    keywordResults := r.keywordIndex.Search(ctx, query, r.config.TopK, filters)

    // 3. 结果融合（RRF - Reciprocal Rank Fusion）
    merged := reciprocalRankFusion(vectorResults, keywordResults, r.config.VectorWeight, r.config.KeywordWeight)

    // 4. 重排序（Cross-Encoder）
    reranked := r.reranker.Rerank(ctx, query, merged, r.config.FinalTopK)

    // 5. 过滤低相关度
    filtered := filterByRelevance(reranked, r.config.MinRelevance)

    return filtered, nil
}
```

### 4.4 检索增强的分层过滤

```go
// 检索时根据合同类型进行分层过滤
func buildRetrievalFilters(contractType string, riskType string) map[string]string {
    filters := map[string]string{}

    // 优先检索对应合同类型的规范
    if contractType != "" {
        filters["sub_category"] = contractType
    }

    // 如果有风险类型，进一步缩小范围
    if riskType != "" {
        filters["risk_category"] = riskType
    }

    return filters
}

// 分层检索策略: 先精确 → 再泛化
func (r *RAGRetriever) LayeredRetrieve(ctx context.Context, query string, contractType string) ([]RetrievalResult, error) {
    // 第一层: 检索该合同类型的专用规范
    specificResults, _ := r.Retrieve(ctx, query, map[string]string{
        "sub_category": contractType,
    })

    // 如果专用规范结果充分，直接返回
    if len(specificResults) >= 3 {
        return specificResults, nil
    }

    // 第二层: 扩大到通用规范
    generalResults, _ := r.Retrieve(ctx, query, map[string]string{
        "sub_category": "通用",
    })

    // 合并去重
    return mergeAndDedup(specificResults, generalResults), nil
}
```

---

## 五、RuleVerifier 工具 — 风险验证的关键创新

### 5.1 设计思想

**核心原则：Agent 识别的风险点，必须通过工具调用在审阅规范/法律条例中找到依据，才能进入最终报告。**

这不是简单的 RAG 检索后拼接，而是一个**结构化验证流程**：

```
RiskAgent 初步识别风险
    ↓
调用 RAGRetriever 检索相关规范
    ↓
调用 RuleVerifier 进行结构化验证
    ↓
验证通过 → RiskFinding.Verified = true，附带 LegalBasis
验证不通过 → RiskFinding.Verified = false，降级为"待人工复核"
```

### 5.2 RuleVerifier 实现

```go
// RuleVerifier 规则验证工具
type RuleVerifier struct {
    llm ChatModel
}

// VerificationRequest 验证请求
type VerificationRequest struct {
    ClauseText      string            // 待验证的合同条款
    IdentifiedRisk  string            // Agent 初步识别的风险描述
    RetrievedRules  []RetrievalResult // RAG 检索到的相关规范
    ContractType    string            // 合同类型
    Stance          string            // 审查立场（甲方/乙方）
}

// VerificationResult 验证结果
type VerificationResult struct {
    IsVerified     bool         // 风险是否得到验证
    Confidence     float64      // 置信度 0.0-1.0
    MatchedRules   []MatchedRule // 匹配到的具体规则
    Reasoning      string       // 验证推理过程
    RiskLevel      string       // 确认后的风险等级
}

type MatchedRule struct {
    RuleSource  string  // 规范来源
    RuleArticle string  // 具体条款编号
    RuleContent string  // 规范内容
    MatchScore  float64 // 匹配度
    Explanation string  // 匹配解释
}

// Verify 执行风险验证
func (rv *RuleVerifier) Verify(ctx context.Context, req *VerificationRequest) (*VerificationResult, error) {
    // 构建验证 Prompt
    verifyPrompt := buildVerificationPrompt(req)

    // 调用 LLM 进行结构化推理
    response := rv.llm.Generate(ctx, verifyPrompt)

    // 解析验证结果
    result := parseVerificationResult(response)

    // 确定性护栏
    result = applyVerificationGuardrails(result, req)

    return result, nil
}

// applyVerificationGuardrails 验证护栏
func applyVerificationGuardrails(result *VerificationResult, req *VerificationRequest) *VerificationResult {
    // 护栏 1: 没有任何匹配规则但声称已验证 → 不可信
    if len(result.MatchedRules) == 0 && result.IsVerified {
        result.IsVerified = false
        result.Confidence = 0.2
        result.Reasoning += " [护栏修正: 无匹配规则，验证结果不可信]"
    }

    // 护栏 2: 匹配度最高的规则低于 0.5 → 降级
    if len(result.MatchedRules) > 0 {
        maxScore := 0.0
        for _, rule := range result.MatchedRules {
            if rule.MatchScore > maxScore {
                maxScore = rule.MatchScore
            }
        }
        if maxScore < 0.5 {
            result.Confidence *= 0.5
        }
    }

    return result
}
```

---

## 六、完整审阅流程（DAG 工作流）

### 6.1 流程编排

```
用户启动审阅任务
    │
    ▼
┌──────────────────────────────────────┐
│  Phase 1: 准备阶段                    │
│  ┌────────────────────────────────┐  │
│  │ 1.1 加载合同文本               │  │
│  │ 1.2 解析合同元信息(甲乙方等)   │  │
│  │ 1.3 加载对应合同类型的 Prompt   │  │
│  │ 1.4 从 RAG 预检索该类型审阅要点 │  │
│  └────────────────────────────────┘  │
└──────────────┬───────────────────────┘
               │
               ▼
┌──────────────────────────────────────┐
│  Phase 2: 条款拆分（ClauseAgent）     │
│  ┌────────────────────────────────┐  │
│  │ 2.1 智能条款拆分(非暴力分块)   │  │
│  │ 2.2 条款分类标注               │  │
│  │ 2.3 依赖关系图构建             │  │
│  └────────────────────────────────┘  │
└──────────────┬───────────────────────┘
               │
               ▼
┌──────────────────────────────────────┐
│  Phase 3: 风险识别（RiskAgent × N）   │
│  ┌────────────────────────────────┐  │
│  │ 3.1 并发审阅各条款             │  │ ← 按条款类别分组并发
│  │ 3.2 初步识别风险点             │  │
│  │ 3.3 RAG 检索相关审阅规范       │  │ ← ReAct: 思考→检索→验证
│  │ 3.4 RuleVerifier 验证风险      │  │ ← 有据可依才确认
│  │ 3.5 生成 RiskFinding           │  │
│  └────────────────────────────────┘  │
└──────────────┬───────────────────────┘
               │
               ▼
┌──────────────────────────────────────┐
│  Phase 4: 修改建议（SuggestionAgent） │
│  ┌────────────────────────────────┐  │
│  │ 4.1 遍历已验证的 RiskFinding   │  │
│  │ 4.2 RAG 检索示范条款文本       │  │
│  │ 4.3 结合上下文生成修改建议     │  │
│  │ 4.4 评估修改影响范围           │  │
│  └────────────────────────────────┘  │
└──────────────┬───────────────────────┘
               │
               ▼
┌──────────────────────────────────────┐
│  Phase 5: 质量反思（QualityGate）     │
│  ┌────────────────────────────────┐  │
│  │ 5.1 评估审阅完整性             │  │
│  │ 5.2 验证法律依据准确性         │  │
│  │ 5.3 检查建议可行性             │  │
│  │ 5.4 确定性护栏检查             │  │
│  │ 5.5 不达标 → 反馈给 Phase 3    │  │ ← Reflection 循环
│  └────────────────────────────────┘  │
└──────────────┬───────────────────────┘
               │
               ▼
┌──────────────────────────────────────┐
│  Phase 6: 报告生成                    │
│  ┌────────────────────────────────┐  │
│  │ 6.1 汇总所有风险发现           │  │
│  │ 6.2 生成审阅摘要               │  │
│  │ 6.3 生成修改建议优先级排序     │  │
│  │ 6.4 SSE 流式输出               │  │
│  └────────────────────────────────┘  │
└──────────────────────────────────────┘
```

### 6.2 DAG 依赖关系

```
Phase1 (准备)
    ↓
Phase2 (条款拆分)              ← 依赖 Phase1 的合同文本
    ↓
Phase3 (风险识别) × N 并发      ← 依赖 Phase2 的条款列表，各条款可并行
    ↓
Phase4 (修改建议)              ← 依赖 Phase3 的风险发现
    ↓
Phase5 (质量评估)              ← 依赖 Phase3 + Phase4 的全部输出
    ↓ (不达标则回到 Phase3)
Phase6 (报告生成)              ← 依赖 Phase5 通过
```

---

## 七、数据模型扩展

### 7.1 新增数据表

```sql
-- 审阅规范知识库文档表
CREATE TABLE review_knowledge_docs (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    title VARCHAR(255) NOT NULL COMMENT '文档标题',
    category ENUM('规范','法规','案例','示范') NOT NULL COMMENT '文档分类',
    sub_category VARCHAR(64) COMMENT '子分类(合同类型)',
    source VARCHAR(255) COMMENT '来源',
    content TEXT COMMENT '文档全文',
    chunk_count INT DEFAULT 0 COMMENT '分块数量',
    status ENUM('pending','indexed','failed') DEFAULT 'pending',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

-- 文档分块与向量索引表（元数据，向量存在 Milvus）
CREATE TABLE review_knowledge_chunks (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    doc_id BIGINT NOT NULL COMMENT '所属文档ID',
    chunk_index INT NOT NULL COMMENT '分块序号',
    content TEXT NOT NULL COMMENT '分块内容',
    vector_id VARCHAR(128) COMMENT '向量数据库中的ID',
    metadata JSON COMMENT '元数据',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (doc_id) REFERENCES review_knowledge_docs(id)
);

-- 审阅 Agent 执行日志表（可观测性）
CREATE TABLE review_agent_logs (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    task_id BIGINT NOT NULL COMMENT '审阅任务ID',
    agent_type VARCHAR(32) NOT NULL COMMENT 'Agent类型',
    phase VARCHAR(32) NOT NULL COMMENT '执行阶段',
    iteration INT DEFAULT 0 COMMENT 'ReAct轮数',
    thought TEXT COMMENT '思考内容',
    action TEXT COMMENT '执行动作',
    observation TEXT COMMENT '观察结果',
    tokens_used INT DEFAULT 0 COMMENT 'Token消耗',
    duration_ms INT DEFAULT 0 COMMENT '耗时(毫秒)',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_task_id (task_id)
);

-- 风险验证记录表
CREATE TABLE risk_verifications (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    task_id BIGINT NOT NULL,
    clause_id VARCHAR(64) NOT NULL COMMENT '条款ID',
    risk_type VARCHAR(64) COMMENT '风险类型',
    is_verified BOOLEAN DEFAULT FALSE COMMENT '是否验证通过',
    confidence DECIMAL(3,2) COMMENT '置信度',
    matched_rules JSON COMMENT '匹配到的规则列表',
    reasoning TEXT COMMENT '验证推理过程',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_task_id (task_id)
);
```

### 7.2 扩展现有 ReviewResult 表

```sql
-- 在现有 review_results 表增加字段
ALTER TABLE review_results ADD COLUMN clause_id VARCHAR(64) COMMENT '关联条款ID';
ALTER TABLE review_results ADD COLUMN is_verified BOOLEAN DEFAULT FALSE COMMENT '是否经RAG验证';
ALTER TABLE review_results ADD COLUMN legal_basis JSON COMMENT '法律依据(JSON)';
ALTER TABLE review_results ADD COLUMN confidence DECIMAL(3,2) COMMENT '置信度';
ALTER TABLE review_results ADD COLUMN verification_id BIGINT COMMENT '关联验证记录ID';
ALTER TABLE review_results ADD COLUMN priority ENUM('必须修改','建议修改','可选修改') DEFAULT '建议修改';
```

---

## 八、项目目录结构优化

```
app/
├── internal/
│   ├── agent/                        # Agent 核心层
│   │   ├── orchestrator.go           # ReviewOrchestrator 主Agent
│   │   ├── clause_agent.go           # ClauseAgent 条款拆分
│   │   ├── risk_agent.go             # RiskAgent 风险识别与验证
│   │   ├── suggestion_agent.go       # SuggestionAgent 修改建议
│   │   ├── quality_gate.go           # QualityGate Reflection 质量评估
│   │   ├── react_loop.go             # ReAct 循环通用实现
│   │   ├── agent_types.go            # Agent 接口与类型定义
│   │   ├── service.go                # 现有的合同解析Agent(保留)
│   │   └── prompts/
│   │       ├── contract_extract.prompt    # 现有(保留)
│   │       ├── clause_split.prompt        # 条款拆分提示词
│   │       ├── risk_identify.prompt       # 风险识别提示词
│   │       ├── risk_verify.prompt         # 风险验证提示词
│   │       ├── suggestion_gen.prompt      # 修改建议提示词
│   │       └── quality_eval.prompt        # 质量评估提示词
│   │
│   ├── rag/                          # RAG 检索增强层
│   │   ├── retriever.go              # RAGRetriever 混合检索器
│   │   ├── embedder.go               # Embedding 服务
│   │   ├── vector_store.go           # 向量数据库接口
│   │   ├── keyword_index.go          # 关键词索引
│   │   ├── reranker.go               # 重排序
│   │   ├── document_processor.go     # 文档处理 Pipeline
│   │   └── types.go                  # RAG 相关类型定义
│   │
│   ├── tools/                        # Agent 工具层
│   │   ├── tool_interface.go         # Tool 统一接口
│   │   ├── rag_search_tool.go        # RAG 检索工具(供Agent调用)
│   │   ├── rule_verifier_tool.go     # 规则验证工具
│   │   ├── contract_context_tool.go  # 合同上下文工具
│   │   └── doc_parser_tool.go        # 文档解析工具
│   │
│   ├── knowledge/                    # 知识库管理
│   │   ├── handler.go                # 知识库管理 API
│   │   ├── service.go                # 知识库管理服务
│   │   ├── repo.go                   # 知识库数据访问
│   │   └── model.go                  # 知识库数据模型
│   │
│   ├── review/                       # 审阅模块(重构)
│   │   ├── handler.go                # HTTP Handler(保留，扩展)
│   │   ├── service.go                # 审阅服务(重构为调用Orchestrator)
│   │   ├── repo.go                   # 数据访问(保留)
│   │   ├── model.go                  # 数据模型(扩展)
│   │   └── schemas.go               # 请求响应结构(扩展)
│   │
│   └── ... (其他模块保持不变)
│
└── pkg/
    └── ... (保持不变)
```

---

## 九、关键接口设计

### 9.1 Agent 统一接口

```go
// Agent 统一接口 — 所有 Worker Agent 实现此接口
type Agent interface {
    // Name 返回 Agent 名称
    Name() string

    // Execute 执行任务，返回结果
    Execute(ctx context.Context, input AgentInput) (AgentOutput, error)

    // AvailableTools 返回该 Agent 可用的工具列表
    AvailableTools() []Tool
}

// AgentInput Agent 输入
type AgentInput struct {
    Task        string                 // 任务描述
    Context     map[string]interface{} // 上下文信息
    Constraints AgentConstraints       // 约束条件
}

// AgentConstraints Agent 约束
type AgentConstraints struct {
    MaxIterations int           // 最大 ReAct 轮数
    TokenBudget   int           // Token 预算
    Timeout       time.Duration // 超时时间
}

// AgentOutput Agent 输出
type AgentOutput struct {
    Result      interface{}   // 执行结果
    Thinking    []ThinkStep   // 思考过程记录（可观测性）
    TokensUsed  int           // Token 消耗
    Duration    time.Duration // 耗时
}

// ThinkStep 思考步骤记录
type ThinkStep struct {
    Iteration   int    // 轮数
    Thought     string // 思考内容
    Action      string // 执行动作
    ActionInput string // 动作输入
    Observation string // 观察结果
    Timestamp   time.Time
}
```

### 9.2 Tool 统一接口

```go
// Tool 工具统一接口 — 对应书中 Function Calling
type Tool interface {
    // Name 工具名称
    Name() string

    // Description 工具描述（供 LLM 理解何时该调用）
    Description() string

    // Parameters 参数 Schema（JSON Schema 格式）
    Parameters() map[string]interface{}

    // Execute 执行工具
    Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error)
}

// ToolResult 工具执行结果
type ToolResult struct {
    Success bool        // 是否成功
    Data    interface{} // 返回数据
    Error   string      // 错误信息
}
```

---

## 十、技术选型

### 10.1 向量数据库选型


| 方案           | 优势                    | 劣势      | 推荐场景 |
| ------------ | --------------------- | ------- | ---- |
| **Milvus**   | 分布式、高性能、Go SDK 成熟     | 部署复杂    | 生产环境 |
| **Weaviate** | 内置混合检索、REST API       | Go 生态较弱 | 快速原型 |
| **Qdrant**   | Rust 实现、性能好、Go client | 社区较小    | 性能敏感 |
| **Chroma**   | 简单易用、Python 生态        | 非生产级    | 开发测试 |


**推荐：Milvus**（与 Go 技术栈匹配，支持分布式扩展，有成熟的 Go SDK）

### 10.2 Embedding 模型


| 模型                     | 维度   | 中文支持 | 推荐用途   |
| ---------------------- | ---- | ---- | ------ |
| text-embedding-3-small | 1536 | 良好   | 通用场景   |
| text-embedding-3-large | 3072 | 良好   | 高精度    |
| BAAI/bge-large-zh-v1.5 | 1024 | 优秀   | 中文法律文档 |
| 智谱 embedding-3         | 2048 | 优秀   | 国产替代   |


**推荐：BAAI/bge-large-zh-v1.5**（中文法律领域效果最佳）或**智谱 embedding-3**（兼容国内部署）

### 10.3 新增 Go 依赖

```
github.com/milvus-io/milvus-sdk-go/v2  # Milvus Go SDK
```

其余组件（Embedding、Reranker）可通过现有 Eino/Arkbot 框架对接，或直接 HTTP 调用 API。

---

## 十一、实施路线图

### Phase 1: RAG 基础设施（预计 3-5 天）

- 搭建 Milvus 或选用向量数据库
- 实现 `rag/` 包：Embedder、VectorStore、Retriever
- 实现知识库文档处理 Pipeline
- 导入首批审阅规范和法律法规文档
- 实现知识库管理 API（上传、索引、查询）

### Phase 2: Agent 核心框架（预计 3-5 天）

- 实现 `agent/react_loop.go` — ReAct 循环通用框架
- 实现 `agent/agent_types.go` — Agent 和 Tool 接口
- 实现 `tools/` 包 — RAG 检索工具、规则验证工具
- 编写各 Agent 的 Prompt 模板

### Phase 3: Worker Agent 实现（预计 5-7 天）

- 实现 ClauseAgent — 智能条款拆分
- 实现 RiskAgent — 风险识别与 RAG 验证
- 实现 SuggestionAgent — 修改建议生成
- 实现 RuleVerifier — 风险验证工具

### Phase 4: 编排与质量控制（预计 3-5 天）

- 实现 ReviewOrchestrator — 主 Agent / Supervisor
- 实现 QualityGate — Reflection 质量评估
- 实现 DAG 工作流编排（Phase1-6）
- 接入三级 Prompt 管理系统

### Phase 5: 集成与优化（预计 3-5 天）

- 重构 `review/service.go` 接入 Orchestrator
- 扩展 SSE 流式输出（支持 Agent 思考过程可视化）
- Agent 执行日志与可观测性
- 数据库 migration
- 端到端测试

---

## 十二、与现有代码的对照关系


| 现有模块                                            | 改造方式   | 说明                                |
| ----------------------------------------------- | ------ | --------------------------------- |
| `agent/service.go`                              | **保留** | 合同解析 Agent 保持不变                   |
| `review/service.go` → `ReviewContract()`        | **重构** | 替换为 Orchestrator 调度               |
| `review/service.go` → `ProcessContractReview()` | **重构** | 替换为 DAG 工作流                       |
| `review/service.go` → `splitTextByLength()`     | **替换** | 替换为 ClauseAgent 智能拆分              |
| `review/service.go` → `parseReviewResult()`     | **优化** | Agent 输出已结构化，简化解析                 |
| `review/service.go` → `buildReviewPrompt()`     | **替换** | Prompt 由 PromptManager + RAG 动态构建 |
| `review/handler.go` → SSE 输出                    | **扩展** | 增加 Agent 思考过程的实时输出                |
| `prompts/model.go` & `repo.go`                  | **接入** | Orchestrator 加载三级 Prompt          |
| `review/model.go`                               | **扩展** | ReviewResult 增加验证字段               |


---

## 十三、参考资料

- [AI Agent 架构：从单体到企业级多智能体](https://www.waylandz.com/ai-agent-book/) — 核心设计范式来源
  - 第 1 章：Agent 的本质 — 四核心组件（大脑+手脚+记忆+主见）
  - 第 2 章：ReAct 循环 — 思考-行动-观察模式
  - 第 3 章：工具调用基础 — Function Calling 实现
  - 第 10 章：Planning 模式 — 任务分解与覆盖度评估
  - 第 11 章：Reflection 模式 — 质量评估与迭代改进
  - 第 13 章：编排基础 — Supervisor 多 Agent 协作
- [Shannon OSS](https://github.com/Kocoro-lab/Shannon) — 三层架构参考实现
- [cloudwego/eino](https://github.com/cloudwego/eino) — 项目使用的 LLM 框架
- [Milvus](https://milvus.io/) — 推荐的向量数据库

