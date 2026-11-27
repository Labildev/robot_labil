package doc

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ledongthuc/pdf"
)

// IsSupportedExt reports whether the file extension is supported for document extraction.
func IsSupportedExt(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf", ".docx", ".txt", ".md":
		return true
	default:
		return false
	}
}

// Extractor extracts clean UTF-8 text from local PDF, DOCX, or plain-text files.
type Extractor struct {
	pdfToTextBin string
}

// NewExtractor creates a document Extractor configured with an optional pdftotext binary path.
func NewExtractor(pdfToTextBin string) *Extractor {
	if strings.TrimSpace(pdfToTextBin) == "" {
		pdfToTextBin = "pdftotext"
	}
	return &Extractor{pdfToTextBin: pdfToTextBin}
}

// Extract reads path and returns normalized extracted text.
func (e *Extractor) Extract(ctx context.Context, path string) (string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	var raw string
	var err error

	switch ext {
	case ".docx":
		raw, err = extractDOCX(path)
	case ".pdf":
		raw, err = e.extractPDF(ctx, path)
	case ".txt", ".md":
		var data []byte
		data, err = os.ReadFile(path)
		raw = string(data)
	default:
		return "", fmt.Errorf("unsupported document extension %q", ext)
	}
	if err != nil {
		return "", err
	}

	cleaned := NormalizeText(raw)
	if cleaned == "" {
		return "", fmt.Errorf("no extractable text found in %q", path)
	}
	return cleaned, nil
}

func (e *Extractor) extractPDF(ctx context.Context, path string) (string, error) {
	if binPath, err := exec.LookPath(e.pdfToTextBin); err == nil {
		cmd := exec.CommandContext(ctx, binPath, "-enc", "UTF-8", path, "-")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if runErr := cmd.Run(); runErr == nil && strings.TrimSpace(stdout.String()) != "" {
			return stdout.String(), nil
		}
	}
	return extractPDFPureGo(path)
}

func extractPDFPureGo(path string) (string, error) {
	f, reader, err := pdf.Open(path)
	if err != nil {
		return "", fmt.Errorf("open pdf %q: %w", path, err)
	}
	defer f.Close()

	plain, err := reader.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("extract pdf text %q: %w", path, err)
	}
	data, err := io.ReadAll(plain)
	if err != nil {
		return "", fmt.Errorf("read pdf stream %q: %w", path, err)
	}
	return string(data), nil
}

func extractDOCX(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("open docx %q: %w", path, err)
	}
	defer zr.Close()

	for _, file := range zr.File {
		if file.Name != "word/document.xml" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return "", fmt.Errorf("open word/document.xml in %q: %w", path, err)
		}
		defer rc.Close()

		return parseWordDocumentXML(rc)
	}
	return "", fmt.Errorf("word/document.xml not found in %q", path)
}

func parseWordDocumentXML(r io.Reader) (string, error) {
	decoder := xml.NewDecoder(r)
	var out strings.Builder
	var paraBuf strings.Builder
	inText := false

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("decode docx xml: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				paraBuf.Reset()
			case "t":
				inText = true
			case "tab":
				paraBuf.WriteByte('\t')
			case "br":
				paraBuf.WriteByte('\n')
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "p":
				line := strings.TrimSpace(paraBuf.String())
				if line != "" {
					if out.Len() > 0 {
						out.WriteString("\n\n")
					}
					out.WriteString(line)
				}
			}
		case xml.CharData:
			if inText {
				paraBuf.Write(t)
			}
		}
	}
	return out.String(), nil
}

// NormalizeText normalizes line endings and collapses redundant blank lines.
func NormalizeText(input string) string {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	input = strings.ReplaceAll(input, "\r", "\n")
	lines := strings.Split(input, "\n")
	var cleaned []string
	blankRun := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			blankRun++
			if blankRun <= 1 && len(cleaned) > 0 {
				cleaned = append(cleaned, "")
			}
			continue
		}
		blankRun = 0
		cleaned = append(cleaned, trimmed)
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}
