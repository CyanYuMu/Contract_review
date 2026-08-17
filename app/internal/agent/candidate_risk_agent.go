package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"contract_review/app/internal/global"
	"contract_review/app/internal/rag"

	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

type CandidateRiskAgent struct {
	llmGenerate func(ctx context.Context, messages []*schema.Message) (*schema.Message, error)
	retriever   *rag.RAGRetriever
	config      CandidateRiskConfig
}

type CandidateRiskConfig struct {
	CandidateTopK        int
	ReviewBatchSize      int
	MaxConcurrentBatches int
}

type RiskCandidate struct {
	ID                  string   `json:"id"`
	RiskType            string   `json:"risk_type"`
	RiskLevel           string   `json:"risk_level"`
	TriggerCondition    string   `json:"trigger_condition"`
	Keywords            []string `json:"keywords"`
	ApplicableClauses   []string `json:"applicable_clauses"`
	LegalBasis          string   `json:"legal_basis"`
	RecommendedTemplate string   `json:"recommended_template"`
	RiskContent         string   `json:"risk_content"`
	Content             string   `json:"content"`
	Source              string   `json:"source"`
	Category            string   `json:"category"`
	Score               float64  `json:"score"`
}

type CandidateClauseResult struct {
	Index      int
	Clause     Clause
	Candidates []RiskCandidate
	Findings   []RiskFinding
}

type candidateClauseSet struct {
	index      int
	clause     Clause
	candidates []RiskCandidate
	err        error
}

func NewCandidateRiskAgent(
	llmGenerate func(ctx context.Context, messages []*schema.Message) (*schema.Message, error),
	retriever *rag.RAGRetriever,
	config CandidateRiskConfig,
) *CandidateRiskAgent {
	if config.CandidateTopK <= 0 {
		config.CandidateTopK = 8
	}
	if config.ReviewBatchSize <= 0 {
		config.ReviewBatchSize = 3
	}
	if config.MaxConcurrentBatches <= 0 {
		config.MaxConcurrentBatches = 2
	}
	return &CandidateRiskAgent{
		llmGenerate: llmGenerate,
		retriever:   retriever,
		config:      config,
	}
}

func (a *CandidateRiskAgent) ExecuteBatchWithCallback(
	ctx context.Context,
	clauses []Clause,
	meta ContractMeta,
	onCandidates func(index int, clause Clause, candidates []RiskCandidate, completed int, total int),
	onClauseResult func(index int, clause Clause, candidates []RiskCandidate, findings []RiskFinding, completed int, total int),
) ([]RiskFinding, []ThinkStep, error) {
	if len(clauses) == 0 {
		return nil, nil, nil
	}

	candidateSets := a.retrieveCandidates(ctx, clauses, meta, onCandidates)
	sort.Slice(candidateSets, func(i, j int) bool {
		return candidateSets[i].index < candidateSets[j].index
	})

	batches := splitCandidateSets(candidateSets, a.config.ReviewBatchSize)
	type batchResult struct {
		batchIndex int
		findings   []RiskFinding
		steps      []ThinkStep
		err        error
	}

	resultChan := make(chan batchResult, len(batches))
	semaphore := make(chan struct{}, a.config.MaxConcurrentBatches)

	for batchIndex, batch := range batches {
		go func(idx int, items []candidateClauseSet) {
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			findings, step, err := a.reviewCandidateBatch(ctx, idx, items, meta)
			resultChan <- batchResult{
				batchIndex: idx,
				findings:   findings,
				steps:      []ThinkStep{step},
				err:        err,
			}
		}(batchIndex, batch)
	}

	findingsByClause := make(map[string][]RiskFinding)
	var allSteps []ThinkStep
	for i := 0; i < len(batches); i++ {
		result := <-resultChan
		if result.err != nil {
			global.Log.Warn("候选驱动批量风险审阅失败",
				zap.Int("batchIndex", result.batchIndex),
				zap.Error(result.err))
			continue
		}
		allSteps = append(allSteps, result.steps...)
		for _, finding := range result.findings {
			findingsByClause[finding.ClauseID] = append(findingsByClause[finding.ClauseID], finding)
		}
	}

	var allFindings []RiskFinding
	completed := 0
	for _, set := range candidateSets {
		completed++
		clauseFindings := findingsByClause[set.clause.ID]
		if onClauseResult != nil {
			onClauseResult(set.index, set.clause, set.candidates, clauseFindings, completed, len(clauses))
		}
		allFindings = append(allFindings, clauseFindings...)
	}

	return allFindings, allSteps, nil
}

