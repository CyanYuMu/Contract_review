# 合同审阅智能体技术优化 Spec

> Spec ID：CR-ARCH-001
> 版本：v1.2.0-draft
> 状态：Implementing / Phase 0 收尾完成，Phase 1 待启动
> 基线日期：2026-08-12
> 当前代码基线：main@2361cdd，加上本地未提交改动
> 适用范围：Contract_review 前端、后端、RAG、模型网关、合同问答与运行基础设施
> 维护方式：本文档既是目标架构，也是后续实施、验收、ADR 与迭代进度的唯一入口

---

## 1. 文档目的

当前项目已经具备合同上传、文档提取、多 Agent 审阅、混合检索、重排、SSE 流式结果、模型网关、合同问答和前端审阅工作台等能力，不应再按“从零搭建 Demo”的方式规划。

本 Spec 的目标是把现有能力收敛成一套安全、可恢复、可验证、可演进的合同审阅产品系统，重点解决以下问题：

1. 合同、审阅结果和问答内容必须严格按用户或组织隔离。
2. 上传、解析、索引、审阅和问答不能绑定一次浏览器连接。
3. 每个风险结论必须能够定位到合同原文，并追溯到法规或审阅规则。
4. 审阅结果必须可复现、可比较、可人工确认、可安全应用到合同新版本。
5. 审阅与问答应复用同一份结构化合同资产和统一检索能力。
6. 前端应以 URL 和服务端数据为真源，支持刷新恢复、深链、重连和多标签页。
7. 模型调用必须具备隔离、预算、降级、故障转移和端到端成本归因。
8. 技术设计必须提升真实体验，而不是仅增加 Agent、RAG 或工作流术语。

---

## 2. 范围与非目标

### 2.1 本期范围

- 合同上传、存储、解析、OCR、结构化、版本化与索引。
- 合同审阅运行、事件流、Agent/DAG 编排、结果存储和人工决策。
- 法律知识库的版本、发布、检索、引用和快照。
- 基于已上传或已审阅合同的多轮问答。
- 前端审阅工作台、合同问答、引用核验和建议应用。
- 资源权限、审计日志、文件安全和多组织预留。
- 模型路由、限流、配额、缓存、成本和运行可观测性。
- 离线评测、线上质量指标、测试和迁移方案。

### 2.2 明确非目标

- Phase 0 至 Phase 3 不引入 Temporal、Cadence 等重型工作流平台。
- 不把产品改造成 deer-flow 式通用 Super-Agent 平台。
- 不复制 Shannon 的多服务部署形态；优先保持 Go 模块化单体。
- 不在首期实现 WeKnora 完整的跨租户共享空间。
- 不允许 Agent 自由执行任意代码、Shell 或外部工具。
- 不以“增加 Agent 数量”作为架构升级指标。
- 不承诺 AI 结果替代律师或法务的最终判断。

---

## 3. 阅读范围与参考基线

### 3.1 当前项目重点代码

后端重点模块：

- app/internal/cmd/main.go
- app/internal/contract
- app/internal/session
- app/internal/review
- app/internal/agent
- app/internal/rag
- app/internal/knowledge
- app/internal/qa
- app/internal/gateway
- app/internal/modelconfig
- app/internal/riskconfig

前端重点模块：

- frontend/src/app/review
- frontend/src/app/qa
- frontend/src/components/ReviewPanel.tsx
- frontend/src/components/ContractResult.tsx
- frontend/src/components/review/RiskCard.v2.tsx
- frontend/src/components/qa
- frontend/src/lib/api
- frontend/src/store

已有架构文档：

- docs/contract-review-agent-architecture.md
- docs/review-pipeline-optimization-plan.md
- docs/implementation-plan.md

### 3.2 参考项目与可迁移设计

| 项目 | 重点参考 | 建议迁移 | 不建议直接复制 |
| --- | --- | --- | --- |
| WeKnora | tenant RBAC、资源归属、知识处理任务、解析 span、RAG 进度、引用 UI | organization/owner 双层资源守卫、异步知识处理、处理配置快照、引用侧栏、请求代次保护 | 首期完整共享空间和复杂跨租户协作 |
| Shannon | 工作流分层、持久事件日志、Redis Streams、幂等、预算、控制信号、部分结果降级 | Orchestrator/Strategy/Node 分层、run + seq 事件、幂等键、取消、预算、degraded 结果 | 直接引入 Temporal 和多微服务 |
| deer-flow | thread/run 分离、checkpoint、Last-Event-ID、断线策略、并发策略、事件存储 | session/thread 与 run 解耦、可恢复 SSE、事件重放、on_disconnect、multitask_strategy | 通用 Agent Harness、sandbox、skills 运行时 |

参考文件示例：

- WeKnora/docs/RBAC说明.md
- WeKnora/internal/middleware/access.go
- WeKnora/internal/middleware/kb_access.go
- WeKnora/internal/types/knowledge_process.go
- WeKnora/internal/application/service/knowledge_span_tracker.go
- WeKnora/frontend/src/views/chat/components/RagPipelineProgress.vue
- WeKnora/frontend/src/utils/citationMarkdown.ts
- Shannon/docs/multi-agent-workflow-architecture.md
- Shannon/docs/token-budget-tracking.md
- Shannon/docs/task-history-and-timeline.md
- Shannon/go/orchestrator/internal/db/event_log.go
- Shannon/go/orchestrator/cmd/gateway/internal/middleware/idempotency.go
- Shannon/go/orchestrator/internal/degradation/partial_results.go
- deer-flow/backend/app/gateway/routers/thread_runs.py
- deer-flow/backend/packages/harness/deerflow/runtime/runs/schemas.py
- deer-flow/backend/packages/harness/deerflow/runtime/events/store
- deer-flow/frontend/src/components/workspace/chats/use-thread-chat.ts
- deer-flow/frontend/src/core/sidecar/reference-state.ts

---

## 4. 当前能力盘点

### 4.1 已有且应复用的能力

| 能力 | 当前状态 | 优化方向 |
| --- | --- | --- |
| Go/Hertz + MySQL + Redis | 已具备 | 保持模块化单体，补资源守卫和 durable run |
| Next.js 15 + React 19 + Ant Design + Zustand | 已具备 | 引入明确的 server state/client state 边界 |
| DOCX/PDF/TXT 文本提取 | 部分具备 | 升级为异步解析、OCR、结构坐标和版本资产 |
| ClauseAgent → CandidateRiskAgent → SuggestionAgent → QualityGate | 已具备 | 改造成有限状态 DAG，明确确定性节点与 LLM 节点 |
| 关键词、向量、BM25、RRF、rerank、MMR、分层检索 | 后端已有 | 收敛为统一 RetrievalService，供审阅和 QA 复用 |
| 风险配置同步知识库 | 已具备 | 增加知识发布流、版本和 snapshot |
| SSE 审阅进度与风险流 | 已具备 | 升级为持久事件、重连和重放 |
| 模型路由、限流、配额、成本、语义缓存 | 已具备 | 修复隔离与原子性，补健康路由、预算和归因 |
| 合同 QA、多轮历史、SSE | 已具备 | 复用合同结构索引，增加引用、停止、重生成和恢复 |
| 文档预览、风险定位、建议应用 | 已具备 | 使用结构化 range + base hash，生成新合同版本 |

### 4.2 当前真实调用链

~~~mermaid
flowchart LR
    U["浏览器上传合同"] --> H["UploadContractFile"]
    H --> F["按原文件名保存到本地"]
    F --> X["同步提取全文"]
    X --> M["LLM 解析合同元数据"]
    M --> C["contracts"]
    C --> S["session"]
    S --> R["StartReviewTask SSE 请求"]
    R --> G["使用请求 ctx 启动 goroutine"]
    G --> O["共享 ReviewOrchestrator"]
    O --> A1["ClauseAgent"]
    O --> A2["CandidateRiskAgent"]
    O --> A3["SuggestionAgent"]
    O --> QG["QualityGate"]
    A2 --> RR["review_results 扁平文本"]
    RR --> LS["Zustand + localStorage"]

    C --> QA["QA 提问"]
    QA --> RX["每次重新读取并提取合同"]
    RX --> CK["固定 600 字切块 + 内存关键词索引"]
    QA --> KK["一次懒加载的知识关键词索引"]
    CK --> LLM["Gateway StreamChat"]
    KK --> LLM
~~~

该链路已经能工作，但任务、证据、合同资产和浏览器连接耦合过紧，导致安全、并发、恢复、审计与扩展问题。

---

## 5. 问题与优先级矩阵

优先级定义：

- P0：存在安全泄漏、数据错误、跨会话污染或核心流程不可恢复风险，必须先处理。
- P1：直接影响结果可信度、用户体验或规模化运行，应在核心重构阶段完成。
- P2：工程效率、运营能力或高级体验优化。

