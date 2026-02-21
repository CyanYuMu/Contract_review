package utils

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"contract_review/app/internal/global"

	"go.uber.org/zap"
)

// ExtractText 从文件中提取文本内容
// 支持 PDF 和 DOCX 格式
func ExtractText(filePath string) (string, error) {
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".pdf":
		return ExtractTextFromPDF(filePath)
	case ".docx":
		return ExtractTextFromDOCX(filePath)
	case ".doc":
		return "", errors.New("不支持 .doc 格式，请转换为 .docx")
	case ".txt":
		return ExtractTextFromTXT(filePath)
	default:
		return "", fmt.Errorf("不支持的文件格式: %s", ext)
	}
}

// ExtractTextFromTXT 从纯文本文件提取内容
func ExtractTextFromTXT(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}
	return string(data), nil
}

// ExtractTextFromDOCX 从DOCX文件提取文本
// DOCX是一个ZIP压缩包，包含word/document.xml
func ExtractTextFromDOCX(filePath string) (string, error) {
	// 打开DOCX文件（实际上是ZIP格式）
	reader, err := zip.OpenReader(filePath)
	if err != nil {
		global.Log.Error("打开DOCX文件失败", zap.Error(err))
		return "", fmt.Errorf("打开DOCX文件失败: %w", err)
	}
	defer reader.Close()

	// 查找document.xml
	var documentFile *zip.File
	for _, file := range reader.File {
		if file.Name == "word/document.xml" {
			documentFile = file
			break
		}
	}

	if documentFile == nil {
		return "", errors.New("DOCX文件格式错误：未找到document.xml")
	}

	// 读取document.xml内容
	rc, err := documentFile.Open()
	if err != nil {
		return "", fmt.Errorf("读取document.xml失败: %w", err)
	}
	defer rc.Close()

	xmlData, err := io.ReadAll(rc)
	if err != nil {
		return "", fmt.Errorf("读取XML内容失败: %w", err)
	}

	// 解析XML提取文本
	text, err := parseDocxXML(xmlData)
	if err != nil {
		return "", fmt.Errorf("解析DOCX XML失败: %w", err)
	}

	global.Log.Info("DOCX文本提取成功", zap.Int("length", len(text)))
	return text, nil
}

// parseDocxXML 解析DOCX的XML内容提取文本
func parseDocxXML(xmlData []byte) (string, error) {
	// DOCX XML结构中，文本在 <w:t> 标签中
	type Text struct {
		Content string `xml:",chardata"`
	}

	type Run struct {
		Text []Text `xml:"t"`
	}

	type Paragraph struct {
		Runs []Run `xml:"r"`
	}

	type Body struct {
		Paragraphs []Paragraph `xml:"p"`
	}

	type Document struct {
		Body Body `xml:"body"`
	}

	var doc Document
	decoder := xml.NewDecoder(bytes.NewReader(xmlData))
	decoder.Strict = false

	if err := decoder.Decode(&doc); err != nil {
		// 如果标准解析失败，使用正则表达式提取
		return extractTextByRegex(xmlData), nil
	}

	var textBuilder strings.Builder
	for _, para := range doc.Body.Paragraphs {
		for _, run := range para.Runs {
			for _, t := range run.Text {
				textBuilder.WriteString(t.Content)
			}
		}
		textBuilder.WriteString("\n")
	}

	result := textBuilder.String()
	if strings.TrimSpace(result) == "" {
		// 如果结构化解析没有内容，使用正则表达式
		return extractTextByRegex(xmlData), nil
	}

	return result, nil
}

// extractTextByRegex 使用正则表达式从XML中提取文本
func extractTextByRegex(xmlData []byte) string {
	// 匹配 <w:t>...</w:t> 或 <w:t ...>...</w:t>
	re := regexp.MustCompile(`<w:t[^>]*>([^<]*)</w:t>`)
	matches := re.FindAllSubmatch(xmlData, -1)

	var textBuilder strings.Builder
	for _, match := range matches {
		if len(match) > 1 {
			textBuilder.Write(match[1])
		}
	}

	// 处理段落标签添加换行
	result := textBuilder.String()

	// 清理多余空白
	result = regexp.MustCompile(`\s+`).ReplaceAllString(result, " ")
	result = strings.TrimSpace(result)

	return result
}

// ExtractTextFromPDF 从PDF文件提取文本
// 这是一个简化的PDF文本提取实现
// 对于复杂PDF可能需要使用专业库如 pdfcpu 或 unidoc
func ExtractTextFromPDF(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		global.Log.Error("读取PDF文件失败", zap.Error(err))
		return "", fmt.Errorf("读取PDF文件失败: %w", err)
	}

	// 简单的PDF文本提取
	// PDF文件中的文本通常在 stream 中以 BT...ET 块包裹
	text := extractPDFText(data)

	if strings.TrimSpace(text) == "" {
		global.Log.Warn("PDF文本提取为空，可能是扫描版PDF或加密PDF")
		return "", errors.New("无法从PDF中提取文本，可能是扫描版或加密的PDF")
	}

	global.Log.Info("PDF文本提取成功", zap.Int("length", len(text)))
	return text, nil
}

