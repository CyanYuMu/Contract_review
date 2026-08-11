# 合同审阅流程链路优化 - 修改计划

## 范围说明

本计划聚焦**审阅流程链路本身**（SSE → Agent 编排 → 前端渲染）的高价值、低风险优化。
以下大型改造**本次不做**，列为后续工作（避免一次性引入过多风险）：
- SSE 任务与请求 ctx 完全解耦 + 断点续传（需新增 task_id 订阅 + Redis 缓冲 + resume 接口，属 API 设计级改动）
- ReAct 循环改造为原生 Function Calling（涉及 react_loop + 全部 agent，风险高）
- Milvus 2.5 原生 BM25（基础设施升级，非代码层）
- 上传时 LLM 解析异步化（触及上传流程）
- RiskCard 预建全文定位索引（RiskCard 大重构）

---

## 一、后端修改

### B1. 知识库按需增量加载（P0）— `app/internal/review/service.go` + `app/internal/knowledge/repo.go`

**问题**：`AgentReviewContract` 每次审阅都调用 `InitOrchestrator`（service.go:962），全量 `LoadKnowledgeChunksFromDB` + 重建 keywordIndex + 重建 retriever/orchestrator。知识库未变时纯属浪费。

**方案**：
1. `knowledge/repo.go` 新增 `KnowledgeSignature(ctx) (string, error)`：对 `review_knowledge_docs`(status=indexed) 与 `review_knowledge_chunks` 做 `COUNT(*) + COALESCE(MAX(updated_at))` 聚合，拼成签名串。查询失败时返回 error（调用方回退到全量加载，保证安全）。
2. `review/service.go` 新增字段 `knowledgeSignature string` + `orchMu sync.Mutex`。
3. `InitOrchestrator` 开头做陈旧判断：`s.orchestrator != nil && s.knowledgeSignature == sig` 时直接 return（复用已构建的 orchestrator）。仅在签名变化或首次时执行全量加载。
4. 全量加载成功后，在 `orchMu` 保护下写入 `s.knowledgeSignature = sig`。
5. 保留原注释意图（"刚配置的风险点能立即进入 RAG"）——签名变化即触发重载，即时性不变。

**风险**：低。并发审阅共享只读 orchestrator 安全；签名查询失败回退原行为。

---

### B2. SSE 心跳 + 失败时显式 error 事件（P0）— `app/internal/review/handler.go`

**问题**：① 长审阅期间无心跳，Next.js 代理/浏览器可能因空闲断连；② 任务失败时只发 `end` 摘要，前端把 `end` 当成功，错误被吞（前端 `finalizeStream` 在 stream `done` 时也调用 `onCompleted`）。

**方案**：
1. 把 `for result := range resultChan` 重构为 `select` 循环，增加 `ticker := time.NewTicker(15s)` 分支，每 15s 写 `: ping\n\n`（SSE 注释行，前端 `processSSEEvent` 已过滤 `:` 开头行，自动忽略）。所有写操作在同一 goroutine 串行，避免并发写 `c`。
2. 任务结束处理改为：`<-doneChan` 收到 `err != nil` → 更新状态 failed + `sendSSEError`（发 `error` 事件）；`err == nil` → 更新状态 completed + `sendSSEMessage(SSEEventEnd, summary)`。前端据此区分成功/失败。

**风险**：低。心跳是注释行不影响解析；error 事件前端已支持（`processDataLine` 已处理 `event:"error"`）。

---

### B3. ClauseAgent 结构化拆分正则容忍 PDF 连续文本（P0）— `app/internal/agent/clause_agent.go`

**问题**：`structuralSplit`（clause_agent.go:84-88）四个正则都用 `(?m)^` 行首锚定。PDF 提取文本常丢失换行/格式，正则匹配不到，退化成整篇一条或走 LLM 兜底。

**方案**：把 `^` 锚定改为 `(?:^|\n|\s)` 容忍匹配（匹配条款编号前为行首/换行/空白），并保持后续 `Index` 取切片逻辑不变。对四个 pattern 统一处理。