| ID | 优先级 | 问题 | 当前证据 | 影响 | 目标阶段 |
| --- | --- | --- | --- | --- | --- |
| SEC-01 | P0 | 合同资源级越权 | contract/handler.go:147-216 的更新、删除、下载只按合同 ID；service/repo 未统一带 owner scope | 任意登录用户可能访问他人合同 | Phase 0 |
| SEC-02 | P0 | review、result、配置、网关管理接口缺少统一角色守卫 | cmd/main.go:220-299 大量接口仅挂登录鉴权 | 数据泄漏、普通用户修改全局配置 | Phase 0 |
| SEC-03 | P0 | 文件使用原始文件名落盘 | contract/handler.go:53-70 | 重名覆盖、路径风险、不可安全迁移对象存储 | Phase 0 |
| SEC-04 | P0 | 静态文件公开暴露 | cmd/main.go:196-197；contract/paths.go:59-65 | 绕过合同资源授权直接读取文件 | Phase 0 |
| SEC-05 | P0 | 缺少大小、魔数、MIME、病毒和压缩炸弹校验 | contract/handler.go:44-70 | 服务耗尽、恶意文件风险 | Phase 0 |
| RUN-01 | P0 | 共享 Orchestrator 保存可变 callback | agent/orchestrator.go:28-30、66-73；review/service.go:638-658 | 并发审阅回调互相覆盖、SSE 串流、data race | Phase 0 |
| RUN-02 | P0 | 审阅生命周期绑定 SSE 请求 ctx | review/handler.go:109-115 | 刷新或断网后运行不可恢复，服务重启丢任务 | Phase 2 |
| RUN-03 | P0 | 重审删除旧任务和旧结果 | review/service.go:148-160 | 无历史、无法比较、无法审计 | Phase 2 |
| RUN-04 | P1 | SSE 无 cursor 语义和事件重放 | review/handler.go、frontend SSE 客户端 | 无法断点续传，网络抖动造成不确定状态 | Phase 2 |
| DATA-01 | P0 | review_results 过度扁平 | review/model.go:25-37 | 引用、置信度、模型、版本、定位信息无法结构化使用 | Phase 2 |
| DATA-02 | P1 | 合同只有单记录，没有不可变版本 | contract/model.go:5-22 | 建议应用、重新上传、比较和问答无法固定内容版本 | Phase 1 |
| INGEST-01 | P1 | 上传同步解析和 LLM 元数据提取 | contract/service.go:118-205 | 上传长时间等待，阶段失败不可重试 | Phase 1 |
| INGEST-02 | P1 | PDF/DOCX 解析缺少稳定结构坐标，扫描件能力弱 | 当前 ExtractText 链路 | 风险定位和引用可靠性不足 | Phase 1 |
| INGEST-03 | P1 | 前端只允许 DOCX，后端允许 PDF/DOCX/TXT | ContractUploader.tsx:218；contract/handler.go:53-57 | 产品能力不一致 | Phase 1 |
| RAG-01 | P1 | QA 每次重新提取、切块并构建内存索引 | qa/service.go:97-124、204-249 | 延迟高、结构丢失、无法复用审阅资产 | Phase 1/3 |
| RAG-02 | P1 | QA 知识检索仅关键词且加载后不刷新 | qa/service.go:21-30、62-84 | 新法规不生效，召回质量低 | Phase 3 |
| RAG-03 | P1 | contractType 未进入实际过滤 | qa/service.go:181-200 | 不同合同类型知识混检 | Phase 3 |
| RAG-04 | P1 | 引用只是拼进 Prompt 的文本 | qa/service.go:192-235 | 前端无法点击、核验、审计 | Phase 3/4 |
| RAG-05 | P1 | 知识库缺少发布状态、生效时间和 snapshot | knowledge 当前模型与同步方式 | 运行不可复现，可能引用失效法规 | Phase 3 |
| QA-01 | P0 | QA 语义缓存未按用户、合同、版本隔离 | gateway/cache.go:41-47、50-108 | 相同问题可能命中其他合同的答案 | Phase 0 |
| QA-02 | P1 | QA 流无 AbortController、会话切换代次保护 | frontend/lib/api/qa.ts:11-101；QAPanel.tsx:148-190 | 旧流可能写入新会话 | Phase 0/4 |
| QA-03 | P1 | 流自然结束但没收到 end 时不报错 | frontend/lib/api/qa.ts:78-101 | 断流被误认为完成 | Phase 0 |
| QA-04 | P1 | QA 无合同预览和引用联动 | frontend/components/qa/QAPanel.tsx | 无法核验回答依据 | Phase 4 |
| FE-01 | P1 | URL 不是工作台真源 | ReviewPageContent.tsx:37-77 及多处 localStorage | 刷新、深链、多标签页、跨设备状态不可靠 | Phase 4 |
| FE-02 | P1 | 大组件混合多种职责 | ReviewPageContent 743 行、RiskCard.v2 1111 行、ContractResult 1860 行 | 难测试、竞态多、修改风险高 | Phase 4 |
| FE-03 | P1 | API response shape 和 SSE parser 分散 | QAPanel.tsx:46-57、frontend/src/lib/api | 错误处理不一致，类型约束弱 | Phase 4 |
| FE-04 | P1 | 建议应用依赖文本查找和 DOM 状态 | RiskCard/ContractResult/editor 链路 | 重复文本可能替换错误，无冲突检测 | Phase 2/4 |
| GW-01 | P0 | feature-specific quota 查询错误 | gateway/repo.go:38-44 固定查询 feature="" | 细分配额可能永不生效 | Phase 0 |
| GW-02 | P1 | 配额检查与实际扣减非原子 | gateway/quota.go:15-88 | 并发请求可一起越过限额 | Phase 5 |
| GW-03 | P1 | 语义缓存 HGETALL 全量扫描 | gateway/cache.go:111-145 | 缓存规模增长后延迟和 Redis 压力上升 | Phase 5 |
| GW-04 | P1 | 缺少模型健康路由、熔断和自动降级 | gateway 当前路由 | 上游故障直接影响审阅与 QA | Phase 5 |
| GW-05 | P1 | 用量日志缺 run/step/prompt/snapshot 关联 | gateway usage model | 成本无法归因到具体审阅阶段 | Phase 2/5 |
| QUALITY-01 | P1 | QualityGate 主要依赖另一次 LLM 打分 | agent/quality_gate.go 与 orchestrator | 无确定性质量保证，无法用 gold set 校准 | Phase 3 |
| QUALITY-02 | P1 | 总体风险算法不统一 | handler 和 Agent report 使用不同逻辑 | 同一结果可能展示不同整体风险 | Phase 2 |

### 5.1 问题实施状态（v1.2.0-draft）

- 已完成首批代码修复：SEC-01、SEC-02、SEC-03、SEC-04、RUN-01、QA-01、QA-02、QA-03、GW-01。
- 已完成 Phase 0 剩余架构项：统一 `ResourceScope`（`app/internal/middleware/scope.go`，含 Redis 缓存 identity resolver），重构 5 个模块（contract/review/qa/session/comparison）handler 和 service 的 identity 解析路径，消除 4 份重复 `ResolveUserID` 实现；IDOR 集成测试覆盖合同、会话、审阅、QA 和比对跨用户隔离（`idor_integration_test.go`，build tag `//go:build integration`）。
- 部分完成：SEC-05 已有大小、类型、魔数、DOCX ZIP 结构、解压上限和路径穿越防护，但病毒扫描仍未接入。
- 临时缓解但未关闭：RUN-02 已使进程内审阅不随浏览器断线立即取消，但服务重启恢复、事件重放和跨实例接管仍未实现，因此问题继续保持 Open 并归入 Phase 2。
- 仍保持 Open：RUN-03、RUN-04、DATA-01/02、INGEST、RAG、QA-04、FE、GW-02 至 GW-05、QUALITY 系列。
- Phase 0 完整退出条件：仅病毒扫描（SEC-05）待 ClamAV 或云端服务组件接入后关闭；其余代码项均已通过编译、单元测试和 go vet 验证。

---

## 6. 总体设计原则

1. 服务端是真源。URL 只携带资源标识，浏览器本地状态不作为业务事实。
2. 合同内容不可变。编辑或采纳建议产生新 contract_version。
3. session/thread 表示长期上下文，run 表示一次可独立重试和审计的执行。
4. 所有长任务先持久化，再执行；事件先有顺序号，再输送到 SSE。
5. 编排器保持无状态；运行态、callback、预算和取消信号属于 run context。
6. RAG 返回结构化证据，不返回只能拼进 Prompt 的匿名文本。
7. 知识库先发布再使用；每个 run 固定 knowledge_snapshot。
8. LLM 负责语义判断，确定性代码负责权限、定位、格式、预算和完整性校验。
9. 所有缓存必须声明可见域、内容版本、模型版本、Prompt 版本和失效条件。
10. 优先演进式替换，保持旧接口和旧数据在迁移窗口可读。
11. 技术升级必须有产品指标、质量指标或运维指标支撑。

---

## 7. 目标架构

