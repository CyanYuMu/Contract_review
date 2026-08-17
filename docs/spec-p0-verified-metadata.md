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

### P0 收尾（可选，低风险）
- [ ] **旧风险点补 metadata**：已存在的风险点（如当前库里的 `RP000001`）其 chunk metadata 仍为 NULL。可写一次性迁移脚本，遍历 `review_risk_points` 重新执行 `syncKnowledgeDoc`（等价于逐条 update），让存量数据也享受结构化 metadata。

### P1-1 真混合检索（召回精度）
- [ ] `SimpleKeywordIndex.Search` 从"TF 匹配"升级为**含 IDF 的 BM25**，或启用 Milvus 2.5 原生 BM25（`milvus_store.go` 已预留注释）。
- [ ] 启用 Rerank（config 已有 `gte-rerank-v2`），保留现有阈值降级护栏。

### P1-2 父子分块（召回 + 上下文完整性，呼应 WeKnora）
- [ ] 落地 `Chunk.ParentChunkID` / `ChunkType("parent"/"child")`（`rag/types.go` 字段已备好但未赋值）。
- [ ] 小子块（~300–500 字）做向量/关键词召回，命中后按 `ParentChunkID` 回填父块（整章/整条）拼进 prompt。
- [ ] `review_knowledge_chunks` 表加 `parent_chunk_id / chunk_type` 列（迁移）。
- [ ] 法规/长指引文档走 `DocumentProcessor`（当前为死代码，需接线或删）。

### P1-3 自适应检索 / Self-RAG（性能 + 精度，轻量路由）
- [ ] **条款级路由（确定性）**：`classifyClauses` 后跳过首部/签署页/送达通知等 boilerplate 条款，不检索不审阅。
- [ ] **检索置信度路由**：候选为空或最高分低于阈值时才触发二次泛化检索（去标题、扩 TopK、走 Rerank），而非每条款固定检索两趟。
- [ ] **法律专业问题路由（可选）**：区分"纯审阅规范问题"（付款节点不明）vs"法律专业问题"（违约金上限/竞业期限），后者才触发法规条文级验证。

### P1-4 跨条款去重合并（报告质量）
- [ ] Agent 层按 `(risk_type, 归一化 legal_basis)` 归并多条款命中的同一风险，保留多个 `ClauseID`（当前只在 SSE `stableKey` 层去重）。

### P1-5 真 Reflection（覆盖率，把质量门反馈注入重审）
- [ ] `QualityGate` 的 `ShouldReflect` 目前是死代码。改为：`ShouldRetry && CriticalGaps` 非空时，**只对缺口条款定向重审**（把 gaps 注入 `CandidateRiskAgent` 下一轮），`ReflectionCount++`，超过 `MaxRetries` 强制停；不做全量重审。

### P2 性能与清理
- [ ] 网关语义缓存对 review 跳过（`gateway.go` 中 `Feature == FeatureReview` 跳过 cache lookup/store）。
- [ ] 清理死代码：`RiskAgent`/`react_loop.go`/`rule_verifier_tool.go`/`DocumentProcessor`/`buildEnhancedCandidateQuery` —— 接线或删除，消除文档与代码脱节。

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
