package utils

import (
	"reflect"
	"testing"
)

func TestParseDocxXMLToParagraphsPreservesTableOrder(t *testing.T) {
	xmlData := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>第一段</w:t></w:r></w:p>
    <w:tbl>
      <w:tr>
        <w:tc><w:p><w:r><w:t>表格A</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>表格B</w:t></w:r></w:p></w:tc>
      </w:tr>
    </w:tbl>
    <w:p><w:r><w:t>第二段</w:t></w:r></w:p>
  </w:body>
</w:document>`)

	lines, err := parseDocxXMLToParagraphs(xmlData)
	if err != nil {
		t.Fatalf("parseDocxXMLToParagraphs failed: %v", err)
	}

	expected := []string{"第一段", "表格A\t表格B", "第二段"}
	if !reflect.DeepEqual(lines, expected) {
		t.Fatalf("expected %#v, got %#v", expected, lines)
	}
}