// extractPDFText 从PDF二进制数据中提取文本
func extractPDFText(data []byte) string {
	content := string(data)
	var textBuilder strings.Builder

	// 方法1: 提取 BT...ET 文本块
	btPattern := regexp.MustCompile(`BT\s*(.*?)\s*ET`)
	btMatches := btPattern.FindAllStringSubmatch(content, -1)

	for _, match := range btMatches {
		if len(match) > 1 {
			// 提取 Tj 或 TJ 操作符中的文本
			tjPattern := regexp.MustCompile(`\((.*?)\)\s*Tj`)
			tjMatches := tjPattern.FindAllStringSubmatch(match[1], -1)
			for _, tj := range tjMatches {
				if len(tj) > 1 {
					// 解码PDF字符串中的转义序列
					text := decodePDFString(tj[1])
					textBuilder.WriteString(text)
				}
			}

			// TJ数组格式
			tjArrayPattern := regexp.MustCompile(`\[(.*?)\]\s*TJ`)
			tjArrayMatches := tjArrayPattern.FindAllStringSubmatch(match[1], -1)
			for _, tja := range tjArrayMatches {
				if len(tja) > 1 {
					// 提取数组中的字符串
					strPattern := regexp.MustCompile(`\((.*?)\)`)
					strMatches := strPattern.FindAllStringSubmatch(tja[1], -1)
					for _, s := range strMatches {
						if len(s) > 1 {
							text := decodePDFString(s[1])
							textBuilder.WriteString(text)
						}
					}
				}
			}
		}
	}

	// 方法2: 直接查找可读文本（作为补充）
	if textBuilder.Len() == 0 {
		// 尝试提取看起来像中文或英文的连续文本
		// 这是一个简单的启发式方法
		chinesePattern := regexp.MustCompile(`[\x{4e00}-\x{9fff}]+`)
		chineseMatches := chinesePattern.FindAllString(content, -1)
		for _, m := range chineseMatches {
			if len(m) > 2 { // 只保留超过2个字符的
				textBuilder.WriteString(m)
				textBuilder.WriteString(" ")
			}
		}
	}

	return textBuilder.String()
}

// decodePDFString 解码PDF字符串中的转义序列
func decodePDFString(s string) string {
	// 处理常见的PDF转义序列
	result := s
	result = strings.ReplaceAll(result, "\\n", "\n")
	result = strings.ReplaceAll(result, "\\r", "\r")
	result = strings.ReplaceAll(result, "\\t", "\t")
	result = strings.ReplaceAll(result, "\\(", "(")
	result = strings.ReplaceAll(result, "\\)", ")")
	result = strings.ReplaceAll(result, "\\\\", "\\")

	// 处理八进制转义 \ddd
	octalPattern := regexp.MustCompile(`\\([0-7]{1,3})`)
	result = octalPattern.ReplaceAllStringFunc(result, func(match string) string {
		// 跳过 \\ 开头的
		if strings.HasPrefix(match, "\\\\") {
			return match
		}
		var val int
		fmt.Sscanf(match, "\\%o", &val)
		if val > 0 && val < 256 {
			return string(rune(val))
		}
		return match
	})

	return result
}

// GetFileType 根据文件扩展名获取文件类型
func GetFileType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".pdf":
		return "pdf"
	case ".docx":
		return "docx"
	case ".doc":
		return "doc"
	case ".txt":
		return "txt"
	default:
		return "unknown"
	}
}

// ExtractParagraphsFromDOCX 从DOCX文件提取段落列表（包括表格内容）
// 用于文档比对，将表格每行作为单独的段落返回
func ExtractParagraphsFromDOCX(filePath string) ([]string, error) {
	reader, err := zip.OpenReader(filePath)
	if err != nil {
		global.Log.Error("打开DOCX文件失败", zap.Error(err))
		return nil, fmt.Errorf("打开DOCX文件失败: %w", err)
	}
	defer reader.Close()

	var documentFile *zip.File
	for _, file := range reader.File {
		if file.Name == "word/document.xml" {
			documentFile = file
			break
		}
	}

	if documentFile == nil {
		return nil, errors.New("DOCX文件格式错误：未找到document.xml")
	}

	rc, err := documentFile.Open()
	if err != nil {
		return nil, fmt.Errorf("读取document.xml失败: %w", err)
	}
	defer rc.Close()

	xmlData, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("读取XML内容失败: %w", err)
	}

	return parseDocxXMLToParagraphs(xmlData)
}