func (a *CandidateRiskAgent) retrieveCandidates(
	ctx context.Context,
	clauses []Clause,
	meta ContractMeta,
	onCandidates func(index int, clause Clause, candidates []RiskCandidate, completed int, total int),
) []candidateClauseSet {
	resultChan := make(chan candidateClauseSet, len(clauses))
	maxConcurrent := a.config.MaxConcurrentBatches * 3
	if maxConcurrent < 4 {
		maxConcurrent = 4
	}
	semaphore := make(chan struct{}, maxConcurrent)

	for i, clause := range clauses {
		go func(idx int, c Clause) {
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			candidates, err := a.retrieveClauseCandidates(ctx, c, meta)
			resultChan <- candidateClauseSet{
				index:      idx,
				clause:     c,
				candidates: candidates,
				err:        err,
			}
		}(i, clause)
	}

	results := make([]candidateClauseSet, 0, len(clauses))
	for i := 0; i < len(clauses); i++ {
		result := <-resultChan
		if result.err != nil {
			global.Log.Warn("条款候选风险检索失败",
				zap.Int("index", result.index),
				zap.String("clauseID", result.clause.ID),
				zap.Error(result.err))
		}
		completed := i + 1
		if onCandidates != nil {
			onCandidates(result.index, result.clause, result.candidates, completed, len(clauses))
		}
		results = append(results, result)
	}
	return results
}

func (a *CandidateRiskAgent) retrieveClauseCandidates(ctx context.Context, clause Clause, meta ContractMeta) ([]RiskCandidate, error) {
	if a.retriever == nil {
		return nil, nil
	}
	query := buildCandidateQuery(clause, meta)
	var results []rag.SearchResult
	var err error
	if strings.TrimSpace(meta.ContractType) != "" {
		results, err = a.retriever.LayeredRetrieve(query, meta.ContractType)
	} else {
		results, err = a.retriever.Retrieve(query, nil)
	}
	if err != nil {
		return nil, err
	}
	candidates := make([]RiskCandidate, 0, len(results))
	seen := make(map[string]bool)
	for _, result := range results {
		candidate := riskCandidateFromSearchResult(result)
		if candidate.ID == "" {
			candidate.ID = result.ChunkID
		}
		if seen[candidate.ID] {
			continue
		}
		seen[candidate.ID] = true
		candidates = append(candidates, candidate)
		if len(candidates) >= a.config.CandidateTopK {
			break
		}
	}
	return candidates, nil
}

func (a *CandidateRiskAgent) reviewCandidateBatch(ctx context.Context, batchIndex int, sets []candidateClauseSet, meta ContractMeta) ([]RiskFinding, ThinkStep, error) {
	start := time.Now()
	prompt := buildCandidateRiskPrompt(sets, meta)
	messages := []*schema.Message{
		{
			Role:    schema.System,
			Content: candidateRiskSystemPrompt,
		},
		{
			Role:    schema.User,
			Content: prompt,
		},
	}
	resp, err := a.llmGenerate(ctx, messages)
	step := ThinkStep{
		Iteration:   batchIndex + 1,
		Thought:     "候选驱动批量风险审阅",
		Action:      "llm_batch_review",
		ActionInput: fmt.Sprintf("clauses=%d", len(sets)),
		Timestamp:   time.Now(),
	}
	if err != nil {
		step.Observation = err.Error()
		return nil, step, err
	}
	step.Observation = truncateText(resp.Content, 1200)

	findings := parseCandidateRiskFindings(resp.Content, sets)
	global.Log.Info("候选驱动批量风险审阅完成",
		zap.Int("batchIndex", batchIndex),
		zap.Int("clauseCount", len(sets)),
		zap.Int("findingCount", len(findings)),
		zap.Duration("duration", time.Since(start)))
	return findings, step, nil
}

