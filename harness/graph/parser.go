package graph

import (
	"context"
	"strings"
)

// RawRef is a name used at a call/reference site, resolved to an edge later.
type RawRef struct {
	Name string
	Line int // 1-based
}

// Parser extracts definition symbols and unresolved references from one file.
// The tree-sitter implementation lives in binding.go (build tag !nots); a no-op
// stub in binding_stub.go covers pure-Go (CGO_ENABLED=0) builds.
type Parser interface {
	Supports(lang string) bool
	Parse(ctx context.Context, lang, rel string, src []byte) (syms []*Node, refs []RawRef, err error)
}

// kindOf maps a tag-query definition suffix to a NodeKind.
func kindOf(suffix string) NodeKind {
	switch suffix {
	case "method":
		return KindMethod
	case "class":
		return KindClass
	case "type":
		return KindType
	default:
		return KindFunc
	}
}

// sigLine returns a bounded, single-line signature from a definition's source.
func sigLine(content string) string {
	line := content
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	line = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(line), "{("))
	if len(line) > 100 {
		line = line[:100]
	}
	return line
}
