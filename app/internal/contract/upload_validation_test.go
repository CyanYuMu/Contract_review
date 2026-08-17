package contract

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestValidateDOCXReader(t *testing.T) {
	valid := makeTestZIP(t, map[string]string{
		"[Content_Types].xml": "<Types/>",
		"word/document.xml":   "<document/>",
	})
	if err := validateDOCXReader(bytes.NewReader(valid), int64(len(valid))); err != nil {
		t.Fatalf("valid DOCX rejected: %v", err)
	}

	missingDocument := makeTestZIP(t, map[string]string{
		"[Content_Types].xml": "<Types/>",
	})
	if err := validateDOCXReader(bytes.NewReader(missingDocument), int64(len(missingDocument))); err == nil {
		t.Fatal("ZIP without word/document.xml was accepted")
	}

	traversal := makeTestZIP(t, map[string]string{
		"[Content_Types].xml": "<Types/>",
		"word/document.xml":   "<document/>",
		"../outside.txt":      "unsafe",
	})
	if err := validateDOCXReader(bytes.NewReader(traversal), int64(len(traversal))); err == nil {
		t.Fatal("DOCX containing a traversal entry was accepted")
	}
}

func TestSanitizeFilename(t *testing.T) {
	if got := sanitizeFilename("../../合同\r\n.pdf"); got != "合同.pdf" {
		t.Fatalf("sanitizeFilename returned %q", got)
	}
	if got := sanitizeFilename(".."); got != "contract" {
		t.Fatalf("sanitizeFilename fallback returned %q", got)
	}
}

func makeTestZIP(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
