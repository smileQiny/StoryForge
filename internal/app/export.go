package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"storyforge/internal/model"
	"storyforge/internal/store"
)

// ExportService handles book export use cases.
type ExportService struct {
	dataDir  string
	books    *store.BookStore
	chapters *store.ChapterStore
}

// ExportResult is the rendered book export payload.
type ExportResult struct {
	Filename    string
	ContentType string
	Content     []byte
}

// NewExportService creates an ExportService.
func NewExportService(dataDir string, books *store.BookStore, chapters *store.ChapterStore) *ExportService {
	return &ExportService{dataDir: dataDir, books: books, chapters: chapters}
}

// ExportBook renders the book into txt, markdown, or HTML.
func (s *ExportService) ExportBook(bookID, format string, approvedOnly bool) (*ExportResult, error) {
	if _, err := s.books.Get(bookID); err != nil {
		return nil, err
	}

	metas, err := s.chapters.ListMeta(bookID)
	if err != nil {
		return nil, err
	}
	if len(metas) == 0 {
		metas = []*model.ChapterMeta{}
	}
	sort.Slice(metas, func(i, j int) bool {
		return metas[i].Number < metas[j].Number
	})
	if approvedOnly {
		filtered := make([]*model.ChapterMeta, 0, len(metas))
		for _, meta := range metas {
			if meta.Status == model.ChapterStatusApproved {
				filtered = append(filtered, meta)
			}
		}
		metas = filtered
	}

	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "txt":
		return s.renderText(bookID, metas)
	case "md", "markdown":
		return s.renderMarkdown(bookID, metas)
	case "html", "epub":
		return s.renderHTML(bookID, metas)
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
}

// SaveBook renders a book and persists the export into the book directory.
func (s *ExportService) SaveBook(bookID, format string, approvedOnly bool) (string, int, error) {
	result, err := s.ExportBook(bookID, format, approvedOnly)
	if err != nil {
		return "", 0, err
	}
	path := filepath.Join(s.dataDir, bookID, result.Filename)
	if err := os.WriteFile(path, result.Content, 0o644); err != nil {
		return "", 0, err
	}
	metas, err := s.chapters.ListMeta(bookID)
	if err != nil {
		return "", 0, err
	}
	count := 0
	for _, meta := range metas {
		if approvedOnly && meta.Status != model.ChapterStatusApproved {
			continue
		}
		count++
	}
	return path, count, nil
}

func (s *ExportService) renderText(bookID string, metas []*model.ChapterMeta) (*ExportResult, error) {
	var buf bytes.Buffer
	for i, meta := range metas {
		if i > 0 {
			buf.WriteString("\n\n")
		}
		body, err := s.chapters.GetContent(bookID, meta.Number)
		if err != nil {
			return nil, err
		}
		if meta.Title != "" {
			fmt.Fprintf(&buf, "Chapter %d: %s\n\n", meta.Number, meta.Title)
		} else {
			fmt.Fprintf(&buf, "Chapter %d\n\n", meta.Number)
		}
		buf.WriteString(strings.TrimRight(body, "\n"))
	}
	filename := fmt.Sprintf("%s.txt", bookID)
	return &ExportResult{
		Filename:    filename,
		ContentType: "text/plain; charset=utf-8",
		Content:     buf.Bytes(),
	}, nil
}

func (s *ExportService) renderMarkdown(bookID string, metas []*model.ChapterMeta) (*ExportResult, error) {
	var buf bytes.Buffer
	for i, meta := range metas {
		if i > 0 {
			buf.WriteString("\n\n")
		}
		body, err := s.chapters.GetContent(bookID, meta.Number)
		if err != nil {
			return nil, err
		}
		if meta.Title != "" {
			fmt.Fprintf(&buf, "# Chapter %d: %s\n\n", meta.Number, meta.Title)
		} else {
			fmt.Fprintf(&buf, "# Chapter %d\n\n", meta.Number)
		}
		buf.WriteString(strings.TrimRight(body, "\n"))
	}
	filename := fmt.Sprintf("%s.md", bookID)
	return &ExportResult{
		Filename:    filename,
		ContentType: "text/markdown; charset=utf-8",
		Content:     buf.Bytes(),
	}, nil
}

func (s *ExportService) renderHTML(bookID string, metas []*model.ChapterMeta) (*ExportResult, error) {
	var toc bytes.Buffer
	var body bytes.Buffer
	for i, meta := range metas {
		content, err := s.chapters.GetContent(bookID, meta.Number)
		if err != nil {
			return nil, err
		}
		title := fmt.Sprintf("Chapter %d", meta.Number)
		if meta.Title != "" {
			title = fmt.Sprintf("Chapter %d: %s", meta.Number, meta.Title)
		}
		fmt.Fprintf(&toc, `<li><a href="#ch%d">%s</a></li>`, meta.Number, htmlEscape(title))
		if i < len(metas)-1 {
			toc.WriteString("\n")
		}

		fmt.Fprintf(&body, `<section><h2 id="ch%d">%s</h2>`, meta.Number, htmlEscape(title))
		for _, para := range splitExportParagraphs(content) {
			fmt.Fprintf(&body, "<p>%s</p>", htmlEscape(para))
		}
		body.WriteString("</section>")
		if i < len(metas)-1 {
			body.WriteString("\n<hr/>\n")
		}
	}

	document := fmt.Sprintf(
		"<!DOCTYPE html><html><head><meta charset=\"utf-8\"><title>%s</title><style>%s</style></head><body><h1>%s</h1><nav><ol>%s</ol></nav><hr/>%s</body></html>",
		htmlEscape(bookID),
		`body{font-family:serif;max-width:42em;margin:auto;padding:2em;line-height:1.8}nav ol{padding-left:1.4em}h2{margin-top:2.5em}p{text-indent:2em}`,
		htmlEscape(bookID),
		toc.String(),
		body.String(),
	)
	return &ExportResult{
		Filename:    fmt.Sprintf("%s.html", bookID),
		ContentType: "text/html; charset=utf-8",
		Content:     []byte(document),
	}, nil
}

func splitExportParagraphs(content string) []string {
	parts := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n\n")
	paragraphs := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			paragraphs = append(paragraphs, trimmed)
		}
	}
	if len(paragraphs) == 0 && strings.TrimSpace(content) != "" {
		return []string{strings.TrimSpace(content)}
	}
	return paragraphs
}

func htmlEscape(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(s)
}