~~~mermaid
flowchart TB
    FE["Next.js 合同工作台"] --> API["Hertz API / Resource Guard"]

    subgraph Asset["合同资产面"]
        UP["Upload Service"]
        IR["Ingest Run"]
        PARSE["Parser / OCR / Structurer"]
        BLOCK["Contract Version + Blocks + Clauses"]
        CINDEX["Contract Search Index"]
        OBJ["Private Object Storage"]
        UP --> OBJ
        UP --> IR
        IR --> PARSE
        PARSE --> BLOCK
        BLOCK --> CINDEX
    end

    subgraph Knowledge["知识面"]
        KDOC["Knowledge Document / Version"]
        KPUB["Review + Publish"]
        KSNAP["Knowledge Snapshot"]
        KINDEX["Hybrid Index"]
        KDOC --> KPUB
        KPUB --> KSNAP
        KSNAP --> KINDEX
    end

    subgraph Execution["审阅执行面"]
        RRUN["Review Run"]
        WORKER["Durable Worker"]
        DAG["Review Strategy DAG"]
        EVENT["Run Event Store"]
        FIND["Finding / Evidence / Suggestion"]
        RRUN --> WORKER
        WORKER --> DAG
        DAG --> FIND
        DAG --> EVENT
    end

    subgraph QAPlane["合同问答面"]
        THREAD["QA Thread"]
        QRUN["QA Run"]
        QPLAN["Query Rewrite / Dual Retrieval"]
        ANSWER["Answer + Citations"]
        THREAD --> QRUN
        QRUN --> QPLAN
        QPLAN --> ANSWER
    end

    subgraph Shared["共享能力"]
        RET["Unified Retrieval Service"]
        GW["LLM Gateway"]
        OBS["Trace / Metrics / Audit"]
    end

    API --> Asset
    API --> RRUN
    API --> THREAD
    API --> Knowledge
    DAG --> RET
    QPLAN --> RET
    RET --> CINDEX
    RET --> KINDEX
    DAG --> GW
    QPLAN --> GW
    API --> EVENT
    GW --> OBS
    EVENT --> OBS
~~~

### 7.1 部署策略

第一阶段继续使用模块化单体：

- 一个 Hertz API 进程。
- 一个或多个同代码库 worker 进程，也可在开发环境中以内嵌 worker 启动。
- MySQL 作为业务事实、run 和 event 的持久化存储。
- Redis 用于限流、配额、锁、短期事件加速和可选 Streams。
- 私有文件系统先兼容，生产建议迁移 MinIO/S3。
- Milvus 或当前向量实现继续承载向量索引。

满足以下条件后再评估独立工作流平台：

- 单日审阅 run 数量或并发已明显超过数据库 worker 模型能力。
- 工作流包含跨小时人工等待、多级审批或大量外部回调。
- 已经有独立平台团队维护工作流基础设施。

---

## 8. 领域模型与数据设计

### 8.1 身份与资源域

#### organizations

- id
- name
- status
- created_at

#### organization_members

- organization_id
- user_id
- role：owner/admin/reviewer/member/viewer
- status
- created_at

#### 资源统一字段

以下资源逐步增加：

- organization_id
- owner_user_id
- visibility：private/organization
- created_by
- updated_by

资源查询必须使用 ResourceScope，不允许 handler 先按 ID 查询后再做可选校验。

### 8.2 合同资产域

#### contracts

表示业务合同聚合，不直接表示某一份文件内容：

- id
- organization_id
- owner_user_id
- title
- contract_type_id
- current_version_id
- party_a
- party_b
- amount
- status
- created_at
- updated_at

#### contract_versions

每次上传、编辑或采纳建议都创建不可变版本：

- id
- contract_id
- version_no
- parent_version_id
- source：upload/editor/suggestion_apply
- object_key
- original_filename
- media_type
- size_bytes
- sha256
- parser_version
- status：uploaded/processing/ready/failed
- created_by
- created_at

唯一约束：

- unique(contract_id, version_no)
- unique(contract_id, sha256) 可按产品策略选择是否去重

#### contract_blocks

持久化 canonical document：

- id
- contract_version_id
- block_no
- block_type：title/paragraph/table/table_cell/list/header/footer
- parent_block_id
- text
- page_no
- paragraph_no
- char_start
- char_end
- bbox_json
- content_hash
- metadata_json

#### contract_clauses

- id
- contract_version_id
- clause_key，例如 article-3-2
- title
- clause_path
- category
- parent_clause_id
- start_block_id
- end_block_id
- text
- content_hash
- metadata_json

#### contract_ingest_runs / contract_ingest_steps

记录 validating、extracting、ocr、structuring、indexing 各阶段状态、错误和重试次数。

### 8.3 审阅运行域

#### review_runs

- id
- organization_id
- owner_user_id
- session_id
- contract_id
- contract_version_id
- run_no
- status：queued/running/succeeded/failed/cancelled/timed_out/interrupted
- stance
- intensity
- description
- strategy_version
- prompt_snapshot_id
- knowledge_snapshot_id
- model_route_snapshot_json
- retrieval_config_snapshot_json
- token_budget
- token_used
- degraded
- error_code
- error_message
- idempotency_key
- started_at
- completed_at
- created_at

#### review_run_steps

- id
- run_id
- step_key
- step_type
- status
- attempt
- input_hash
- output_ref
- token_budget
- token_used
- latency_ms
- error_code
- started_at
- completed_at

#### review_events

- id：全局事件 ID，可使用 ULID
- run_id
- seq：run 内单调递增
- type
- payload_json
- created_at

唯一约束：

- unique(run_id, seq)
- unique(id)

#### review_findings

- id
- run_id
- clause_id
- finding_key：稳定去重键
- risk_type
- severity
- title
- description
- original_text
- confidence
- verified
- materiality
- status：open/accepted/rejected/modified/resolved
- created_at

#### finding_evidences

- id
- finding_id
- evidence_type：contract/legal_rule/risk_config/model_trace
- source_id
- source_version_id
- chunk_id
- article
- quote
- page_no
- block_id
- char_start
- char_end
- retrieval_score
- rerank_score
- support_score
- source_url
- metadata_json

#### review_suggestions

- id
- finding_id
- target_contract_version_id
- target_block_id
- target_start
- target_end
- base_text
- base_hash
- replacement_text
- rationale
- patch_json
- status：proposed/applied/conflicted/rejected
- applied_version_id

#### review_decisions

- id
- finding_id
- actor_user_id
- action
- comment
- previous_state_json
- next_state_json
- created_at

### 8.4 知识域

#### knowledge_documents

- id
- organization_id，可为空表示平台公共知识
- owner_user_id
- title
- jurisdiction
- document_type
- source_url
- status：draft/reviewed/published/retired

#### knowledge_versions

- id
- document_id
- version_no
- effective_from
- effective_to
- published_at
- source_hash
- supersedes_version_id
- reviewed_by
- parser_version

#### knowledge_chunks

- id
- version_id
- article
- title_path
- content
- embedding_version
- metadata_json

#### knowledge_snapshots / knowledge_snapshot_items

snapshot 是一次审阅可复现的知识集合。run 创建时只引用已发布且当时生效的知识版本。

### 8.5 QA 域

现有 sessions 可演进为 thread 概念，避免立即重建全部会话表。

#### qa_runs

- id
- session_id
- user_message_id
- assistant_message_id
- contract_version_id
- knowledge_snapshot_id
- status
- rewritten_query
- history_hash
- prompt_version
- model_snapshot_json
- token_used
- started_at
- completed_at

#### qa_message_citations

- id
- message_id
- ordinal
- evidence_type
- source_id
- source_version_id
- chunk_id
- quote
- page_no
- block_id
- char_start
- char_end
- score

---

## 9. 资源权限与文件安全

### 9.1 ResourceGuard

新增统一的资源访问接口：

    type ResourceScope struct {
        OrganizationID uint64
        UserID         uint64
        Role           string
    }

    type ResourceGuard interface {
        CanReadContract(ctx, scope, contractID) error
        CanWriteContract(ctx, scope, contractID) error
        CanReadRun(ctx, scope, runID) error
        CanManageKnowledge(ctx, scope) error
        CanManageGateway(ctx, scope) error
    }

Repository 层方法应采用以下形式：

    GetContractByID(ctx, scope, contractID)
    DeleteContract(ctx, scope, contractID)
    ListReviewRuns(ctx, scope, filter)

禁止继续使用只接收 contractID 或 runID 的公共 service 方法。

### 9.2 最小角色矩阵

| 操作 | owner | admin | reviewer | member | viewer |
| --- | --- | --- | --- | --- | --- |
| 查看组织合同 | 是 | 是 | 是 | 按 visibility | 是 |
| 上传合同 | 是 | 是 | 是 | 是 | 否 |
| 发起审阅/QA | 是 | 是 | 是 | 是 | 否 |
| 接受或拒绝风险 | 是 | 是 | 是 | 自己资源 | 否 |
| 管理知识发布 | 是 | 是 | 可审核 | 否 | 否 |
| 管理模型、路由、配额 | 是 | 是 | 否 | 否 | 否 |

Phase 0 即使暂不提供组织 UI，也应先加入 owner_user_id 和服务端资源校验。organization_id 可先使用默认组织回填。

### 9.3 文件安全

上传流程必须满足：