func splitCandidateSets(items []candidateClauseSet, batchSize int) [][]candidateClauseSet {
	if batchSize <= 0 {
		batchSize = 3
	}
	batches := make([][]candidateClauseSet, 0, (len(items)+batchSize-1)/batchSize)
	for start := 0; start < len(items); start += batchSize {
		end := start + batchSize
		if end > len(items) {
			end = len(items)
		}
		batches = append(batches, items[start:end])
	}
	return batches
}

func buildCandidateQuery(clause Clause, meta ContractMeta) string {
	parts := []string{
		meta.ContractType,
		meta.Stance,
		clause.Title,
		clause.Category,
		truncateText(clause.Content, 1200),
	}
	return strings.Join(nonEmptyStrings(parts), "\n")
}

func riskCandidateFromSearchResult(result rag.SearchResult) RiskCandidate {
	content := result.Content
	md := result.Metadata
	if md == nil {
		md = map[string]string{}
	}
	// 结构化元数据优先；旧数据（无 metadata）回退到从拍扁文本中反解字段。
	field := func(metaKey, contentField string) string {
		if v := strings.TrimSpace(md[metaKey]); v != "" {
			return v
		}
		return extractKnowledgeField(content, contentField)
	}

	candidate := RiskCandidate{
		ID:                  firstNonEmpty(field("risk_id", "风险点ID"), md["vector_id"], result.ChunkID),
		RiskType:            field("risk_type", "风险类型"),
		RiskLevel:           normalizeRiskLevel(field("risk_level", "风险等级")),
		TriggerCondition:    field("trigger_condition", "触发条件"),
		Keywords:            splitListField(field("keywords", "关键词")),
		ApplicableClauses:   splitListField(field("applicable_clauses", "适用条款")),
		LegalBasis:          field("legal_basis", "法律依据"),
		RecommendedTemplate: field("recommended_template", "推荐修改模板"),
		RiskContent:         field("risk_content", "风险内容"),
		Content:             truncateText(content, 900),
		Source:              firstNonEmpty(result.Source, md["source"], md["title"]),
		Category:            md["category"],
		Score:               result.Score,
	}
	if candidate.RiskLevel == "" {
		candidate.RiskLevel = "中"
	}
	return candidate
}

func buildCandidateRiskPrompt(sets []candidateClauseSet, meta ContractMeta) string {
	type promptClause struct {
		ID         string          `json:"id"`
		Title      string          `json:"title"`
		Category   string          `json:"category"`
		Content    string          `json:"content"`
		Candidates []RiskCandidate `json:"candidates"`
	}
	payload := struct {
		ContractMeta ContractMeta   `json:"contract_meta"`
		Clauses      []promptClause `json:"clauses"`
		OutputSchema string         `json:"output_schema"`
	}{
		ContractMeta: meta,
		Clauses:      make([]promptClause, 0, len(sets)),
		OutputSchema: `{"findings":[{"finding_id":"clause-id-risk-1","clause_id":"条款ID","candidate_ids":["候选ID"],"risk_type":"风险类型","risk_level":"高/中/低","risk_description":"风险描述","original_text":"原文摘录","legal_basis":[{"source":"来源","article":"条款/风险点ID","content":"依据摘要","relevance":0.8}],"verified":true,"requires_human_review":false,"confidence":0.8,"suggested_text":"可直接替换或补充的条款文本","suggestion_reason":"修改理由","priority":"必须修改/建议修改/可选修改"}]}`,
	}
	for _, set := range sets {
		payload.Clauses = append(payload.Clauses, promptClause{
			ID:         set.clause.ID,
			Title:      set.clause.Title,
			Category:   set.clause.Category,
			Content:    truncateText(set.clause.Content, 2600),
			Candidates: set.candidates,
		})
	}
	bytes, _ := json.Marshal(payload)
	return string(bytes)
}

