package tools

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"harness/harness/events"
)

const maxDocBytes = 400 * 1024

var xmlTagRe = regexp.MustCompile(`<[^>]+>`)

// readDocumentTool extracts the text of office/pdf documents so the model can
// read a PRD or spec that read_file would return as binary garbage.
func readDocumentTool() *Tool {
	return &Tool{
		Name:        "read_document",
		Description: "Extract and read the plain text of a document file (.docx, .pdf, .pptx, .xlsx). Use this INSTEAD of read_file for those formats — read_file returns unreadable binary for them. Returns the document's text.",
		Params:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Document file to read (.docx/.pdf/.pptx/.xlsx)."}},"required":["path"]}`),
		Run: func(env Env, args Args) (any, *events.Diff, error) {
			p, err := safePath(env.WorkDir, args.Str("path"))
			if err != nil {
				return nil, nil, err
			}
			text, err := extractDocument(p)
			if err != nil {
				return nil, nil, err
			}
			markSeen(env, p)
			truncated := false
			if len(text) > maxDocBytes {
				text = text[:maxDocBytes]
				truncated = true
			}
			return map[string]any{"path": args.Str("path"), "text": text, "chars": len(text), "truncated": truncated}, nil, nil
		},
	}
}

// ExtractDocument returns the plain text of a document file, for callers outside
// tool dispatch (e.g. a docx/pdf passed as a task file).
func ExtractDocument(path string) (string, error) { return extractDocument(path) }

// IsDocument reports whether a path is an extractable document format.
func IsDocument(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".docx", ".pdf", ".pptx", ".xlsx":
		return true
	}
	return false
}

// extractDocument returns the plain text of a document by extension.
func extractDocument(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".docx":
		return extractOOXMLPara(path, "word/document.xml", "</w:p>", "</w:tr>")
	case ".pptx":
		return extractPptx(path)
	case ".xlsx":
		return extractXlsx(path)
	case ".pdf":
		return extractPdf(path)
	default:
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return string(raw), nil
	}
}

func stripXML(s string) string {
	return html.UnescapeString(xmlTagRe.ReplaceAllString(s, ""))
}

func zipEntry(z *zip.ReadCloser, name string) (string, error) {
	for _, f := range z.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			defer rc.Close()
			b, err := io.ReadAll(rc)
			return string(b), err
		}
	}
	return "", fmt.Errorf("%s not found in archive", name)
}

// extractOOXMLPara reads one OOXML part and turns the given block-closing tags
// into newlines before stripping markup (docx paragraphs/table rows).
func extractOOXMLPara(path, part string, breaks ...string) (string, error) {
	z, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer z.Close()
	raw, err := zipEntry(z, part)
	if err != nil {
		return "", err
	}
	raw = strings.ReplaceAll(raw, "<w:tab/>", "\t")
	for _, b := range breaks {
		raw = strings.ReplaceAll(raw, b, "\n")
	}
	return strings.TrimSpace(stripXML(raw)), nil
}

func extractPptx(path string) (string, error) {
	z, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer z.Close()
	var names []string
	for _, f := range z.File {
		if strings.HasPrefix(f.Name, "ppt/slides/slide") && strings.HasSuffix(f.Name, ".xml") {
			names = append(names, f.Name)
		}
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		raw, err := zipEntry(z, n)
		if err != nil {
			continue
		}
		raw = strings.ReplaceAll(raw, "</a:p>", "\n")
		b.WriteString(strings.TrimSpace(stripXML(raw)))
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String()), nil
}

func extractXlsx(path string) (string, error) {
	z, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer z.Close()
	raw, err := zipEntry(z, "xl/sharedStrings.xml")
	if err != nil {
		return "", nil
	}
	raw = strings.ReplaceAll(raw, "</si>", "\n")
	return strings.TrimSpace(stripXML(raw)), nil
}

func extractPdf(path string) (string, error) {
	if _, err := exec.LookPath("pdftotext"); err == nil {
		if out, err := exec.Command("pdftotext", "-layout", path, "-").Output(); err == nil {
			return strings.TrimSpace(string(out)), nil
		}
	}
	return "", fmt.Errorf("cannot extract PDF text: install poppler (brew install poppler) for the pdftotext command")
}
