package rag

import (
	"crypto/md5"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// DocumentProcessor 文档处理器 —— 将审阅规范/法律法规文档处理为父子分块：
//   - child 分块（maxChunkSize 大小、带 overlap）用于向量/关键词检索；
//   - parent 分块（完整章节/条款）不参与检索，供 child 命中后回填上下文。
//
// 设计理念（参考 WeKnora parent-child chunking）：小块负责召回精度，大块负责上下文完整性。
type DocumentProcessor struct {
	embedder     EmbeddingModel
	maxChunkSize int
	overlapSize  int
}

// NewDocumentProcessor 创建文档处理器。
func NewDocumentProcessor(embedder EmbeddingModel, maxChunkSize, overlapSize int) *DocumentProcessor {
	if maxChunkSize <= 0 {
		maxChunkSize = 300
	}
	if overlapSize <= 0 {
		overlapSize = 50
	}
	return &DocumentProcessor{
		embedder:     embedder,
		maxChunkSize: maxChunkSize,
		overlapSize:  overlapSize,
	}
}

// ProcessDocument 处理单个文档，产出父子分块。
func (dp *DocumentProcessor) ProcessDocument(doc Document) ([]Chunk, error) {
	sections := dp.splitBySections(doc.Content)
	if len(sections) == 0 {
		sections = []string{doc.Content}
	}

	var chunks []Chunk
	for i, section := range sections {
		parent, children := dp.splitSectionWithParent(section, doc, i)
		if parent != nil {
			chunks = append(chunks, *parent)
		}
		chunks = append(chunks, children...)
	}

	// 仅对 child 分块生成 embedding（parent 不参与检索）。
	if dp.embedder != nil {
		childIdx := make([]int, 0, len(chunks))
		texts := make([]string, 0, len(chunks))
		for i, c := range chunks {
			if c.ChunkType != ChunkTypeParent {
				childIdx = append(childIdx, i)
				texts = append(texts, c.EmbeddingContent())
			}
		}
		if len(texts) > 0 {
			embeddings, err := dp.embedder.EmbedBatch(texts)
			if err != nil {
				return chunks, fmt.Errorf("Embedding 生成失败: %w", err)
			}
			for j, i := range childIdx {
				if j < len(embeddings) {
					chunks[i].Embedding = embeddings[j]
				}
			}
		}
	}

	return chunks, nil
}

// splitBySections 按章节/条款结构拆分文档。
func (dp *DocumentProcessor) splitBySections(content string) []string {
	sectionPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?m)^第[一二三四五六七八九十百千\d]+[章节条款编]`),
		regexp.MustCompile(`(?m)^[一二三四五六七八九十]+[、.]`),
		regexp.MustCompile(`(?m)^\d+[.、]\s`),
		regexp.MustCompile(`(?m)^#+\s`),
	}

	for _, pattern := range sectionPatterns {
		indices := pattern.FindAllStringIndex(content, -1)
		if len(indices) >= 2 {
			var sections []string
			for i, idx := range indices {
				var end int
				if i < len(indices)-1 {
					end = indices[i+1][0]
				} else {
					end = len(content)
				}
				section := strings.TrimSpace(content[idx[0]:end])
				if section != "" {
					sections = append(sections, section)
				}
			}
			if len(sections) > 0 {
				return sections
			}
		}
	}

	return []string{content}
}

// splitSectionWithParent 将一个章节拆分为一个 parent 分块 + 若干 child 分块。
// 章节较短（<= maxChunkSize）时直接作为单个 child（并附 parent 指向完整章节）。
func (dp *DocumentProcessor) splitSectionWithParent(section string, doc Document, sectionIdx int) (*Chunk, []Chunk) {
	parentID := dp.chunkID(doc.ID, sectionIdx, 0, true)
	parent := &Chunk{
		ID:        parentID,
		DocID:     doc.ID,
		Content:   section,
		Metadata:  dp.baseMetadata(doc, sectionIdx, -1),
		ChunkType: ChunkTypeParent,
	}

	// 短章节：单 child 即可，无需切子块。
	if utf8.RuneCountInString(section) <= dp.maxChunkSize {
		child := *parent
		child.ID = dp.chunkID(doc.ID, sectionIdx, 0, false)
		child.Metadata = dp.baseMetadata(doc, sectionIdx, 0)
		child.ParentChunkID = parentID
		child.ChunkType = ChunkTypeChild
		return parent, []Chunk{child}
	}

	runes := []rune(section)
	var children []Chunk
	chunkIdx := 0
	for start := 0; start < len(runes); {
		end := start + dp.maxChunkSize
		if end > len(runes) {
			end = len(runes)
		}

		if end < len(runes) {
			// 尽量在句末/段末断句，避免切断语义。
			for i := end; i > start+dp.maxChunkSize/2; i-- {
				r := runes[i]
				if r == '。' || r == '；' || r == '\n' || r == '.' {
					end = i + 1
					break
				}
			}
		}

		children = append(children, Chunk{
			ID:            dp.chunkID(doc.ID, sectionIdx, chunkIdx, false),
			DocID:         doc.ID,
			Content:       string(runes[start:end]),
			Metadata:      dp.baseMetadata(doc, sectionIdx, chunkIdx),
			ParentChunkID: parentID,
			ChunkType:     ChunkTypeChild,
		})

		// 已切到末尾则结束；否则按 overlap 回退形成重叠窗口，并保证 start 单调前进。
		if end >= len(runes) {
			break
		}
		next := end - dp.overlapSize
		if next <= start {
			next = start + 1
		}
		start = next
		chunkIdx++
	}

	return parent, children
}

// chunkID 生成稳定分块 ID（16 位十六进制）。
func (dp *DocumentProcessor) chunkID(docID string, sectionIdx, chunkIdx int, parent bool) string {
	kind := "child"
	if parent {
		kind = "parent"
	}
	sum := md5.Sum([]byte(fmt.Sprintf("%s-%s-%d-%d", docID, kind, sectionIdx, chunkIdx)))
	return fmt.Sprintf("%x", sum)[:16]
}

// baseMetadata 构建分块基础元数据。
func (dp *DocumentProcessor) baseMetadata(doc Document, sectionIdx, chunkIdx int) map[string]string {
	m := map[string]string{
		"title":        doc.Title,
		"category":     doc.Category,
		"sub_category": doc.SubCategory,
		"source":       doc.Source,
		"section_idx":  fmt.Sprintf("%d", sectionIdx),
	}
	if chunkIdx >= 0 {
		m["chunk_idx"] = fmt.Sprintf("%d", chunkIdx)
	}
	return m
}

// ProcessDocuments 批量处理文档。
func (dp *DocumentProcessor) ProcessDocuments(docs []Document) ([]Chunk, error) {
	var allChunks []Chunk
	for _, doc := range docs {
		chunks, err := dp.ProcessDocument(doc)
		if err != nil {
			return allChunks, fmt.Errorf("处理文档 %s 失败: %w", doc.Title, err)
		}
		allChunks = append(allChunks, chunks...)
	}
	return allChunks, nil
}