1. 原始文件名只作为展示元数据，不作为 object key。
2. object key 使用 organization/contract/version/UUID。
3. 扩展名统一转小写，并同时校验 MIME 和文件魔数。
4. 配置默认最大文件大小，例如 50 MB。
5. DOCX 按 ZIP 结构限制解压文件数、单文件大小和总展开大小。
6. PDF 设置页数和解析超时限制。
7. 扫描文件进入隔离 OCR 队列。
8. 可接入 ClamAV 或云端病毒扫描；扫描完成前状态为 quarantined。
9. 删除 h.Static("/api/static", uploadDir) 的公开访问方式。
10. 下载使用受鉴权 API 或短时签名 URL。
11. 数据库只保存 object key，不保存可直接拼接的服务器绝对路径。
12. 文件日志不得输出全文、密钥或带签名的长期 URL。

### 9.4 审计日志

至少记录：

- contract.upload/read/download/delete
- contract.version.created
- review.run.created/cancelled/retried
- review.finding.accepted/rejected/modified
- suggestion.applied/conflicted
- knowledge.published/retired
- gateway.route.updated
- quota.updated
- access.denied

---

## 10. 合同 Ingestion 设计

### 10.1 状态机

~~~mermaid
stateDiagram-v2
    [*] --> uploaded
    uploaded --> validating
    validating --> extracting
    extracting --> ocr: 需要 OCR
    extracting --> structuring: 可直接提取
    ocr --> structuring
    structuring --> indexing
    indexing --> ready
    validating --> failed
    extracting --> failed
    ocr --> failed
    structuring --> failed
    indexing --> failed
    failed --> validating: retry
~~~

### 10.2 API 行为

上传 API 只负责：

- 鉴权与资源范围确认。
- 文件流式写入隔离区。
- 基础校验和 sha256 计算。
- 创建 contract、contract_version 和 ingest_run。
- 返回 202、contract_id、version_id、ingest_run_id。

前端随后订阅 ingest 事件或轮询状态，不再等待 LLM 元数据解析完成。

### 10.3 Canonical Document

解析结果必须一次生成、长期复用：

- 审阅按 clause/block 读取。
- QA 直接检索 contract_version 的索引。
- 建议应用使用 block/range。
- 合同对比使用稳定 block/clause ID。
- 引用跳转使用 page/block/offset。

Parser 需要输出统一中间格式，DOCX、PDF、TXT 和 OCR 只是不同 adapter。

### 10.4 失败与重试

- 每个 ingest step 记录 error_code 和可重试性。
- 解析失败不要求重新上传文件。
- 重试默认复用原 object 和 sha256。
- parser_version 变化时允许 reprocess 生成新的解析 artifact，但不得覆盖旧 artifact。
- 部分页面 OCR 失败时允许标记 degraded，并在 UI 展示缺失页。

---

## 11. Durable Review Run

### 11.1 session 与 run 分离

- session：合同审阅工作区的长期上下文。
- run：某个合同版本、立场、尺度、知识快照和策略版本下的一次执行。
- 同一 session 可以存在多个 run。
- 历史 run 不删除，可以比较差异。
- UI 默认展示 latest successful run，也允许查看失败 run 的部分结果和日志。

### 11.2 Run 状态机

~~~mermaid
stateDiagram-v2
    [*] --> queued
    queued --> running: worker claim
    queued --> cancelled: cancel
    running --> succeeded
    running --> failed
    running --> timed_out
    running --> interrupted
    running --> cancelled: cancel signal
    failed --> queued: retry as new attempt
    timed_out --> queued: retry as new attempt
    interrupted --> queued: resume/retry
~~~

### 11.3 并发策略

创建 run 时支持 multitask_strategy：

- enqueue：默认。相同 contract_version 的新 run 排队。
- reject：已有 running run 时返回 409。
- interrupt：向旧 run 发取消信号，再创建新 run。

首期建议：

- 审阅默认 enqueue。
- 用户点击“重新审阅”时显式选择 enqueue 或 interrupt。
- 同一个 run 只能被一个 worker claim，使用数据库行锁或租约。

### 11.4 断线策略

- review 默认 on_disconnect=continue。
- QA 首期默认 on_disconnect=cancel，完成 qa_run 持久化后可切换 continue。
- SSE 连接只消费 review_events，不承载工作本身。
- 浏览器刷新后使用 Last-Event-ID 继续。

### 11.5 事件协议

事件类型建议：

| 类型 | 用途 |
| --- | --- |
| run.created | run 已创建 |
| run.started | worker 开始执行 |
| step.started | 节点开始 |
| step.progress | 节点阶段进度 |
| retrieval.completed | 检索摘要与 trace |
| finding.created | 新风险 |
| finding.updated | 补齐建议或证据 |
| step.completed | 节点完成 |
| run.degraded | 存在部分失败 |
| run.completed | 正常完成 |
| run.failed | 失败 |
| run.cancelled | 已取消 |
| heartbeat | 保活 |

SSE 示例：

    id: 01J...
    event: finding.created
    data: {"run_id":123,"seq":18,"finding_id":456}

客户端发送：

    Last-Event-ID: 01J...

服务端行为：

1. 从 event store 查询该 ID 后的事件。
2. 按 seq 重放。
3. 重放完成后切换为实时订阅。
4. 每 15 秒发送 heartbeat。
5. 未知或已过期 cursor 返回可识别错误，客户端回退为按 run snapshot 全量恢复。

### 11.6 轻量 durable worker

首期实现：

- review_runs 作为任务表。
- worker 使用 SELECT ... FOR UPDATE SKIP LOCKED 或租约字段领取任务。
- step 输出先写业务表，再在同一事务写 event/outbox。
- 独立 publisher 将 outbox 推到 Redis Streams，用于低延迟 SSE。
- 即使 Redis 不可用，SSE 仍可从 MySQL 轮询事件恢复。

该方案吸收 Shannon 的事件日志和 deer-flow 的 run/event 设计，但避免立即引入 Temporal。

### 11.7 幂等

创建 run、发送 QA 消息、应用建议均支持 Idempotency-Key。

幂等维度：

- organization_id
- user_id
- HTTP method
- route
- normalized body hash

重复请求返回第一次的资源 ID 和结果，不再次扣费或生成新版本。

---

## 12. Review Strategy 与 Agent 设计

### 12.1 三层职责

1. Orchestrator：装载 snapshot、调度 DAG、处理取消、预算、事件和失败策略。
2. Strategy：定义不同合同类型、立场、尺度对应的步骤和依赖。
3. Node：可复用的确定性或 LLM 执行节点。

### 12.2 推荐 DAG

~~~mermaid
flowchart LR
    V["Validate Inputs"] --> C["Load Clauses"]
    C --> P["Plan Review Scope"]
    P --> K["Retrieve Risk Rules"]
    K --> D["Detect Candidate Risks"]
    D --> E["Retrieve Legal Evidence"]
    E --> VR["Deterministic Verify"]
    VR --> S["Generate Suggestions"]
    S --> Q["Quality Checks"]
    Q --> A["Aggregate Overall Risk"]
    A --> O["Persist Findings + Events"]
~~~

### 12.3 节点分类

确定性节点：

- 权限与 snapshot 完整性检查。
- clause/block 定位。
- 风险等级枚举校验。
- evidence 引用存在性和有效期校验。
- original_text 与 target range 一致性校验。
- finding 去重。
- 建议 base_hash 生成。
- token budget 和超时检查。
- 整体风险聚合。

LLM 节点：

- 复杂条款语义分类。
- 风险候选判断。
- 证据与风险的支持关系判断。
- 修改建议生成。
- 必要的查询改写。

### 12.4 无状态 Orchestrator

必须移除实例字段 onProgress 和 onFinding。

目标接口：

    type ReviewEventSink interface {
        Append(ctx context.Context, event ReviewEvent) error
    }

    type ReviewRunContext struct {
        RunID       uint64
        Scope       ResourceScope
        Budget      Budget
        Snapshot    ReviewSnapshot
        EventSink   ReviewEventSink
        Cancel      CancellationToken
    }

    func (o *ReviewOrchestrator) Execute(
        ctx context.Context,
        runCtx ReviewRunContext,
        input ReviewInput,
    ) error

共享的 Orchestrator、Retriever 和模型 client 只能包含只读配置或并发安全依赖。

### 12.5 部分结果与降级

参考 Shannon 的 partial results：

- 某一条款失败不应让整份合同结果全部消失。
- run 标记 degraded=true。
- failed step、缺失 clause、未验证 finding 必须显式展示。
- 未验证 finding 不得伪装成已确认法律风险。
- 用户可以仅重试失败步骤或指定条款。

### 12.6 预算与控制信号

每个 run 记录：

- 总 token budget。
- 每个 step 的预算和已用量。
- review/QA/model/embedding/rerank 分桶。
- cancel、pause、resume 信号。

首期必须实现 cancel；pause/resume 可在 durable worker 稳定后实现。

### 12.7 质量门

QualityGate 改为两层：

第一层确定性校验：

- finding 必须关联 clause。
- original_text 必须能在 contract_version 中定位。
- severity 合法。
- suggestion 非空且 base_hash 匹配。
- legal evidence 必须来自 snapshot。
- 引用法规必须处于有效期或明确标注历史状态。

