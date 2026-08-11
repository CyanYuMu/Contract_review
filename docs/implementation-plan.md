# 合同审阅智能体 — 增强方案实施计划

> 决策已确认:① Go 原生网关 ② 合同分类替换为七大类 ③ 问答 SSE 流式+多轮+绑定合同 ④ 成本按 用户+功能模块 双维度

---

## 一、现状与差距总结

| 模块 | 已有 | 需新增/改造 |
|---|---|---|
| 合同分类 | `contract.ContractType` 表(基建/货物/服务/通用) | **替换为七大类** + 迁移数据 + 补 prompt 模板 + 知识库 sub_category 对齐 |
| 知识库风险点 | `riskconfig.RiskPoint`→`syncKnowledgeDoc`→`review_knowledge_*` + RAG 三路混合检索 | 基本完整;随分类调整刷新种子,补齐七类风险点条例 |
| RAG 审阅 | `ReviewOrchestrator`(Clause→CandidateRisk→Suggestion→QualityGate) | 随分类调整,无大改 |
| 大模型网关 | `modelconfig` 表(统一接口+集中 Key)+ `llm.NewChatModel` 工厂 | **新增**:模型路由别名、限流、配额、成本追踪、语义缓存、流式支持 |
| 合同问答 | `/api/chat` stub | **全新**:qa 模块+SSE 流式+多轮+绑定合同 RAG |
| 前端 | 审阅页/比对页/历史页;`chat.ts` 占位 | **新增**:问答页(`PageLayout`+Zustand+fetch SSE)+ 成本看板 |

---

## 二、模块一:合同分类体系对齐(七大类)

**标准分类**(8 个,含兜底):
`买卖合同` `服务合同` `劳动合同` `租赁合同` `借款合同` `合作合同` `知识产权合同` `通用`

### 改动点
1. **`app/internal/review/service.go`** `contractTypePrompts` map:替换键为七大类 + 通用;新增 7 个 prompt 文件 `app/prompts/contract_reviewer_prompt_{sale,service,labor,lease,loan,coop,ip}.txt`(沿用现有 unified 模板风格,各类型审阅要点不同)。
2. **`app/internal/tools/rag_search_tool.go`** `Description()` 中合同类型示例更新为七大类。
3. **`docs/sql/review_knowledge.sql`** + **`review/service.go` `defaultReviewKnowledgeChunks()`**:为七大类各补 1-2 条 `sub_category` 种子风险点(买卖/劳动/租赁/借款/合作/知识产权),保留通用。
4. **数据迁移脚本** `docs/sql/migrate_contract_types.sql`:
   - `UPDATE contract_types SET name='买卖合同' WHERE name='货物类合同'`
   - `UPDATE contract_types SET name='买卖合同' WHERE name='基建类合同'`(或按需映射合作);`服务类合同`→`服务合同`
   - `UPDATE review_knowledge_docs SET sub_category='买卖合同' WHERE sub_category IN('货物类合同','基建类合同')` 等
   - `INSERT IGNORE` 七大类默认类型行(若不存在)
   - 风险点 `review_risk_points.contract_type_name` 同步更新
5. **`app/internal/contract/service.go`** `ensureContractTypeID`:LLM 解析出的类型名做归一化映射到七大类。

### 验收
- 七大类在 `contract_type/list` 可见;审阅时按 `sub_category` 命中对应知识库风险点。

---

## 三、模块二:知识库风险点按分类入库(增强现有)

现有 `riskconfig.Service.Create/Update` 已通过 `syncKnowledgeDoc` 写入 `review_knowledge_docs`(category=风险点, sub_category=合同类型名)。**无需重写**,仅:
- 确保七大类各有示例风险点(写入 SQL 种子 + 前端可配)。
- `riskconfig` 前端 `setting/risk` 已支持按合同类型筛选,无需改。
- RAG 检索 `LayeredRetrieve(query, contractType)` 已按 `sub_category` 分层,七大类自动生效。

> 该模块本质是数据补齐 + 分类对齐,代码改动极小。

---

## 四、模块三:大模型网关(Go 原生)— 核心新增

### 4.1 架构

新增 `app/internal/gateway/` 包,作为**业务代码调用模型的唯一入口**:

```
业务代码(review/qa/comparison)
      │  gateway.Generate(ctx, GatewayRequest{Purpose:"qa", Messages, Stream:true}, userID)
      ▼
┌─ gateway.Gateway ──────────────────────────────────────────────┐
│ 1. 路由: Purpose("qa") → model_configs(经 llm_routes) → 底层 client │
│ 2. 语义缓存: embed(query) → Redis 余弦比对 → 命中则直接返回         │
│ 3. 限流: 令牌桶(用户+功能, Redis 计数)                             │
│ 4. 配额: 校验用户 token 预算(Redis 日计数 + MySQL 配置)            │
│ 5. 调用模型(Generate 或 Stream)                                   │
│ 6. 成本记录: prompt/completion tokens + 估算花费 → llm_usage_logs  │
│ 7. 写语义缓存(未命中时)                                           │
└──────────────────────────────────────────────────────────────────┘
```

### 4.2 文件清单(`app/internal/gateway/`)
| 文件 | 职责 |
|---|---|
| `gateway.go` | `Gateway` 结构体 + `Generate()`/`Stream()` 主入口,串联中间件链 |
| `router.go` | `Purpose→ModelConfig` 路由;复用 `modelconfig` + 新增 `llm_routes` 表 |
| `ratelimit.go` | 令牌桶限流(Redis: `RL:{user}:{feature}:{window}`) |
| `quota.go` | 配额校验/扣减(Redis 日计数 + MySQL `llm_quotas` 配置) |
| `cost.go` | token 统计(解析 OpenAI usage)+ 成本估算(按 provider 单价)+ 写 `llm_usage_logs` |
| `semantic_cache.go` | 复用 `rag.OpenAIEmbeddingModel` 生成 query 向量;Redis 存 `{emb,response,ttl}`;命中用余弦相似度 ≥ 阈值 |
| `model.go` | `GatewayRequest`/`GatewayResponse`/`Usage` 等类型 + `llm_routes`/`llm_usage_logs`/`llm_quotas` GORM 模型 |
| `handler.go` | 成本/配额查询 HTTP 接口(供前端看板) |

### 4.3 关键设计

**模型路由(只改配置不动代码)**:
- 新增表 `llm_routes`: `purpose`(PK, 如 review/qa/comparison/chat/embedding)→ `model_config_id` + 可选 `params`(temperature 等)。
- 业务代码只传 `purpose`;换模型 = 改 `llm_routes` 一行(或前端 setting 配置)。
- 复用现有 `llm.NewChatModel(ctx, modelConfig)` 构造底层 client(支持 ark/openai-compatible)。

**流式支持**:
- 现有 `OpenAICompatibleModel.Stream` 未实现。在 gateway 层新增 `openai_stream.go`:请求带 `stream:true`,SSE 解析 `data: {choices[0].delta.content}` 逐 token 回调;同时累计 usage(`usage` 字段或按字符估算)。
- 审阅/Q&A 的 SSE 直接透传底层流式 token。

**语义缓存**:
- 入参:`{purpose, messages 去掉 system 后拼接, user_id}`。对"最后一条 user 消息"做 embedding。
- 存储:Redis hash `SEMCACHE:{purpose}:{cache_id}` = `{embedding_json, response, prompt_hash, created_at}`;ZSET `SEMCACHEIDX:{purpose}` 存 cache_id+score 维护 LRU/TTL。
- 查询:取出该 purpose 全部缓存向量(量小,通常 <1000),Go 内算余弦,取最大者;≥ 阈值(默认 0.92,可配)则命中。命中直接返回缓存响应(标记 `cache_hit=true`,不计成本)。
- TTL 默认 24h;可按 purpose 配置;支持手动失效。
- 复用 `rag.EmbeddingModel` 接口 + 现有 embedding 配置(vector.embedding)。

**成本追踪(用户+功能双维度)**:
- 表 `llm_usage_logs`: `id, user_id, account, feature(review/qa/comparison/chat), purpose, model_name, provider, prompt_tokens, completion_tokens, total_tokens, cost, latency_ms, cache_hit, status, created_at`。索引 `(user_id,created_at)` `(feature,created_at)`。
- 配额表 `llm_quotas`: `id, subject_type(user/role), subject_id, feature, daily_token_limit, monthly_token_limit`。
- 限流:令牌桶 Redis,`RL:{user_id}:{feature}` 每分钟 N 请求。
- 单价:`app/internal/gateway/pricing.go` 维护各 provider/model 每 1K token 单价(可配置文件)。

