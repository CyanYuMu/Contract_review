# P0 优化落地 Spec：收紧 verified 判定 + 风险点结构化 Metadata

> 版本 v1.0 | 状态：已完成并验收（编译 + 全量单测 + 启动冒烟 + DB 端到端 round-trip 均通过）
> 关联文档：`docs/contract-review-agent-architecture.md`（设计）、`docs/review-pipeline-optimization-plan.md`（链路优化）

---

## 一、目标与背景

审阅链路中，风险点"是否可验证（verified）"是结果可信度的核心开关。改造前存在两个问题：

1. **verified 判定被稀释**：`normalizeLegalBasis` 会用候选风险点的 `LegalBasis / TriggerCondition / Content` 三选一兜底填充"法律依据"，而这三者里 `Content` 是整段拍扁模板、`LegalBasis` 可能是"未配置"占位文本。结果：只要命中候选，几乎必然 `verified=true`，`RuleVerifier` 的护栏思想（`rule_verifier_tool.go`）实际未接入。
2. **结构化风险点被"拍扁再抠回来"**：风险点本是结构化数据，`buildKnowledgeContent` 先拼成一段 `【风险点配置】…` 文本存 chunk，检索命中后又靠 `extractKnowledgeField` 正则按行反解字段。信息被往返编码两遍，脆弱且浪费。

本次 P0 落地两项：**① 收紧 verified 判定**（占位文本/拍扁模板不再构成依据）；**② 风险点结构化字段写入 chunk metadata**（替代正则反解）。

---

## 二、已完成的改动

### 2.1 风险点结构化字段写入分块 metadata

**数据模型**（`app/internal/knowledge/model.go`）
`ReviewKnowledgeChunk` 新增 `Metadata string`（`gorm:"type:json"`），存放结构化元数据 JSON。

**写入**（`app/internal/riskconfig/service.go`）
- `syncKnowledgeDoc` 创建分块时写入 `Metadata: buildRiskPointMetadataJSON(riskPoint)`。
- 新增 `buildRiskPointMetadata` / `buildRiskPointMetadataJSON`：把以下字段结构化为 map 并序列化：
  `risk_id / contract_type / risk_type / risk_level / applicable_scope / trigger_condition / keywords / applicable_clauses / legal_basis / recommended_template / risk_content`。
- `keywords` / `applicable_clauses` 存"、"连接串，与 `splitListField` 的分隔符保持一致（兼容旧解析）。

**读取合并**（`app/internal/rag/knowledge_loader.go` + `app/internal/knowledge/repo.go`）
- `ListIndexedChunksForRAG` 的 SELECT 增加 `COALESCE(c.metadata, '{}') AS metadata`（对老数据 NULL 安全）。
- 新增 `mergeChunkMetadata`：解析 JSON 元数据并与基础字段（title/category/source 等）合并，**结构化值优先、基础字段兜底**。

**迁移**（`docs/sql/review_knowledge.sql`）
- CREATE TABLE 增加 `metadata json DEFAULT NULL` 列。
- 老库手动补列语句以注释形式给出；应用启动时 GORM `AutoMigrate`（`knowledge.AutoMigrate`）也会自动补列。

### 2.2 候选解析优先读 metadata

**`app/internal/agent/candidate_risk_agent.go`**
- `RiskCandidate` 新增 `RiskContent` 字段。
- `riskCandidateFromSearchResult` 重写为"**metadata 优先，旧数据回退到拍扁文本反解**"（`field(metaKey, contentField)` 闭包）。
- 兼容性：老库 chunk 无 metadata 时行为不变（回退 `extractKnowledgeField`）。

### 2.3 收紧 verified 判定（核心）

`normalizeLegalBasis` 重写，只保留"**可定位、有实质内容**"的依据：

1. **LLM 直接输出的依据**：`isCredibleBasis(source, article, content)` — 内容有效，且来源或条款编号至少一个能定位。
2. **候选派生的依据**：`candidateBasisFields` 按优先级取——
   - `LegalBasis`（有效，非占位）→ 作为法律依据；
   - `RiskContent`（有效）→ 作为"审阅规范"依据；
   - `Content`（纯文本审阅指引，如种子/内置知识）→ 作为审阅规范依据；
   - **显式排除 `【风险点配置】` 开头的拍扁模板**。

新增判定工具：
- `isMeaningfulBasis`：空/占位（"未配置/无/暂无/待补全/待补充/n-a/na/null/none/-/—"）/长度 ≤1 均视为无效。
- `placeholderBasis` 占位词表。