第二层语义评估：

- 证据是否支持风险。
- 风险描述是否过度推断。
- 建议是否保持交易意图。
- 是否存在相似重复 finding。

只对低置信度、高金额、高严重度或未通过校验的条款定向重审，不做全量空转 Reflection。

### 12.8 整体风险模型

统一由一个 RiskAggregator 计算，前后端不得各自推导。

建议公式：

    finding_score =
        severity_weight
        × confidence
        × verification_factor
        × materiality

其中：

- severity_weight：high=1.0，medium=0.55，low=0.2。
- verification_factor：verified=1.0，unverified=0.5。
- materiality：结合合同金额、立场、关键条款类型，限制在 0.5 至 1.5。

总分采用 capped aggregation，避免大量低风险简单相加导致“高风险”。API 同时返回 overall_level、overall_score 和 explain_breakdown。

---

## 13. 可验证 RAG 与证据链

### 13.1 Unified RetrievalService

审阅和 QA 必须调用同一套服务：

    RetrieveContract(ctx, contractVersionID, query, filters)
    RetrieveKnowledge(ctx, snapshotID, query, filters)
    RetrieveReviewPolicy(ctx, organizationID, contractType, stance)

服务内部复用现有：

- keyword/BM25
- vector search
- RRF
- reranker
- MMR
- hierarchical retrieval

禁止 QA 再创建独立 SimpleKeywordIndex。

### 13.2 双路检索

合同 QA 的检索不是把两种文本混在一个 top-k：

1. 合同路：从 contract_version 的 clause/block 索引检索事实。
2. 法律路：从 knowledge_snapshot 检索法规、司法解释、内部规则。
3. Query planner 判断问题类型：
   - 合同事实问题优先合同路。
   - 合法性或风险问题同时检索两路。
   - 纯摘要问题按结构提取，不需要法律路。
4. 最终 answer planner 按来源类型组织引用。

### 13.3 检索过滤

至少支持：

- organization_id / visibility
- contract_version_id
- knowledge_snapshot_id
- contract_type
- stance
- jurisdiction
- effective_at
- document_type
- risk_category
- language

### 13.4 Retrieval Trace

每次检索记录：

- query_original
- query_rewritten
- index_version
- filters
- candidate IDs 和原始分数
- RRF 分数
- rerank 分数
- MMR 选择结果
- top-k
- latency
- embedding/model version
- 最终被 Prompt 使用的 evidence IDs

前端默认只展示简化的“检索了哪些来源”，管理员可展开完整 trace。

### 13.5 Citation 结构

模型输出使用 citation token 或结构化输出引用 evidence_id，不直接生成任意来源名称。

服务端在输出前验证：

- evidence_id 是否属于当前 run snapshot。
- quote 是否能在 source chunk 中找到。
- page/block/range 是否有效。
- 法规版本和生效日期是否匹配。

答案渲染时：

- 合同引用点击后定位到原文。
- 法规引用在右侧 sidecar 展示名称、条款、版本、生效时间和原文。
- 无证据结论显示“基于一般推理，未找到直接依据”。

### 13.6 知识发布

知识状态：

    draft → reviewed → published → retired

只有 published 版本可进入新 snapshot。旧 snapshot 保留，历史 run 不受后续知识变更影响。

风险配置不再“保存即直接覆盖运行知识”，而是：

1. 保存草稿。
2. 解析、切块和索引。
3. 审核。
4. 发布新知识版本。
5. 生成或更新可用 snapshot。

---

## 14. 合同问答设计

### 14.1 产品语义

- 用户先选择一份合同或从审阅工作台进入 QA。
- QA 默认绑定当前 contract_version，而不是模糊绑定 contract。
- 如果合同产生新版本，线程提示“当前对话基于 vN”，用户可选择迁移到最新版本。
- 审阅结果可作为附加上下文，但必须与合同事实和法规证据分开标识。

### 14.2 问答执行链

1. 校验 session、contract 和 version 归属。
2. 持久化 user message 和 qa_run。
3. 解析最近多轮中的指代，生成 rewritten_query。
4. 进行合同路和法律路检索。
5. 生成答案和 citation IDs。
6. 逐事件持久化 delta 或至少持久化 checkpoint。
7. 完成后保存 assistant message、citations、usage。
8. 断流时标记 cancelled/failed，不能静默成功。

### 14.3 缓存策略

默认禁用 QA 最终答案语义缓存，优先缓存以下安全中间结果：

- query embedding
- contract chunk retrieval
- knowledge retrieval
- rerank result

如果未来启用答案缓存，cache scope 必须包含：

- organization_id
- user visibility scope
- contract_version_id / content hash
- knowledge_snapshot_id
- prompt_version
- model/version
- retrieval_config_hash
- history_hash
- normalized query

任一维度变化即不得复用。

### 14.4 QA 前端体验

三栏布局：

- 左栏：合同与 QA 会话。
- 中栏：消息、生成状态、引用标记。
- 右栏：合同原文或法规引用 sidecar。

必须支持：

- 停止生成。
- 重新生成。
- 复制。
- 有用/无用反馈。
- 点击引用跳转。
- 查看当前绑定合同版本。
- 查看简化检索过程。
- 网络重连与错误重试。
- 会话 URL 化。

### 14.5 前端竞态保护

- 每次 ask 创建 AbortController。
- 切换 session 时取消旧 controller。
- store 保存 activeRunID 和 generation。
- onDelta 只有 runID 和 generation 都匹配时才能写入。
- client_message_id 使用 UUID，并由服务端回传对账。
- 滚动采用 requestAnimationFrame 节流；用户向上阅读时停止自动追随。

---

## 15. 前端架构与信息设计

### 15.1 路由设计

建议 URL：

- /contracts
- /contracts/:contractId
- /contracts/:contractId/versions/:versionId
- /contracts/:contractId/reviews/:runId
- /contracts/:contractId/qa/:sessionId
- /knowledge
- /settings/models
- /settings/usage

刷新时根据 URL 从服务端恢复，不再读取 uploaded_file_id、review_session_id、workspace_active 等业务 localStorage。

### 15.2 状态边界

TanStack Query 管理 server state：

- contract
- contract version
- ingest run
- review run
- findings
- events snapshot
- QA sessions/messages
- knowledge sources
- gateway usage

Zustand 只管理 client state：

- 当前编辑器选择和高亮。
- sidecar 是否展开。
- 本地过滤条件。
- 尚未提交的编辑草稿。
- 当前 active stream 的短期 buffer。

localStorage 只允许保存：

- 不敏感 UI 偏好。
- 最近选择的视图。
- 可清除的草稿索引。

不得保存：

- 合同全文。
- 审阅结果真值。
- 当前业务资源唯一标识作为唯一恢复来源。
- access token；建议迁移为 HttpOnly Cookie。

### 15.3 Typed API Client

建立统一层：

- OpenAPI 生成 TypeScript 类型，或使用 Zod 对响应做运行时校验。
- 统一解析 response.Result。
- 统一 ApiError：code、message、request_id、retryable、details。
- 统一注入 auth、Idempotency-Key、request ID。
- 统一 SSE client 支持 id、event、data、heartbeat、Last-Event-ID、AbortSignal。

删除组件中 res?.data?.data 等多层兼容逻辑。

### 15.4 审阅工作台

建议拆分：

- ReviewWorkspaceShell
- ContractViewer
- ReviewRunHeader
- ReviewProgressTimeline
- FindingList
- FindingCard
- EvidenceSidecar
- SuggestionDiff
- DecisionToolbar
- ReviewRunHistory

ReviewPageContent 只负责路由参数、布局和顶层错误边界。

### 15.5 Finding 卡片

卡片展示：

- 风险等级、类型、置信度、验证状态。
- 合同原文定位。
- 风险分析。
- 法律依据及生效日期。
- 建议前后 diff。
- 接受、修改、拒绝。
- 处理人和处理时间。

Finding 在流式阶段允许逐步补齐，但每次更新基于 finding_id 和 revision，不做模糊文本拼接。

### 15.6 建议应用与版本冲突

应用流程：

1. 读取 suggestion 的 target range、base_text、base_hash。
2. 校验当前 editor contract_version。
3. 校验目标范围 hash。
4. 匹配则应用 patch。
5. 不匹配则标记 conflicted，显示三方对比。
6. 保存时创建新 contract_version，不覆盖旧版本。
7. 记录 review_decision 和 applied_version_id。

### 15.7 引用侧栏

借鉴 WeKnora citation popover 和 deer-flow reference sidecar：

- 答案正文只展示紧凑编号。
- 侧栏展示完整来源。
- 合同来源与法规来源使用不同视觉标识。
- 点击合同来源联动编辑器定位。
- 点击法规来源显示版本、生效时间、全文链接和截取段落。

---

## 16. 模型网关设计

### 16.1 请求上下文

所有模型调用附带：

- request_id
- organization_id
- user_id
- feature
- contract_id
- contract_version_id
- run_id
- step_id
- prompt_version
- knowledge_snapshot_id
- model_route_version

