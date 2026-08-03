package orchestrator

import (
	"regexp"
	"strings"
)

const maxSections = 40

var headingRe = regexp.MustCompile(`^\s{0,3}#{1,6}\s+(.+?)\s*#*\s*$`)

// numHeadingRe catches numbered section headings in non-markdown specs (e.g. a
// .docx exported to text): "1. Title", "3.1 Title", "5.2 Title (…)". Only applied
// to SHORT lines, so a descriptive numbered list item ("1. Media Assets: Minimum
// 3 images …") is not mistaken for a section heading.
var numHeadingRe = regexp.MustCompile(`^\s{0,3}\d+(?:\.\d+)*\.?\s+\S`)

// headingTitle returns the section title for a heading line (markdown or numbered),
// and whether the line is a heading at all.
func headingTitle(ln string) (string, bool) {
	if m := headingRe.FindStringSubmatch(ln); m != nil {
		return strings.TrimSpace(m[1]), true
	}
	if t := strings.TrimSpace(ln); len(t) <= 70 && numHeadingRe.MatchString(ln) {
		return t, true // keep the leading number in the title — useful context
	}
	return "", false
}

// SplitSections slices a PRD into sections on Markdown headings; content before
// the first heading is an untitled intro. This is what guarantees no single call
// ever sees the whole PRD.
func SplitSections(text string) []Section {
	lines := strings.Split(text, "\n")
	var sections []Section
	title := ""
	var cur []string

	flush := func() {
		body := strings.TrimSpace(strings.Join(cur, "\n"))
		if title == "" && body == "" {
			cur = nil
			return
		}
		sections = append(sections, Section{Title: title, Body: body})
		cur = nil
	}

	for _, ln := range lines {
		if ht, ok := headingTitle(ln); ok {
			flush()
			title = ht
			continue
		}
		cur = append(cur, ln)
	}
	flush()

	if len(sections) == 0 {
		return []Section{{Index: 0, Title: "", Body: strings.TrimSpace(text)}}
	}
	if len(sections) > maxSections {
		sections = sections[:maxSections]
	}
	for i := range sections {
		sections[i].Index = i
	}
	return sections
}

// Outline renders section titles plus a one-line preview for the decompose call.
func Outline(sections []Section) string {
	var b strings.Builder
	for _, s := range sections {
		title := s.Title
		if title == "" {
			title = "(intro)"
		}
		b.WriteString(itoa(s.Index))
		b.WriteString(". ")
		b.WriteString(title)
		if preview := firstLine(s.Body); preview != "" {
			b.WriteString(" — ")
			b.WriteString(preview)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// SectionBody returns the body for an index, or "" when out of range.
func SectionBody(sections []Section, idx int) string {
	if idx < 0 || idx >= len(sections) {
		return ""
	}
	return sections[idx].Body
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