**风险**：低。仅在原有匹配基础上放宽，不会破坏已有 DOCX 拆分。

---

### B4. 质量反思：保留单次评估，移除空转重试循环（P1）— `app/internal/agent/orchestrator.go`

**问题**：orchestrator.go:264-294 的 `for retry` 循环：`ShouldReflect` 返回 true 后**没有任何代码重新调用 RiskAgent**，只更新 `ReflectionCount`，下一轮 `Evaluate` 看同一份 report，空转。注释写着"将反馈注入"但未实现。

**方案**：
1. 移除 `for retry` 循环，改为单次 `Evaluate`（保留质量评分 + CriticalGaps，仍通过 SSE 推送给前端，有报告价值）。
2. `InitOrchestrator` 中 `orchConfig.MaxReflectionRetries` 设为 0（service.go:720），去掉 `ReflectionThreshold` 误导配置。
3. `ShouldReflect` 仍保留（供未来真正实现 Reflection 时复用），但编排器不再调用空转循环。

**风险**：低。原本循环就是空转，移除不改变实际审阅结果。

---

### B5. `LLMContractParse` 复用全局 ContractAgent（P2）— `app/internal/agent/service.go`

**问题**：service.go:122-128 每次上传都 `NewContractAgent`（重建 LLM client + 重读 prompt 文件），而启动时 `InitContractAgent` 已建好全局实例（main.go:157 确认调用）。

**方案**：`LLMContractParse` 改为优先 `GetContractAgent()`，仅在全局为 nil 时兜底 `NewContractAgent`（防御性，避免启动顺序假设）。

**风险**：低。

---

### B6. 网关语义缓存对 review 功能跳过（P2）— `app/internal/gateway/gateway.go`

**问题**：审阅每条款 prompt 含不同条款内容，cosine 相似度几乎不可能 >0.92，缓存命中率≈0，反而每次算 embedding。`Chat`/`StreamChat` 无条件走缓存逻辑。

**方案**：`Chat` 与 `StreamChat` 中，`req.Feature == FeatureReview` 时跳过 cache lookup 与 store（直接走限流/配额/模型）。缓存开关仍由配置控制其他功能。

**风险**：低。

---

### B7. 删除旧模式死代码（P2）— `app/internal/review/service.go`

**问题**：`ProcessContractReview`/`splitTextByLength`/`ReviewContract`/`buildReviewPrompt`/`parseReviewResult`/`parseLooseFormat`/`extractField` 已被 `AgentReviewContract` 取代，handler 仅调用新路径。grep 确认无外部调用方。

**方案**：删除上述函数集群（service.go 约 240-612 行的死代码）。**保留** `CalculateOverallRisk`（handler.go:196 在用）、`defaultBasePrompt`（`loadPromptTemplates` 在用）、`InitLLM`/`loadPromptTemplates`。

**风险**：低。已 grep 确认无调用方；保留所有仍被引用的符号。

---

## 二、前端修改

### F1. 移除 `flushSync`（P0）— `frontend/src/components/ReviewPanel.tsx`

**问题**：ReviewPanel.tsx:304 每条风险点 `flushSync(() => addRiskData(risk))` 强制同步渲染，几十条风险点时主线程持续阻塞。

**方案**：改为 `addRiskData(risk)`，交给 React 批处理。`flushSync` import 若无其他用处则一并删除。

**风险**：低。store 已是响应式，批处理不影响数据正确性。

---

### F2. SSE 连接成功后再导航（P0）— `frontend/src/components/ReviewPanel.tsx`

**问题**：ReviewPanel.tsx:292 `router.push("/review")` 在 `startTask` 之前，连接同步失败时用户已跳到空审阅页只剩 toast。

**方案**：把 `router.push("/review")` 移入 `startTask` 的 `onConnected` 回调（startTask.ts:51 在 `response.ok` 后调用）。连接失败时用户留在首页看到错误提示。`resetRiskData`/`setSourceFileUrl`/初始 progress 仍在调用前执行。

