package doc

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createTestDOCX(t *testing.T, path string, documentXML string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create docx: %v", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatalf("create entry: %v", err)
	}
	if _, err := w.Write([]byte(documentXML)); err != nil {
		t.Fatalf("write xml: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
}

func TestExtractDOCXAndUnsupported(t *testing.T) {
	tmp := t.TempDir()
	docxPath := filepath.Join(tmp, "sample.docx")
	xmlBody := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>Bab 1 Pendahuluan</w:t></w:r></w:p>
    <w:p><w:r><w:t>Ini isi dokumen lokal pertama.</w:t></w:r></w:p>
  </w:body>
</w:document>`
	createTestDOCX(t, docxPath, xmlBody)

	ext := NewExtractor("")
	got, err := ext.Extract(context.Background(), docxPath)
	if err != nil {
		t.Fatalf("Extract docx failed: %v", err)
	}
	expected := "Bab 1 Pendahuluan\n\nIni isi dokumen lokal pertama."
	if got != expected {
		t.Fatalf("unexpected docx output:\ngot:  %q\nwant: %q", got, expected)
	}

	if _, err := ext.Extract(context.Background(), filepath.Join(tmp, "image.png")); err == nil {
		t.Fatalf("expected error for unsupported extension")
	}
}

func TestChunkByWords(t *testing.T) {
	text := strings.Join([]string{
		"satu dua tiga empat",
		"lima enam tujuh delapan",
		"sembilan sepuluh sebelas",
	}, "\n\n")

	chunks := ChunkByWords(text, 5)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d (%v)", len(chunks), chunks)
	}

	longPara := "a b c d e f g"
	longChunks := ChunkByWords(longPara, 3)
	if len(longChunks) != 3 {
		t.Fatalf("expected 3 split chunks for oversized paragraph, got %d", len(longChunks))
	}
}
