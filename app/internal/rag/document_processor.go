package rag

import (
	"crypto/md5"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// DocumentProcessor 文档处理器 — 将审阅规范/法律法规文档处理为可检索的 Chunks
type DocumentProcessor struct {
	embedder      EmbeddingModel
	maxChunkSize  int
	overlapSize   int
}

// NewDocumentProcessor 创建文档处理器
func NewDocumentProcessor(embedder EmbeddingModel, maxChunkSize, overlapSize int) *DocumentProcessor {
	if maxChunkSize <= 0 {
		maxChunkSize = 500
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

// ProcessDocument 处理单个文档，拆分为 Chunks
func (dp *DocumentProcessor) ProcessDocument(doc Document) ([]Chunk, error) {
	sections := dp.splitBySections(doc.Content)

	var chunks []Chunk
	for i, section := range sections {
		sectionChunks := dp.splitSection(section, doc, i)
		chunks = append(chunks, sectionChunks...)
	}

	if dp.embedder != nil {
		texts := make([]string, len(chunks))
		for i, c := range chunks {
			texts[i] = c.Content
		}

		embeddings, err := dp.embedder.EmbedBatch(texts)
		if err != nil {
			return chunks, fmt.Errorf("Embedding 生成失败: %w", err)
		}

		for i := range chunks {
			if i < len(embeddings) {
				chunks[i].Embedding = embeddings[i]
			}
		}
	}

	return chunks, nil
}

// splitBySections 按章节/条款结构拆分文档
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

// splitSection 对单个章节进一步按大小拆分
func (dp *DocumentProcessor) splitSection(section string, doc Document, sectionIdx int) []Chunk {
	if utf8.RuneCountInString(section) <= dp.maxChunkSize {
		chunkID := fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprintf("%s-%d-0", doc.ID, sectionIdx))))
		return []Chunk{{
			ID:      chunkID[:16],
			DocID:   doc.ID,
			Content: section,
			Metadata: map[string]string{
				"title":        doc.Title,
				"category":     doc.Category,
				"sub_category": doc.SubCategory,
				"source":       doc.Source,
				"section_idx":  fmt.Sprintf("%d", sectionIdx),
			},
		}}
	}

	runes := []rune(section)
	var chunks []Chunk
	chunkIdx := 0

	for start := 0; start < len(runes); {
		end := start + dp.maxChunkSize
		if end > len(runes) {
			end = len(runes)
		}

		if end < len(runes) {
			for i := end; i > start+dp.maxChunkSize/2; i-- {
				r := runes[i]
				if r == '。' || r == '；' || r == '\n' || r == '.' {
					end = i + 1
					break
				}
			}
		}

		chunkContent := string(runes[start:end])
		chunkID := fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprintf("%s-%d-%d", doc.ID, sectionIdx, chunkIdx))))

		chunks = append(chunks, Chunk{
			ID:      chunkID[:16],
			DocID:   doc.ID,
			Content: chunkContent,
			Metadata: map[string]string{
				"title":        doc.Title,
				"category":     doc.Category,
				"sub_category": doc.SubCategory,
				"source":       doc.Source,
				"section_idx":  fmt.Sprintf("%d", sectionIdx),
				"chunk_idx":    fmt.Sprintf("%d", chunkIdx),
			},
		})

		start = end - dp.overlapSize
		if start < 0 {
			start = 0
		}
		if start >= len(runes) {
			break
		}
		chunkIdx++
	}

	return chunks
}

// ProcessDocuments 批量处理文档
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