### 16.2 配额修复

Phase 0：

- GetQuotas(ctx, userID, feature) 正确查询 feature-specific 和通配配额。
- 增加单元测试覆盖 feature=review、qa 和空 feature。

Phase 5：

- 使用 reservation → settle/refund。
- Redis Lua 原子判断和预占预计 token。
- 完成后按实际 usage 结算。
- 失败或取消时退回未使用额度。
- 数据库异步汇总用于审计和对账。

### 16.3 限流

固定窗口升级为 token bucket 或 sliding window：

- 用户维度。
- 组织维度。
- feature 维度。
- provider/model 维度。

返回 Retry-After 和结构化错误。

### 16.4 路由、熔断和降级

- route params 必须真正应用到模型调用。
- provider/model 定期健康检查。
- 连续失败进入短时熔断。
- 定义同能力模型 failover chain。
- 审阅关键步骤可选择高质量模型，摘要等步骤使用低成本模型。
- 降级必须写 run.degraded 和实际模型。
- 禁止静默换模型后仍声称使用原模型。

### 16.5 缓存

- Review 最终输出默认不使用语义答案缓存。
- QA 最终答案默认禁用。
- embedding、retrieval 和确定性中间结果可缓存。
- 大规模语义缓存使用向量索引，不再 HGETALL。
- 每种缓存定义 scope、version、TTL、最大条目和失效事件。

### 16.6 用量写入

移除无界“每次调用启动一个 goroutine 写日志”的方式，改为：

- 有界 channel。
- 批量写入。
- backpressure 和 drop metric。
- 关键审阅结算使用同步 outbox，避免丢失。

---

## 17. 可观测性与评测

### 17.1 Trace 层级

    HTTP request
      → ingest/review/qa run
        → step
          → retrieval
            → embedding/vector/keyword/rerank
          → model call
          → persistence

统一使用 request_id、run_id、step_id 关联日志、trace、event 和 usage。

### 17.2 运行指标

- ingest_success_rate
- ingest_stage_latency
- review_queue_latency
- review_run_latency
- review_step_latency
- review_cancel_rate
- review_degraded_rate
- sse_reconnect_rate
- event_replay_count
- qa_time_to_first_token
- qa_completion_rate
- retrieval_latency
- model_error_rate
- model_failover_rate
- token/cost per run
- citation_count and citation_validation_failures

### 17.3 离线评测集

建立版本化 gold set：

- 不同合同类型。
- 甲乙方不同立场。
- 高、中、低风险。
- 跨条款引用。
- 无风险负样本。
- 扫描 PDF、表格和重复文本。
- 法规版本变化。
- 合同事实 QA、解释 QA、风险 QA。

标注字段：

- expected clause ranges
- expected risk category/severity
- acceptable legal evidence
- unacceptable hallucinations
- acceptable suggestion constraints

### 17.4 核心质量指标

- Risk Recall：关键风险召回率。
- Finding Precision：风险准确率。
- Citation Correctness：引用是否真实支持结论。
- Citation Coverage：需要证据的结论中有证据的比例。
- Grounded Answer Rate：QA 回答可由合同或法规支撑的比例。
- Suggestion Applicability：建议可无冲突应用的比例。
- Duplicate Finding Rate：重复风险率。
- Human Acceptance Rate：人工接受或轻微修改的比例。

### 17.5 线上反馈闭环

- 接受、修改、拒绝 finding 均作为隐式反馈。
- QA 有用/无用和引用纠错作为显式反馈。
- 反馈不能直接在线训练或修改 Prompt。
- 先进入评测集候选，经过脱敏、审核、版本化后用于回归。

---

## 18. API 草案

建议新增 /api/v2，旧接口在迁移期保留。

### 18.1 合同与解析

| Method | Path | 说明 |
| --- | --- | --- |
| POST | /api/v2/contracts | 上传并创建合同版本，返回 202 |
| GET | /api/v2/contracts/:id | 合同聚合详情 |
| GET | /api/v2/contracts/:id/versions | 版本列表 |
| GET | /api/v2/contract-versions/:id | 指定版本和 ingest 状态 |
| GET | /api/v2/contract-versions/:id/content | canonical blocks/clauses |
| POST | /api/v2/contract-versions/:id/reprocess | 重新解析 |
| GET | /api/v2/ingest-runs/:id/events | 解析事件流 |
| GET | /api/v2/contract-versions/:id/download | 鉴权下载 |

### 18.2 审阅

| Method | Path | 说明 |
| --- | --- | --- |
| POST | /api/v2/review-runs | 创建 run，支持 Idempotency-Key |
| GET | /api/v2/review-runs/:id | 状态、snapshot 和汇总 |
| GET | /api/v2/review-runs/:id/events | SSE 重放与实时事件 |
| POST | /api/v2/review-runs/:id/cancel | 取消 |
| POST | /api/v2/review-runs/:id/retry | 以新 attempt 或新 run 重试 |
| GET | /api/v2/review-runs/:id/findings | 结构化风险 |
| PATCH | /api/v2/findings/:id/decision | 接受、拒绝或修改 |
| POST | /api/v2/suggestions/:id/apply | 应用建议并创建合同新版本 |
| GET | /api/v2/review-runs/:id/compare/:otherId | run 差异 |

创建 run 请求示例：

    {
      "session_id": 12,
      "contract_version_id": 34,
      "stance": "party_a",
      "intensity": "standard",
      "description": "重点关注付款、交付和违约责任",
      "multitask_strategy": "enqueue",
      "on_disconnect": "continue"
    }

### 18.3 QA

| Method | Path | 说明 |
| --- | --- | --- |
| POST | /api/v2/contracts/:id/qa-sessions | 创建绑定指定版本的会话 |
| GET | /api/v2/qa-sessions/:id | 会话与绑定版本 |
| GET | /api/v2/qa-sessions/:id/messages | 消息和 citations |
| POST | /api/v2/qa-sessions/:id/messages | 创建 user message 和 qa_run |
| GET | /api/v2/qa-runs/:id/events | QA SSE |
| POST | /api/v2/qa-runs/:id/cancel | 停止生成 |
| POST | /api/v2/qa-messages/:id/regenerate | 重新生成 |
| POST | /api/v2/qa-messages/:id/feedback | 反馈 |

### 18.4 错误结构

    {
      "code": "REVIEW_RUN_ALREADY_ACTIVE",
      "message": "该合同已有正在执行的审阅",
      "request_id": "req_...",
      "retryable": true,
      "details": {
        "active_run_id": 123
      }
    }

---

## 19. 分阶段实施路线图

### Phase 0：安全与正确性门禁

目标：先消除跨用户数据泄漏、并发串流和缓存污染。

工作项：

- [x] contract/review/session/comparison/QA 公共调用链按当前用户归属查询；foreign 与 missing 资源统一隐藏为 404。
- [x] risk config、model config、gateway routes/quotas 等管理接口增加数据库 `system_role` admin guard。
- [x] 前端设置布局读取 `/user/me.system_role`，member/匿名用户不挂载管理页面并重定向到“关于我们”；owner/admin 才展示管理菜单。
- [x] 文件名改 UUID/object key，限制 50 MB，并校验扩展名、MIME、魔数和 DOCX ZIP 结构。
- [x] 移除后端公开 static 文件访问和 Next.js 本地文件 fallback，改为 bearer 鉴权下载与二进制流直通。
- [x] Orchestrator callback 改为每次 run 参数，共享编排器不再保存运行期可变 callback。
- [x] 使用 `go test -race` 覆盖两个并发审阅的 callback 隔离。
- [x] QA 与 review 最终答案语义缓存默认禁用，阻断当前 cache key 不完整造成的跨合同污染。
- [x] 修复 `GetQuotas` feature 查询，同时拆分运行期 feature 查询与管理列表查询。
- [x] QA SSE 增加 heartbeat、AbortController、请求 generation、停止按钮和未收到 end 的断流错误。
- [x] 增加安全上传、DOCX ZIP traversal、答案缓存策略和并发 callback 单元测试。
- [x] 增加基于真实测试数据库的双用户 IDOR 集成测试，覆盖合同、会话、审阅、比对和下载（`app/internal/middleware/idor_integration_test.go`，build tag `//go:build integration`，通过 `CONTRACT_REVIEW_TEST_DSN` 环境变量注入）。
- [ ] 接入病毒扫描或隔离区扫描流程；扫描完成前文件不可进入解析和审阅。
- [x] 将当前 user/account scope 抽象为统一 `ResourceScope`，并为 organization_id 与组织角色预留迁移路径（`app/internal/middleware/scope.go`）。

退出条件：

- 任何合同相关 ID 访问都无法跨用户读取或修改。
- 50 组并发双审阅测试无回调串流，race detector 通过。
- QA 不可能命中其他合同或其他用户答案。
- 普通用户无法管理模型、路由和配额。

#### Phase 0 Batch 1 实施结果（2026-08-11）

本批次已经完成最紧急的代码级安全与并发门禁，并保留以下边界：