### 4.4 集成点
- `app/internal/global/global.go`:新增 `Gateway *gateway.Gateway`。
- `app/internal/cmd/main.go`:初始化 gateway(注入 modelconfig repo + redis + embedding),替换 `global.LLM`。
- `review/service.go` `InitLLM`/`InitOrchestrator`:改用 `global.Gateway.Generate(...)`;`llmGenerate` 闭包改为调 gateway。
- 新 `qa` 模块同样经 gateway。
- 配置 `app/config/config.yaml` 新增 `gateway` 段(ratelimit/quota/cache 开关与阈值)。

---

## 五、模块四:合同问答(SSE 流式 + 多轮 + 绑定合同)

### 5.1 后端 `app/internal/qa/`
| 文件 | 职责 |
|---|---|
| `model.go` | `QAMessage` 表(session_id, user_id, role, content, parent_id, tokens, created_at) |
| `repo.go` | 消息 CRUD + 取最近 N 条历史 |
| `schemas.go` | 请求/响应/SSE 事件结构(沿用 review SSE 模式:`message`/`error`/`end`) |
| `service.go` | 核心问答逻辑 |
| `handler.go` | SSE 流式 handler(POST `/api/qa/ask`) + 历史消息/会话接口 |

### 5.2 问答流程(service)
```
用户提问(question) + session_id(绑定合同)
  │
  ├─ 1. 校验会话: session_type="chat", file_id→contract→ExtractText(合同全文)
  ├─ 2. 取多轮历史: qa_messages 最近 10 条(role/content)
  ├─ 3. RAG 检索:
  │     a. 知识库: retriever.LayeredRetrieve(question, contract_type) → 相关风险点/法规
  │     b. 合同条款: ClauseAgent 拆分合同→按问题关键词检索相关条款(复用 ContractContextTool.search 逻辑)
  ├─ 4. 构建 prompt: system(合同法律助手) + 合同元信息 + 检索到的知识库片段 + 相关合同条款 + 历史 + question
  ├─ 5. 经 gateway.Stream(purpose="qa", messages, userID):
  │     - 语义缓存命中 → 直接回放缓存答案(标记)
  │     - 否则流式调模型,逐 token SSE 推送 {event:"message", data:{delta}}
  ├─ 6. 完成后: 存 user 消息 + assistant 消息到 qa_messages;记录 usage(经 gateway 自动完成)
  └─ 7. SSE end 事件 {event:"end", data:{message_id, tokens, cache_hit}}
```

### 5.3 路由(main.go)
- `POST /api/qa/ask`(authMW, SSE)— 提问
- `GET  /api/qa/messages?session_id=`(authMW)— 历史消息
- `POST /api/qa/session/create`(authMW)— 创建问答会话(绑定 file_id, session_type="chat")
- 复用现有 `/api/session/*`(list_sessions 已支持按 session_type 过滤)

### 5.4 前端代理
`frontend/src/app/api/proxy/[...path]/route.ts` POST handler 的 SSE 直通条件新增 `qa/ask`(或让前端带 `Accept: text/event-stream` 头即可命中现有条件)。

---

## 六、模块五:前端

### 6.1 问答页(新建)
- `frontend/src/app/qa/page.tsx` → 渲染 `<PageLayout activeTab="qa">`(沿用 history/contrast 模式)。
- `frontend/src/components/qa/QAPanel.tsx`:左合同预览/选择 + 右对话区(消息列表 + 输入框)。
- `frontend/src/lib/api/qa.ts`:
  - `askQuestion(payload, {onDelta, onEnd, onError})`:**沿用 `lib/api/startTask.ts` 的 fetch+ReadableStream 模式**(POST `/api/proxy/qa/ask`, `Accept: text/event-stream`,`response.body.getReader()`+`TextDecoder`,按 `\n\n` 切分事件,JSON.parse `data:` 行)。
  - `getMessages(sessionId)`:axios `client`。
  - `createQASession({title, file_id})`:复用 `createSession` 传 `session_type:"chat"`。
- `frontend/src/store/qaStore.ts`:Zustand(沿用 riskStore 模式),存 messages/currentSession/isStreaming。
- `frontend/src/components/TopbarTabs.tsx`:`TabType` 新增 `'qa'`;`tabs`/`tabRoutes` 数组加 qa 项。
- `frontend/src/lib/Interface.ts`:新增 `QAAskRequest`/`QAMessage` 等类型。