type rawCandidateRiskFinding struct {
	FindingID           string       `json:"finding_id"`
	ClauseID            string       `json:"clause_id"`
	CandidateIDs        []string     `json:"candidate_ids"`
	RiskType            string       `json:"risk_type"`
	RiskLevel           string       `json:"risk_level"`
	RiskDescription     string       `json:"risk_description"`
	Description         string       `json:"description"`
	OriginalText        string       `json:"original_text"`
	LegalBasis          []LegalBasis `json:"legal_basis"`
	Verified            bool         `json:"verified"`
	RequiresHumanReview bool         `json:"requires_human_review"`
	Confidence          float64      `json:"confidence"`
	SuggestedText       string       `json:"suggested_text"`
	SuggestionReason    string       `json:"suggestion_reason"`
	Priority            string       `json:"priority"`
}

func parseCandidateRiskFindings(text string, sets []candidateClauseSet) []RiskFinding {
	jsonStr := extractJSON(text)
	if jsonStr == "" {
		jsonStr = extractJSONArray(text)
	}
	if jsonStr == "" {
		return nil
	}

	var rawItems []rawCandidateRiskFinding
	if strings.HasPrefix(strings.TrimSpace(jsonStr), "[") {
		if err := json.Unmarshal([]byte(jsonStr), &rawItems); err != nil {
			return nil
		}
	} else {
		var wrapper struct {
			Findings []rawCandidateRiskFinding `json:"findings"`
			Risks    []rawCandidateRiskFinding `json:"risks"`
		}
		if err := json.Unmarshal([]byte(jsonStr), &wrapper); err != nil {
			return nil
		}
		if len(wrapper.Findings) > 0 {
			rawItems = wrapper.Findings
		} else {
			rawItems = wrapper.Risks
		}
	}

	candidateMap := mapCandidatesByClause(sets)
	clauseSet := make(map[string]bool)
	for _, set := range sets {
		clauseSet[set.clause.ID] = true
	}

	findings := make([]RiskFinding, 0, len(rawItems))
	sequenceByClause := make(map[string]int)
	for _, raw := range rawItems {
		clauseID := strings.TrimSpace(raw.ClauseID)
		if !clauseSet[clauseID] {
			continue
		}
		desc := firstNonEmpty(raw.RiskDescription, raw.Description)
		if strings.TrimSpace(desc) == "" && strings.TrimSpace(raw.OriginalText) == "" {
			continue
		}
		sequenceByClause[clauseID]++
		findingID := raw.FindingID
		if findingID == "" {
			findingID = fmt.Sprintf("%s-risk-%d", clauseID, sequenceByClause[clauseID])
		}

		legalBasis := normalizeLegalBasis(raw.LegalBasis, raw.CandidateIDs, candidateMap[clauseID])
		verified := raw.Verified && len(legalBasis) > 0
		// 确定性护栏：LLM 声称已验证但自评置信度过低（且确实给出了分数）→ 降级为待人工确认。
		if verified && raw.Confidence > 0 && raw.Confidence < 0.3 {
			verified = false
		}
		requiresHumanReview := raw.RequiresHumanReview || !verified
		findings = append(findings, RiskFinding{
			FindingID:           findingID,
			ClauseID:            clauseID,
			CandidateIDs:        cleanCandidateIDs(raw.CandidateIDs),
			RiskType:            limitText(firstNonEmpty(raw.RiskType, "待人工确认风险"), 64),
			RiskLevel:           normalizeRiskLevel(raw.RiskLevel),
			RiskDescription:     strings.TrimSpace(desc),
			OriginalText:        limitText(raw.OriginalText, 300),
			LegalBasis:          legalBasis,
			Verified:            verified,
			RequiresHumanReview: requiresHumanReview,
			Confidence:          clampConfidence(raw.Confidence),
			SuggestedText:       strings.TrimSpace(raw.SuggestedText),
			SuggestionReason:    strings.TrimSpace(raw.SuggestionReason),
			Priority:            normalizePriority(raw.Priority, raw.RiskLevel),
		})
	}
	return findings
}