`parseCandidateRiskFindings` 中：
```go
verified := raw.Verified && len(legalBasis) > 0
// 确定性护栏：LLM 声称已验证但自评置信度过低（且确实给了分数）→ 降级待人工确认
if verified && raw.Confidence > 0 && raw.Confidence < 0.3 {
    verified = false
}
requiresHumanReview := raw.RequiresHumanReview || !verified
```

**效果**：候选只有"未配置"占位、无风险内容、无实质指引 → 不再 verified，标记 `requires_human_review=true`；有 `RiskContent`（审阅规范）或有真实法律依据 → 仍 verified，但依据不再是占位文本或拍扁模板。

---

## 三、验收结果（全部通过）

| 验证项 | 结果 |
|---|---|
| `gofmt` | 干净 |
| `go build ./...` | 通过 |
| `go vet`（agent/rag/knowledge/riskconfig） | 通过 |
| `go test ./...`（全量，含新增 6 个单测） | 全部 ok |
| 启动冒烟（8080 被旧实例占用 → 换 18080，vector 关闭走纯关键词路径） | 启动成功、知识库加载成功、编排器预热完成 |
| DB schema 迁移 | `review_knowledge_chunks.metadata` 已生成（`json` 类型） |
| DB 端到端 round-trip（临时程序走真实 `riskconfig.Service.Create` → 落库 → 读回 → Delete 清理） | `PASS: metadata round-trip verified` |

新增单测（`app/internal/agent/candidate_risk_agent_test.go`、`app/internal/riskconfig/service_test.go`）：
- `TestRiskCandidateFromSearchResultPrefersMetadata`：metadata 优先解析。
- `TestRiskCandidateFromSearchResultFallsBackToContentParsing`：老数据回退。
- `TestParseCandidateRiskFindingsPlaceholderBasisNotVerified`：占位依据不 verified。
- `TestParseCandidateRiskFindingsRiskContentAsReviewBasisVerified`：风险内容作审阅规范依据仍 verified，且依据非占位。
- `TestParseCandidateRiskFindingsLowConfidenceDowngraded`：低置信度降级。
- `TestCandidateBasisFieldsExcludesFlattenedTemplate`：拍扁模板被排除、纯文本指引保留。
- `TestBuildRiskPointMetadata`（riskconfig）：元数据字段正确 + JSON 可序列化。

---

## 四、行为变化与影响面

- **更保守的 verified**：命中候选但候选"无实质依据"的风险，从"已验证"变为"待人工确认"（`requires_human_review=true`）。这是本次的**预期行为**，会让前端"已通过规范验证"的计数更真实。
- **法律依据更干净**：`LegalBasis.Content` 不再出现 `【风险点配置】…` 整段模板，而是具体条文或风险内容。
- **兼容性**：老库数据（无 metadata）完全兼容，走回退解析；新数据走 metadata。
- **无对外接口变更**：数据结构 `RiskCandidate`/`LegalBasis` 仅内部增字段，不破坏现有 JSON 输出契约。

---

## 五、接下来应完成的事（后续阶段，按优先级）

> 建议按顺序推进，每阶段同样"编译 + 单测 + 冒烟 + 端到端"验收。

### P0 收尾（可选，低风险）— ✅ 已完成
- [x] **旧风险点补 metadata**：新增 `riskconfig.Service.BackfillRiskPointMetadata`（幂等，复用 `buildRiskPointMetadataJSON`，仅更新 `metadata IS NULL` 的存量分块），并在 `main.go` 启动时、预热编排器之前调用。验证：`meta_covered` 由 0 → 1，二次启动无重复补齐。

### P1-1 真混合检索（召回精度）— ✅ 已完成
- [x] `SimpleKeywordIndex.Search` 从"TF 子串匹配"升级为**含 IDF 的 BM25**（k1=1.2, b=0.75），分数经 `score/(score+norm)` 归一化到 (0,1)。
- [x] 统一 `tokenize`：查询/文档共用 CJK bigram + 法律关键词 + 词/标点切分，保证 term 跨查询/文档匹配。
- [~] Rerank 已代码实现且 dev 配置 `rerank.enabled=true`（配置驱动，非本阶段代码改动）。
- [ ]（可选）Milvus 2.5 原生 BM25 替代 LIKE 全文检索（基础设施升级，非代码层）。