### 6.2 成本看板(新建)
- `frontend/src/app/setting/cost/page.tsx` + `CostDashboard.tsx`:按功能模块/用户的 token 用量与花费图表(复用 signboard 图表组件风格)。
- 后端 `gateway/handler.go` 提供 `/api/gateway/usage_stats`(按 feature/user 聚合)+ `/api/gateway/quotas`。

---

## 七、数据库变更(新增表)

```sql
-- 模型路由
CREATE TABLE llm_routes (
  purpose VARCHAR(32) PRIMARY KEY,          -- review/qa/comparison/chat/embedding
  model_config_id INT NOT NULL,
  params JSON,                               -- temperature/max_tokens 等
  updated_at DATETIME
);

-- 成本日志
CREATE TABLE llm_usage_logs (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  user_id INT NOT NULL, account VARCHAR(64),
  feature VARCHAR(32) NOT NULL,              -- review/qa/comparison/chat
  purpose VARCHAR(32), model_name VARCHAR(128), provider VARCHAR(64),
  prompt_tokens INT, completion_tokens INT, total_tokens INT,
  cost DECIMAL(12,6), latency_ms INT, cache_hit TINYINT(1), status VARCHAR(16),
  created_at DATETIME,
  INDEX idx_user_created (user_id, created_at),
  INDEX idx_feature_created (feature, created_at)
);

-- 配额
CREATE TABLE llm_quotas (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  subject_type VARCHAR(16) NOT NULL,         -- user/role
  subject_id INT NOT NULL,
  feature VARCHAR(32),                      -- NULL=该用户全部功能
  daily_token_limit INT, monthly_token_limit INT,
  UNIQUE KEY uniq_quota (subject_type, subject_id, feature)
);

-- 问答消息
CREATE TABLE qa_messages (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  session_id BIGINT NOT NULL, user_id INT NOT NULL,
  role VARCHAR(16) NOT NULL,                  -- user/assistant/system
  content LONGTEXT NOT NULL, parent_id BIGINT,
  tokens INT DEFAULT 0, created_at DATETIME,
  INDEX idx_session (session_id)
);
```
全部经 GORM AutoMigrate(main.go `db.AutoMigrate` 新增 4 个模型)。

---

## 八、实施顺序(分阶段提交)

1. **阶段 A — 分类对齐**:合同七大类 + 数据迁移 SQL + prompt 模板 + 知识库种子。改动小,先跑通审阅不回归。
2. **阶段 B — 网关核心**:`gateway` 包 + 4 张表 + 流式支持 + 成本/限流/配额/语义缓存;`review` 改接 gateway。先保证非流式 Generate + 成本记录可用。
3. **阶段 C — 合同问答后端**:`qa` 模块 + SSE handler + 多轮 + 绑定合同 RAG。用网关流式。
4. **阶段 D — 问答前端**:`/qa` 页 + TopbarTabs + qaStore + fetch SSE。
5. **阶段 E — 成本看板**:`/setting/cost` + 统计接口。
6. **阶段 F — 收尾**:配置项完善、单元测试(`gateway` 缓存命中/限流/配额扣减;`qa` prompt 构建)、文档更新。

每阶段结束 `go build ./...` + `go vet` + 相关包 `go test`。

---

## 九、测试策略
- `gateway`:语义缓存命中/未命中;限流触发;配额超限;成本记录正确;流式 token 累计。
- `qa`:多轮历史拼接;绑定合同检索命中相关条款;无知识库时降级。
- 复用现有 `rag/reranker_test.go` 风格写表驱动测试。
- 端到端:上传合同→创建问答会话→提问→验证 SSE 流 + 消息落库 + usage 落库。

---

## 十、风险与降级
- 网关任一中间件失败(限流/缓存/配额)不阻断主链路:缓存/限流出错则跳过继续调模型,仅记日志(参考现有 `rag` 多处 `Warn` 兜底)。
- Milvus/embedding 不可用时:语义缓存自动关闭;问答 RAG 退化为关键词检索(现有 `SimpleKeywordIndex` 兜底已就绪)。
- 模型流式不可用:gateway Stream 回退为 Generate 一次性 + 分段回放。