// parseDocxXMLToParagraphs 解析DOCX XML提取段落列表（包括表格）
func parseDocxXMLToParagraphs(xmlData []byte) ([]string, error) {

	// 定义XML结构
	type Text struct {
		XMLName xml.Name `xml:"t"`
		Content string   `xml:",chardata"`
	}

	type Run struct {
		XMLName xml.Name `xml:"r"`
		Texts   []Text   `xml:"t"`
	}

	type Paragraph struct {
		XMLName xml.Name `xml:"p"`
		Runs    []Run    `xml:"r"`
	}

	type TableCell struct {
		XMLName    xml.Name    `xml:"tc"`
		Paragraphs []Paragraph `xml:"p"`
	}

	type TableRow struct {
		XMLName xml.Name    `xml:"tr"`
		Cells   []TableCell `xml:"tc"`
	}

	type Table struct {
		XMLName xml.Name   `xml:"tbl"`
		Rows    []TableRow `xml:"tr"`
	}

	type Body struct {
		XMLName    xml.Name    `xml:"body"`
		Paragraphs []Paragraph `xml:"p"`
		Tables     []Table     `xml:"tbl"`
	}

	type Document struct {
		XMLName xml.Name `xml:"document"`
		Body    Body     `xml:"body"`
	}

	var doc Document
	decoder := xml.NewDecoder(bytes.NewReader(xmlData))
	decoder.Strict = false

	if err := decoder.Decode(&doc); err != nil {
		return extractParagraphsByRegex(xmlData), nil
	}

	var lines []string

	// 提取普通段落
	for _, para := range doc.Body.Paragraphs {
		var textBuilder strings.Builder
		for _, run := range para.Runs {
			for _, t := range run.Texts {
				textBuilder.WriteString(t.Content)
			}
		}
		text := strings.TrimSpace(textBuilder.String())
		if text != "" {
			lines = append(lines, text)
		}
	}

	// 提取表格内容
	for _, table := range doc.Body.Tables {
		for _, row := range table.Rows {
			var cellTexts []string
			for _, cell := range row.Cells {
				var cellBuilder strings.Builder
				for _, para := range cell.Paragraphs {
					for _, run := range para.Runs {
						for _, t := range run.Texts {
							cellBuilder.WriteString(t.Content)
						}
					}
				}
				cellText := strings.TrimSpace(cellBuilder.String())
				if cellText != "" {
					cellTexts = append(cellTexts, cellText)
				}
			}
			if len(cellTexts) > 0 {
				lines = append(lines, strings.Join(cellTexts, "\t"))
			}
		}
	}

	if len(lines) == 0 {
		return extractParagraphsByRegex(xmlData), nil
	}

	return lines, nil
}

// extractParagraphsByRegex 使用正则表达式提取段落列表
func extractParagraphsByRegex(xmlData []byte) []string {
	content := string(xmlData)
	var lines []string

	// 提取段落文本
	paraPattern := regexp.MustCompile(`<w:p[^>]*>(.*?)</w:p>`)
	paraMatches := paraPattern.FindAllStringSubmatch(content, -1)

	for _, match := range paraMatches {
		if len(match) > 1 {
			text := extractTextFromXML(match[1])
			if strings.TrimSpace(text) != "" {
				lines = append(lines, text)
			}
		}
	}

	// 提取表格内容
	tablePattern := regexp.MustCompile(`<w:tbl[^>]*>(.*?)</w:tbl>`)
	tableMatches := tablePattern.FindAllStringSubmatch(content, -1)

	for _, tableMatch := range tableMatches {
		if len(tableMatch) > 1 {
			rowPattern := regexp.MustCompile(`<w:tr[^>]*>(.*?)</w:tr>`)
			rowMatches := rowPattern.FindAllStringSubmatch(tableMatch[1], -1)

			for _, rowMatch := range rowMatches {
				if len(rowMatch) > 1 {
					cellPattern := regexp.MustCompile(`<w:tc[^>]*>(.*?)</w:tc>`)
					cellMatches := cellPattern.FindAllStringSubmatch(rowMatch[1], -1)

					var cellTexts []string
					for _, cellMatch := range cellMatches {
						if len(cellMatch) > 1 {
							cellText := extractTextFromXML(cellMatch[1])
							if strings.TrimSpace(cellText) != "" {
								cellTexts = append(cellTexts, strings.TrimSpace(cellText))
							}
						}
					}

					if len(cellTexts) > 0 {
						lines = append(lines, strings.Join(cellTexts, "\t"))
					}
				}
			}
		}
	}

	return lines
}

// extractTextFromXML 从XML片段中提取文本
func extractTextFromXML(xmlContent string) string {
	re := regexp.MustCompile(`<w:t[^>]*>([^<]*)</w:t>`)
	matches := re.FindAllStringSubmatch(xmlContent, -1)

	var textBuilder strings.Builder
	for _, match := range matches {
		if len(match) > 1 {
			textBuilder.WriteString(match[1])
		}
	}

	return textBuilder.String()
}