### P1-2 父子分块（召回 + 上下文完整性，呼应 WeKnora）— ✅ 已完成
- [x] 落地 `Chunk.ParentChunkID` / `Chunk.ChunkType` / `SearchResult.ParentChunkID`（含 child/parent 常量）。
- [x] 小子块（maxChunkSize=300、带 overlap、句末断句）用于检索，命中后按 `ParentChunkID` 回填父块（整章/整条）并按 parent 去重。
- [x] `review_knowledge_chunks` 表加 `parent_chunk_id / chunk_type` 列（迁移）。
- [x] `DocumentProcessor` 复活为父子分块产出器（child 检索 + parent 上下文，仅 child 生成 embedding）。
- [x] **长文档入库入口**：新增 `rag.IngestKnowledgeDocument`（父子分块 + 持久化，事务内用真实 doc.ID 生成 chunk ID）+ `knowledgeapi` 包（`POST /api/knowledge/document` 管理端接口）。验证：入库长文档 → DB 读出 1 parent + 5 child 全部正确联动，预热日志 `parents=1 child_chunks=5`。

### P1-3 自适应检索 / Self-RAG（性能 + 精度，轻量路由）
- [ ] **条款级路由（确定性）**：`classifyClauses` 后跳过首部/签署页/送达通知等 boilerplate 条款，不检索不审阅。
- [ ] **检索置信度路由**：候选为空或最高分低于阈值时才触发二次泛化检索（去标题、扩 TopK、走 Rerank），而非每条款固定检索两趟。
- [ ] **法律专业问题路由（可选）**：区分"纯审阅规范问题"（付款节点不明）vs"法律专业问题"（违约金上限/竞业期限），后者才触发法规条文级验证。

### P1-4 跨条款去重合并（报告质量）— ✅ 已完成
- [x] 新增 `agent.MergeFindings`：按候选风险点 ID（或风险类型+法律依据）归并多条款重复风险，保留 `ClauseIDs`，聚合最严重等级/最高置信度/已验证/待复核，主条款取最早条款。
- [x] `ExecuteBatchWithCallback` 在分发回调前合并，流式输出与最终报告一致，避免重复行。
- [x] `buildRiskAnalysis` 命中多条款时展示"命中条款"列表。

### P1-5 真 Reflection（覆盖率，把质量门反馈注入重审）— ✅ 已完成
- [x] Phase 5 由"单次评估"改为反思循环：质量门评分后，存在无风险发现的缺口条款且质量信号要求重审时，把缺口作为 `reflection_hints` 注入 `CandidateRiskAgent`，**只对缺口条款定向重审**（不做全量重审），合并后重新评估，受 `ReflectionConfig.MaxRetries` 上限约束。
- [x] 新增 `findGapClauses`（找未被 finding 覆盖的条款）与 `buildReflectionHints`（由 CriticalGaps/Feedback 构建反馈）。
- [x] 风险审阅链路（`ExecuteBatchWithCallback`/`reviewCandidateBatch`/`buildCandidateRiskPrompt`）支持 `reflectionHints` 注入。

### P2 性能与清理
- [ ] 网关语义缓存对 review 跳过（`gateway.go` 中 `Feature == FeatureReview` 跳过 cache lookup/store）。
- [ ] 清理死代码：`RiskAgent`/`react_loop.go`/`rule_verifier_tool.go`/`buildEnhancedCandidateQuery` —— 接线或删除，消除文档与代码脱节（`DocumentProcessor` 已在 P1-2 复活）。

---

## 六、改动文件清单

| 文件 | 改动 |
|---|---|
| `app/internal/knowledge/model.go` | `ReviewKnowledgeChunk` 增 `Metadata` 字段 |
| `app/internal/knowledge/repo.go` | `IndexedChunkRow` 增 `Metadata`；SELECT 加 `COALESCE(metadata,'{}')` |
| `app/internal/riskconfig/service.go` | `syncKnowledgeDoc` 写 metadata；新增 `buildRiskPointMetadata(JSON)` |
| `app/internal/rag/knowledge_loader.go` | 加载时解析并合并 metadata；新增 `mergeChunkMetadata` |
| `app/internal/agent/candidate_risk_agent.go` | `RiskCandidate` 增 `RiskContent`；`riskCandidateFromSearchResult` metadata 优先；`normalizeLegalBasis` 收紧；`parseCandidateRiskFindings` 收紧 verified；新增判定工具函数 |
| `docs/sql/review_knowledge.sql` | 分块表增 `metadata` 列 + 老库补列注释 |
| `app/internal/agent/candidate_risk_agent_test.go`（新增） | 6 个单测 |
| `app/internal/riskconfig/service_test.go`（新增） | 1 个单测 |

---

## 七、P1 落地记录（真混合检索 + 父子分块）

> 版本 v1.1 | 状态：已完成并验收（编译 + 全量单测 + 启动冒烟通过）