func mapCandidatesByClause(sets []candidateClauseSet) map[string]map[string]RiskCandidate {
	out := make(map[string]map[string]RiskCandidate)
	for _, set := range sets {
		out[set.clause.ID] = make(map[string]RiskCandidate)
		for _, candidate := range set.candidates {
			out[set.clause.ID][candidate.ID] = candidate
		}
	}
	return out
}

// normalizeLegalBasis 归一化法律/审阅依据，只保留"可定位、有实质内容"的依据。
// 目的：避免把"未配置"等占位文本或整段拍扁模板当作依据，导致未经验证的风险被误判为已验证。
func normalizeLegalBasis(raw []LegalBasis, candidateIDs []string, candidates map[string]RiskCandidate) []LegalBasis {
	out := make([]LegalBasis, 0, len(raw)+len(candidateIDs))
	seen := make(map[string]bool)

	// 1. LLM 直接输出的依据：内容有效，且来源或条款编号至少一个能定位。
	for _, basis := range raw {
		if !isCredibleBasis(basis.Source, basis.Article, basis.Content) {
			continue
		}
		key := basis.Source + "|" + basis.Article + "|" + basis.Content
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, LegalBasis{
			Source:    limitText(basis.Source, 120),
			Article:   limitText(basis.Article, 120),
			Content:   limitText(basis.Content, 500),
			Relevance: clampConfidence(basis.Relevance),
		})
	}

	// 2. 候选风险点派生的依据：优先法律依据，其次风险内容（审阅规范依据），最后纯文本审阅指引。
	for _, id := range candidateIDs {
		candidate, ok := candidates[id]
		if !ok {
			continue
		}
		content, source, article := candidateBasisFields(candidate)
		if content == "" {
			continue
		}
		key := source + "|" + article + "|" + content
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, LegalBasis{
			Source:    source,
			Article:   article,
			Content:   limitText(content, 500),
			Relevance: candidate.Score,
		})
	}
	return out
}

// candidateBasisFields 返回候选风险点可作为"依据"的 (内容, 来源, 条款)。
// 优先级：法律依据 > 风险内容（审阅规范依据）> 纯文本审阅指引（种子/内置知识）。
// 显式排除"【风险点配置】"这类拍扁模板，避免把它当依据。
func candidateBasisFields(candidate RiskCandidate) (content, source, article string) {
	src := candidateSource(candidate)
	ref := firstNonEmpty(candidate.RiskType, candidate.ID)

	if isMeaningfulBasis(candidate.LegalBasis) {
		return candidate.LegalBasis, src, ref
	}
	if isMeaningfulBasis(candidate.RiskContent) {
		return candidate.RiskContent, src, ref
	}
	trimmed := strings.TrimSpace(candidate.Content)
	if isMeaningfulBasis(trimmed) && !strings.HasPrefix(trimmed, "【风险点配置】") {
		return trimmed, src, ref
	}
	return "", "", ""
}

// candidateSource 返回候选风险点的来源标识。
func candidateSource(candidate RiskCandidate) string {
	return firstNonEmpty(candidate.Source, candidate.Category, "审阅知识库")
}