- 审阅在浏览器或代理断开后使用最长 30 分钟的独立进程内 context 继续运行。这是 Phase 0 临时止血方案，不具备服务重启恢复、事件重放和跨实例接管能力；正式方案仍由 Phase 2 的 durable review run/event store 完成。
- 旧 `/api/static/...` URL 不再提供本地文件 fallback，可能返回 404。若需兼容历史数据，只能实现“登录用户 + 数据库 file_path + owner scope”校验的 legacy adapter，不能恢复按 basename 直接读盘。
- 数据库 `system_role` 是管理权限权威来源；JWT role 仅可作为前端显示提示。启动和新用户创建后会幂等确保系统至少存在一个 owner/admin，避免管理接口永久锁死。
- review 与 QA 的语义答案缓存暂时整体关闭。后续只有在 cache key 完整包含 organization、user、contract_version、knowledge_snapshot、prompt_version 和 model policy 后才允许重新开启。
- 当前 quota 已能按 feature 正确查询，但“检查 + 扣减”仍非原子事务，原子 reservation/commit/release 留在 Phase 5。

验证记录：

- `GOCACHE=/private/tmp/contract-review-gocache go test ./...`：通过。
- `go test -race ./app/internal/agent ./app/internal/contract ./app/internal/gateway`：通过。
- `npm run build`：通过，Next.js 生产构建和 TypeScript 校验成功。
- 本批次前端文件定向 ESLint：0 error；仓库全量 lint 仍受历史文件错误和 vendor worker warnings 影响。
- `git diff --check`：通过。

尚未满足的 Phase 0 完整退出条件：病毒扫描（需 ClamAV 或云端服务接入）。

### Phase 1：统一合同资产与异步解析

目标：合同只解析一次，审阅、QA、对比和建议应用共享同一版本资产。

工作项：

- [ ] contracts 与 contract_versions 分离。
- [ ] 新增 contract_blocks、contract_clauses。
- [ ] 新增 ingest run/step/event。
- [ ] 上传 API 改 202 异步。
- [ ] 统一 DOCX/PDF/TXT/OCR adapter。
- [ ] 持久化 canonical document 和合同索引。
- [ ] 前端展示解析阶段、失败原因和重试按钮。
- [ ] QA 去掉每次 ExtractText 和 600 字临时切块。

退出条件：

- 同一版本只解析和索引一次。
- 上传接口 P95 在文件写入完成后 2 秒内返回任务 ID。
- 解析失败无需重新上传即可重试。
- 审阅与 QA 使用同一 contract_version 和 clause ID。

### Phase 2：Durable Review Run 与结构化结果

目标：审阅可断线继续、可重放、可取消、可比较、可审计。

工作项：

- [ ] review_runs、steps、events、outbox。
- [ ] durable worker 和租约。
- [ ] Last-Event-ID、heartbeat、事件重放。
- [ ] run 历史保留，不再删除旧结果。
- [ ] finding/evidence/suggestion/decision 结构化表。
- [ ] snapshot 固定 contract/prompt/model/retrieval/knowledge 版本。
- [ ] 支持 cancel、retry、enqueue/reject/interrupt。
- [ ] 统一整体风险算法。
- [ ] 建议使用 range + base hash。

退出条件：

- 浏览器刷新后可恢复同一 run。
- API 服务或 SSE 连接重启不丢已提交事件和 finding。
- 每个 finding 均能定位到合同版本和 clause。
- 重审产生新 run，历史可比较。

### Phase 3：可验证 RAG、知识发布与评测

目标：风险和回答真正做到“有据可查、版本可复现”。

工作项：

- [ ] Unified RetrievalService。
- [ ] 合同与法律知识双路检索。
- [ ] knowledge document/version/status/snapshot。
- [ ] retrieval trace。
- [ ] citation entity 和 citation validator。
- [ ] 确定性 QualityGate。
- [ ] gold set 和自动回归。
- [ ] 按合同类型、立场、法域、生效时间过滤。

退出条件：

- 新 run 固定 knowledge_snapshot。
- 关键风险 Citation Coverage 达到约定阈值。
- 引用均可点击并定位到原文。
- 知识更新不会改变历史 run 的证据。

### Phase 4：前端工作台重构

目标：提升深链、恢复、核验、编辑和 QA 体验。

工作项：

- [ ] URL 资源化。
- [ ] TanStack Query 管理 server state。
- [ ] 统一 typed API 和 SSE client。
- [ ] 拆分三个超大组件。
- [ ] ReviewProgressTimeline 和 run history。
- [ ] EvidenceSidecar。
- [ ] QA 三栏布局、引用、停止、重生成、反馈。
- [ ] editor patch 冲突处理。
- [ ] 移除业务 localStorage 真源。

退出条件：

- 任意审阅和 QA URL 可刷新恢复并分享给有权限用户。
- 多标签页打开不同合同不会串状态。
- 引用可在合同预览或法规侧栏中核验。
- 旧流不会写入新会话。

### Phase 5：网关、运营和规模化

目标：具备稳定的成本、容量和故障治理能力。

工作项：

- [ ] 原子 quota reservation/settle/refund。
- [ ] token bucket/sliding window。
- [ ] model health、circuit breaker 和 failover。
- [ ] 有界 usage queue 和批量落库。
- [ ] 运行成本归因看板。
- [ ] 管理员 RBAC UI。
- [ ] 缓存向量索引和容量治理。
- [ ] SLO dashboard 和报警。

退出条件：

- 并发下配额不超卖。
- 模型故障可按策略降级并显式记录。
- 每个 run 可展示各 step token、成本和延迟。

---

## 20. 验收指标

以下是初始目标，需根据真实压测基线调整。

### 20.1 安全

- 资源级越权自动化测试覆盖合同、版本、run、finding、QA、下载和配置接口。
- 跨用户访问成功率必须为 0。
- 非法文件类型、伪造扩展名、超限文件和 ZIP bomb 均被拒绝。
- 生产环境不存在无需鉴权的合同静态 URL。

### 20.2 可靠性

- 已持久化的 review event 丢失率为 0。
- SSE 重连后事件重复可以幂等处理，顺序不倒退。
- review run 在浏览器断线后继续执行。
- worker 异常退出后，租约到期的 run 可被重新领取。
- 同一 Idempotency-Key 不产生重复 run、消息或合同版本。

### 20.3 性能体验

- 上传完成后 2 秒内返回 ingest run。
- 首个解析进度事件 2 秒内可见。
- QA 正常网络下 TTFT P95 目标小于 3 秒，不包含模型供应商重大故障。
- 检索服务 P95 目标小于 800 ms。
- 100 个 finding 的工作台滚动和筛选不出现明显长任务。

### 20.4 质量

- 高严重度 finding 需要至少一个有效 contract evidence。
- 需要法律依据的 finding，其 Citation Coverage 目标不低于 95%。
- Citation Correctness 在 gold set 上目标不低于 90%。
- 合同事实 QA 的 Grounded Answer Rate 目标不低于 95%。
- 重复 finding 比例目标低于 5%。
- 建议自动应用前 base hash 校验覆盖率为 100%。

---

## 21. 测试策略

### 21.1 后端单元测试

- ResourceGuard 角色矩阵。
- repository scope 查询。
- 文件类型和魔数。
- contract block/range 定位。
- run 状态转换。
- event seq 生成和重放。
- idempotency。
- quota feature 查询和原子预占。
- citation validator。
- RiskAggregator。

### 21.2 并发与竞态测试

- go test -race ./...
- 同时启动多个 review run，验证 progress/finding 只进入所属 run。
- worker 重复领取保护。
- cancel 与 step 完成竞争。
- SSE replay 与实时事件切换不丢不乱。
- quota 并发预占不超卖。

### 21.3 集成测试

- 上传 → 解析 → 索引 → 审阅 → finding → 应用建议 → 新版本。
- 上传 → QA → 合同引用定位。
- 发布知识 v2 后，新 run 使用 v2，旧 run 仍读取 v1 snapshot。
- Redis 不可用时从 DB 重放事件。
- 模型主路由失败时 failover，并记录 degraded。

### 21.4 前端测试

- URL 刷新恢复。
- 多标签页合同隔离。
- 切换 QA session 会取消旧流。
- SSE 重连和重复事件幂等。
- finding revision 更新。
- citation 点击定位。
- patch 冲突显示。
- 键盘操作和可访问性。

### 21.5 E2E

至少维护以下主路径：

1. DOCX 合同完整审阅。
2. PDF 扫描件异步 OCR。
3. 断开 SSE 后恢复。
4. 重审并对比两个 run。
5. QA 多轮指代与引用。
6. 接受建议后生成新版本。
7. 两用户资源隔离。
8. 管理员发布知识和普通用户禁止访问设置。

---

## 22. 数据迁移与兼容

### 22.1 迁移原则

- 先加表和双写，再切读路径，最后停止旧写入。
- 不直接删除 contracts、review_tasks、review_results、sessions。
- 所有迁移可重入并记录 migration checkpoint。
- 对象存储迁移前保留旧本地路径读取 adapter。

### 22.2 建议步骤