### 7.1 P1-1 真混合检索（BM25 + IDF）

`SimpleKeywordIndex` 从"TF 子串匹配"升级为 **BM25**：

- 每次 `Index` 重建统计：每文档词频、term 文档频率（DF）、平均文档长度。
- 打分：对查询每个 term 累加 `IDF * tf * (k1+1) / (tf + k1*(1-b+b*dl/avgdl))`，`k1=1.2, b=0.75`。
- IDF 抑制高频词（"合同/甲方"等），TF 饱和 + 长度归一避免长文刷分。
- 分数经 `score/(score+1.5)` 归一化到 (0,1)，保持"相关度"语义，兼容 RRF 融合与 `MinRelevance` 过滤。
- 统一 `tokenize`：查询/文档共用 CJK bigram + 法律关键词 + 英文词 + 标点切分。

### 7.2 P1-2 父子分块（parent-child，参考 WeKnora）

数据流：`DocumentProcessor`（入库时切块）→ `review_knowledge_chunks`（parent/child 标记）→ `LoadKnowledgeChunksFromDB`（读取）→ `PartitionChunks`（分区）→ 检索命中 child → `expandParents`（回填 parent 上下文）。

- **数据模型**：分块表新增 `parent_chunk_id` / `chunk_type`；`Chunk.ChunkType`、`SearchResult.ParentChunkID`、`ChunkTypeParent/Child` 常量。
- **切块**：`DocumentProcessor` 产出 child 小块（maxChunkSize=300、overlap、句末断句）+ parent 全章；仅 child 生成 embedding。
- **分区**：`PartitionChunks` 把 parent 从检索单元剥离，child/独立块进入关键词索引与向量库。
- **回填**：`RAGRetriever.expandParents` 在 final TopK 后把命中 child 回填为 parent 内容，同一 parent 多 child 去重，子块原文存 `metadata["child_content"]` 供追溯。
- **接线**：`InitOrchestrator` 分区 + `SetParents`；keyword 与向量（Milvus metadata_json）双通道回传 `ParentChunkID`。

### 7.3 验收结果

| 验证项 | 结果 |
|---|---|
| `gofmt` / `go build ./...` / `go vet` | 通过 |
| `go test ./...`（含新增 keyword_index_test.go + parent_child_test.go） | 全部 ok |
| 启动冒烟（18080，vector 关走关键词路径） | 启动成功、知识库加载 `chunks=1 parents=0`、编排器预热完成 |
| DB schema 迁移 | `parent_chunk_id` / `chunk_type` 列已生成 |

### 7.4 注意事项

- **存量数据**：现有风险点/种子 chunk 无 parent（空类型=独立 child），不触发回填，行为不变；`DocumentProcessor` 已就绪，待接入长文档入库入口后，长法规/指引即可享受父子分块。
- **旧库分块表**：需 GORM AutoMigrate 补 `parent_chunk_id`/`chunk_type` 列（启动自动完成，或见 SQL 注释中的 ALTER）。
- **BM25 分数尺度变化**：关键词通道分数由"匹配率"变为归一化 BM25，RRF 是 rank-based 不受影响；`MinRelevance` 阈值语义仍为 0~1。

### 7.5 P1 改动文件清单

| 文件 | 改动 |
|---|---|
| `app/internal/rag/retriever.go` | SimpleKeywordIndex 升级 BM25+IDF；`SetParents`/`expandParents`/`PartitionChunks`；`tokenize` 统一分词 |
| `app/internal/rag/types.go` | `Chunk.ChunkType`、`SearchResult.ParentChunkID`、`ChunkTypeParent/Child` 常量 |
| `app/internal/rag/document_processor.go` | 重写为父子分块产出器；修复末尾重叠窗口死循环 |
| `app/internal/rag/knowledge_loader.go` | 读取 parent/type；`parent_chunk_id` 入 metadata |
| `app/internal/rag/milvus_store.go` | 向量结果回传 `ParentChunkID` |
| `app/internal/knowledge/model.go` | `ReviewKnowledgeChunk` 增 `ParentChunkID`/`ChunkType` |
| `app/internal/knowledge/repo.go` | 查询增 `parent_chunk_id`/`chunk_type` |
| `app/internal/review/service.go` | `InitOrchestrator` 分区 + `SetParents` |
| `docs/sql/review_knowledge.sql` | 分块表增列 + 索引 |
| `app/internal/rag/keyword_index_test.go`（新增） | BM25 检索/过滤/分词单测 |
| `app/internal/rag/parent_child_test.go`（新增） | 父子产出/分区/回填去重单测 |