// isCredibleBasis 判断 LLM 输出的依据是否可信：内容有效，且来源或条款编号至少一个能定位（非占位）。
func isCredibleBasis(source, article, content string) bool {
	if !isMeaningfulBasis(content) {
		return false
	}
	if !isMeaningfulBasis(source) && !isMeaningfulBasis(article) {
		return false
	}
	return true
}

// placeholderBasis 常见的"未配置"占位文本，不构成有效依据。
var placeholderBasis = map[string]bool{
	"未配置": true, "无": true, "暂无": true, "待补全": true, "待补充": true,
	"n/a": true, "na": true, "null": true, "none": true, "-": true, "—": true,
}

// isMeaningfulBasis 判断依据内容是否有实质含义（非空、非占位、长度足够）。
func isMeaningfulBasis(s string) bool {
	t := strings.ToLower(strings.TrimSpace(s))
	if t == "" {
		return false
	}
	if placeholderBasis[t] {
		return false
	}
	if utf8.RuneCountInString(t) <= 1 {
		return false
	}
	return true
}

func extractKnowledgeField(content, field string) string {
	lines := strings.Split(content, "\n")
	prefixes := []string{field + "：", field + ":"}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		for _, prefix := range prefixes {
			if strings.HasPrefix(line, prefix) {
				return strings.TrimSpace(strings.TrimPrefix(line, prefix))
			}
		}
	}
	return ""
}

func splitListField(value string) []string {
	if strings.TrimSpace(value) == "" || strings.TrimSpace(value) == "未配置" {
		return nil
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '，' || r == '、' || r == ';' || r == '；' || r == '\n'
	})
	return cleanCandidateIDs(parts)
}

func cleanCandidateIDs(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, limitText(value, 128))
	}
	return out
}

func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func truncateText(value string, max int) string {
	return limitText(value, max)
}

func limitText(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || utf8.RuneCountInString(value) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max])
}

func clampConfidence(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func normalizePriority(priority, riskLevel string) string {
	if strings.Contains(priority, "必须") {
		return "必须修改"
	}
	if strings.Contains(priority, "可选") {
		return "可选修改"
	}
	if strings.Contains(priority, "建议") {
		return "建议修改"
	}
	switch normalizeRiskLevel(riskLevel) {
	case "高":
		return "必须修改"
	case "低":
		return "可选修改"
	default:
		return "建议修改"
	}
}

func candidateSources(candidates []RiskCandidate, max int) []string {
	if max <= 0 {
		max = 3
	}
	seen := make(map[string]bool)
	sources := make([]string, 0, max)
	for _, candidate := range candidates {
		source := firstNonEmpty(candidate.Source, candidate.Category, candidate.ID)
		if source == "" || seen[source] {
			continue
		}
		seen[source] = true
		sources = append(sources, source)
		if len(sources) >= max {
			break
		}
	}
	return sources
}

func candidateIDs(candidates []RiskCandidate, max int) []string {
	if max <= 0 {
		max = 5
	}
	ids := make([]string, 0, max)
	for _, candidate := range candidates {
		if candidate.ID == "" {
			continue
		}
		ids = append(ids, candidate.ID)
		if len(ids) >= max {
			break
		}
	}
	return ids
}

const candidateRiskSystemPrompt = `你是一名资深合同审查律师。当前审阅采用知识库候选驱动 DAG，不要请求外部工具，也不要声称已经执行新的检索。

工作原则：
1. 先判断每个条款是否触发候选风险点的触发条件、关键词、适用条款或风险内容。
2. verified=true 仅用于有候选风险点、审阅规范或法律依据支持的风险；legal_basis 必须引用对应 candidate_id 或候选依据。
3. 如果候选未命中但条款仍存在明显法律、履约或表述风险，可以输出 verified=false、requires_human_review=true 的“待人工确认风险”。
4. 修改建议优先使用候选中的推荐修改模板；没有模板时给出可直接替换或补充的条款文本。
5. 只输出 JSON，不输出解释性前后缀。`