**风险**：低。riskStore 全局，导航时机不影响数据接收。

---

### F3. riskStore 优化：去 mergeText + 防抖持久化（P0）— `frontend/src/store/riskStore.ts`

**问题**：① `addRiskData` 的 `mergeText` 重叠拼接逻辑冗余——后端按 stableKey 去重后发送**完整记录**（同 id 是更新而非增量），前端文本拼接既浪费又有错拼风险；② zustand persist 每次 `addRiskData` 同步 `JSON.stringify` 整个 list 写 localStorage，风险点多时阻塞主线程。

**方案**：
1. `addRiskData` 已有 id 改为**整体覆盖**（last-write-wins），移除 `mergeText` 函数及其 4 字段拼接，保留非空字段优先（`risk.x || item.x`）。
2. 自定义 `debouncedStorage`：包装 `localStorage`，`getItem` 同步，`setItem` 用 1s 防抖（trailing）批量写。保留持久化以便刷新恢复，但消除每次更新的同步写。

**风险**：中低。覆盖语义与后端完整记录一致；防抖存储仅延迟写时机，最终一致性不变。

---

### F4. SSE 区分"正常结束"与"意外断流"（P1）— `frontend/src/lib/api/startTask.ts`

**问题**：reader `done` 时无条件 `finalizeStream` → `onCompleted`，后端崩溃断流被当成功。

**方案**：新增 `gotEnd` 标志，收到 `event:"end"` 置 true。`finalizeStream` 中：若 `!gotEnd`（未收到 end 即断流）→ 调 `onError(new Error("审阅连接意外中断"))` 而非 `onCompleted`。

**风险**：低。

---

### F5. 审阅遮罩不阻挡滚动（P1）— `frontend/src/app/review/ReviewPageContent.tsx`

**问题**：审阅中遮罩 `pointerEvents:'all'`（:694）+ 编辑器容器 `pointerEvents:'none'`（:645），1-2 分钟内无法滚动阅读合同。

**方案**：遮罩改为 `pointerEvents:'none'`（仅视觉淡色 tint）；编辑器容器审阅时保留 `pointerEvents:'auto'`（可滚动/选择）。工具栏仍 `none` 防误编辑。即移除 :645 的 `pointerEvents: isReviewing ? 'none' : 'auto'` 中的 none 分支。

**风险**：低。审阅中允许滚动不影响结果流；编辑在完成后才进行。

---

### F6. 删除死代码 `extractWordDOM`（P2）— `frontend/src/components/ReviewPanel.tsx`

**问题**：ReviewPanel.tsx:142-208 `extractWordDOM` 定义但从未调用。

**方案**：删除该函数。

**风险**：低。

---

### F7. 统一编辑器就绪轮询为 `waitFor` 工具（P1）— `frontend/src/app/review/ReviewPageContent.tsx` + 新 util

**问题**：ReviewPageContent 散布 5+ 处 `while + setTimeout` 轮询（容器就绪、`executeImportDocx` 就绪、`executeExportDocx` 就绪）。

**方案**：新增 `frontend/src/utils/waitFor.ts`：`waitFor(predicate, {timeout, interval})` 返回 Promise<boolean>。把主要轮询点（容器就绪 :243、importDocx 就绪 :326、exportDocx 就绪 :496）替换为 `waitFor`。

**风险**：低。等价重构，行为不变。

---

## 三、验证方式

- 后端：`go build ./...` 编译通过；手动跑一次审阅确认 SSE 流正常（心跳不破坏解析、error 事件可触发、知识库复用日志正确）。
- 前端：`npm run build`（或 dev）通过；手动审阅：风险点流式渲染不卡顿、刷新恢复正常、断流报错而非误报完成、审阅中可滚动。
- 无新增单测要求（现有项目无测试基建），以编译 + 手动链路验证为准。

## 修改顺序

后端 B1→B2→B3→B4→B5→B6→B7（编译验证）→ 前端 F1→F2→F3→F4→F5→F6→F7（构建验证）。