1. 为现有用户创建 default organization。
2. 回填 contracts.organization_id 和 owner_user_id。
3. 每条现有 contract 创建 version_no=1。
4. 异步解析旧合同，生成 blocks/clauses。
5. 每条现有 review_task 映射为 review_run。
6. 将 review_results 映射为 review_findings，标记 legacy_unstructured=true。
7. 旧结果缺少 evidence 时不伪造，UI 明确显示“历史结果无结构化依据”。
8. 新旧 API 双读，前端按 feature flag 切换。
9. 观察稳定后停止旧接口写入。

### 22.3 兼容策略

- v1 review SSE 可由 v2 events adapter 生成旧事件格式。
- 旧 localStorage 只用于一次性读取并跳转到新 URL，迁移后删除。
- 旧 file_path 通过 LocalFilePath adapter 读取，禁止继续新增此格式。

---

## 23. 风险与取舍

| 风险 | 影响 | 缓解 |
| --- | --- | --- |
| 数据模型重构范围大 | 迁移周期长 | 分阶段双写、legacy 标记、feature flag |
| Durable worker 引入状态复杂度 | 重复执行或事件不一致 | 幂等 node、事务 outbox、租约、状态机测试 |
| 结构化解析质量不稳定 | 引用定位失败 | parser adapter、content hash、降级标记、人工校正 |
| Citation 约束降低回答流畅性 | 输出可能更保守 | 明确区分“直接依据”和“一般建议” |
| 组织/RBAC 增加产品复杂度 | UI 和迁移成本 | 首期默认组织，先后端守卫后管理 UI |
| 多索引版本占用存储 | 成本增加 | snapshot 引用计数、保留策略、冷存储 |
| 前端重构与业务迭代冲突 | 回归风险 | 路由级渐进替换，复用现有 viewer/editor |

---

## 24. Architecture Decision Records

### ADR-001：保持 Go 模块化单体，首期不引入 Temporal

- 状态：Proposed
- 决策：使用 MySQL run/step/event/outbox + worker lease。
- 原因：当前规模更需要正确性和可恢复语义，不需要先承担工作流平台运维成本。
- 复审条件：高并发、跨小时人工等待或多服务长事务成为常态。

### ADR-002：合同内容采用不可变版本

- 状态：Proposed
- 决策：任何编辑和建议应用都创建 contract_version。
- 原因：保证审阅、问答、引用、比较和审计可复现。

### ADR-003：SSE 是事件传输层，不是任务执行容器

- 状态：Proposed
- 决策：run 独立执行，SSE 从 event store 重放和订阅。
- 原因：浏览器连接不可靠，不能决定业务任务存活。

### ADR-004：QA 最终答案语义缓存默认关闭

- 状态：Proposed
- 决策：优先缓存 embedding/retrieval；只有完整 scope key 后才评估答案缓存。
- 原因：合同内容高度私密，错误缓存命中属于严重数据泄漏。

### ADR-005：有限状态 DAG 优先于通用自由 Agent

- 状态：Proposed
- 决策：合同审阅使用可观察、可预算、可重试的 Strategy DAG。
- 原因：法律场景更需要确定性、证据和可审计，而不是最大自主性。

### ADR-006：URL 与服务端状态作为前端真源

- 状态：Proposed
- 决策：业务资源使用路由 ID 和 query cache 恢复；Zustand 只保存 UI/stream 状态。
- 原因：解决刷新、多标签页、深链和跨设备一致性。

### ADR-007：知识使用发布版本与 snapshot

- 状态：Proposed
- 决策：只有 published knowledge_version 可进入 snapshot，run 固定 snapshot。
- 原因：法规和内部规则会变化，历史结论必须可复现。

### ADR-008：统一 ResourceScope 作为请求级身份载体

- 状态：Accepted（2026-08-12）
- 决策：引入 `middleware.ResourceScope` 统一体（OrganizationID / UserID / Account / SystemRole），通过 `ResolveScope` 中间件一次解析并缓存到 Redis（120s TTL），handler 与 service 通过 `GetScope` 获取后传递 `scope.Account` / `scope.UserID` 给 repo 层，消除 5 个模块各做一份的 account→userID 解析。
- 原因：消除 4 份重复 `ResolveUserID` 实现（qa/review/session + comparison 私有），统一 contracts 表的 account 字符串隔离与新表 user_id 数字隔离的 owner 键差异，为 Phase 1 的 contracts.account→owner_user_id 迁移和 organization 字段预留路径。
- 影响：31 个 handler 调用点从 `GetCurrentUserID` + `ResolveUserID` 两步收敛为 `GetScope` 一步；管理守卫 `RequireSystemRole` 保持每次权威 DB 查询，不受 scope 缓存影响。

---

## 25. 实施检查表

### 基础治理

- [ ] 确认 v1.0 Spec 评审人和负责人。
- [ ] 为每个 Phase 建立 issue/里程碑。
- [ ] 为 P0 项建立安全回归测试。
- [ ] 建立 API v2 兼容原则。
- [ ] 建立 schema migration 规范。

### 每个功能的 Definition of Done

- [ ] 有数据模型或状态变化说明。
- [ ] 有资源权限检查。
- [ ] 有幂等和重试语义。
- [ ] 有错误码和用户可理解提示。
- [ ] 有日志、metric 和 trace 字段。
- [ ] 有单元或集成测试。
- [ ] 有前端刷新/断线/竞态验证。
- [ ] Spec、ADR 和 API 文档同步更新。

---

## 26. Spec 迭代机制

### 26.1 版本规则

- PATCH：澄清文字、补充证据，不改变决策。
- MINOR：新增或调整一个设计模块、API 或里程碑。
- MAJOR：改变核心领域模型、执行语义或兼容策略。

### 26.2 状态规则

- Draft：正在讨论。
- Accepted：架构评审通过。
- Implementing：已有 Phase 进入开发。
- Partially Implemented：部分 Phase 完成。
- Implemented：全部强制项完成。
- Superseded：被新 Spec 替代。

### 26.3 变更记录模板

| 日期 | 版本 | 变更 | 关联 issue/PR | 决策人 |
| --- | --- | --- | --- | --- |
| 2026-08-11 | v1.0.0-draft | 初始版本，完成现状诊断、目标架构和 Phase 0-5 路线图 | 待补充 | 待补充 |
| 2026-08-11 | v1.1.0-draft | Phase 0 Batch 1：资源归属、管理权限及前端镜像、私有文件、安全上传、Orchestrator 并发隔离、QA 流可靠性、缓存与 feature quota 修复，并记录验证结果和剩余门禁 | 本地未提交实现 | Codex + 项目维护者待确认 |
| 2026-08-12 | v1.2.0-draft | Phase 0 收尾：统一 ResourceScope 抽象（消除 4 份重复 ResolveUserID，handler→service 身份解析收敛为 scopeMW 单点缓存），5 模块 identity 路径重构（contract/review/qa/session/comparison），双用户 IDOR 集成测试 12 项覆盖（合同/会话/审阅/QA/比对跨用户读/写/删/列），新增 ADR-008 | 本地未提交实现 | Codex + 项目维护者待确认 |

### 26.4 实施进度模板

| Phase | 状态 | 已完成 | 阻塞 | 下一步 | 最后更新 |
| --- | --- | --- | --- | --- | --- |
| Phase 0 | Implemented | 首批 user scope、admin guard/前端镜像、私有下载、安全上传、callback 隔离、QA 流、cache/quota 修复与验证、统一 ResourceScope、IDOR 集成测试 | 仅病毒扫描（SEC-05）待 ClamAV 接入 | 病毒扫描接入 | 2026-08-12 |
| Phase 1 | Not Started | - | - | 等待 Phase 0 | 2026-08-11 |
| Phase 2 | Not Started | 审阅断线继续已有进程内临时方案 | 尚无 durable worker/event store | 评审 review run、event、outbox 数据模型 | 2026-08-11 |
| Phase 3 | Not Started | - | - | 等待 snapshot 设计 | 2026-08-11 |
| Phase 4 | Not Started | - | - | 等待 v2 API | 2026-08-11 |
| Phase 5 | Not Started | - | - | 等待运行数据基线 | 2026-08-11 |

后续每完成一个实现批次，应同时更新：

1. 问题矩阵中的状态。
2. Phase 检查框。
3. ADR 状态。
4. API 或数据模型差异。
5. 验收结果和已知限制。
6. 变更记录。

---

## 27. 推荐的第一批实现

为了尽快获得最高收益，第一批代码修改应严格控制在 Phase 0：

1. 引入 ResourceScope，并修复合同更新、删除、下载和查询的资源归属。
2. 为管理接口增加 admin guard。
3. 安全化上传文件名和下载链路。
4. 将 ReviewOrchestrator callback 从共享实例字段移到 Execute 参数。
5. 默认关闭 QA 最终答案语义缓存。
6. 修复 feature quota 查询。
7. 补充 QA AbortController、会话 generation 和断流错误。
8. 增加 race、IDOR、缓存隔离和 quota 回归测试。

该批次不需要先改造全部数据模型，却能直接消除最严重的安全和并发风险，也为后续 durable run 奠定正确边界。
