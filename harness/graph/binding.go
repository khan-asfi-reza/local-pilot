//go:build cgo

package graph

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/ruby"
	"github.com/smacker/go-tree-sitter/rust"
	tsx "github.com/smacker/go-tree-sitter/typescript/tsx"
	typescript "github.com/smacker/go-tree-sitter/typescript/typescript"
)

// Enabled reports whether this build has real tree-sitter parsing. It is true
// only in a cgo build (CGO_ENABLED=1 with a C compiler); otherwise the stub in
// binding_stub.go is compiled and callers fall back to the regex repo map.
func Enabled() bool { return true }

// grammarByLang binds each language name to its tree-sitter grammar.
func grammarByLang() map[string]*sitter.Language {
	return map[string]*sitter.Language{
		"go":         golang.GetLanguage(),
		"python":     python.GetLanguage(),
		"javascript": javascript.GetLanguage(),
		"typescript": typescript.GetLanguage(),
		"tsx":        tsx.GetLanguage(),
		"rust":       rust.GetLanguage(),
		"java":       java.GetLanguage(),
		"ruby":       ruby.GetLanguage(),
		"c":          c.GetLanguage(),
		"cpp":        cpp.GetLanguage(),
	}
}

// tsParser is the tree-sitter-backed Parser. Queries are compiled once per
// language; a language whose query fails to compile is skipped, never fatal.
type tsParser struct {
	langs   map[string]*sitter.Language
	queries map[string]*sitter.Query
}

func newParser() Parser {
	p := &tsParser{langs: map[string]*sitter.Language{}, queries: map[string]*sitter.Query{}}
	for name, lang := range grammarByLang() {
		scm := queryFor(name)
		if scm == "" {
			continue
		}
		q, err := sitter.NewQuery([]byte(scm), lang)
		if err != nil {
			continue // one bad query must not disable the whole graph
		}
		p.langs[name] = lang
		p.queries[name] = q
	}
	return p
}

func (p *tsParser) Supports(lang string) bool { return p.queries[lang] != nil }

func (p *tsParser) Parse(ctx context.Context, lang, rel string, src []byte) ([]*Node, []RawRef, error) {
	q, l := p.queries[lang], p.langs[lang]
	if q == nil || l == nil {
		return nil, nil, nil
	}
	parser := sitter.NewParser()
	defer parser.Close()
	parser.SetLanguage(l)
	tree, err := parser.ParseCtx(ctx, nil, src)
	if err != nil {
		return nil, nil, err
	}
	defer tree.Close()

	qc := sitter.NewQueryCursor()
	defer qc.Close()
	qc.Exec(q, tree.RootNode())

	var syms []*Node
	var refs []RawRef
	for {
		m, ok := qc.NextMatch()
		if !ok {
			break
		}
		var name string
		var kind NodeKind
		var defNode *sitter.Node
		var refName string
		var refLine int
		for _, cap := range m.Captures {
			if cap.Node == nil {
				continue
			}
			cn := q.CaptureNameForId(cap.Index)
			switch {
			case strings.HasPrefix(cn, "name.definition."):
				name = cap.Node.Content(src)
				kind = kindOf(strings.TrimPrefix(cn, "name.definition."))
			case strings.HasPrefix(cn, "definition."):
				defNode = cap.Node
			case cn == "name.reference.call" || cn == "reference.call":
				refName = cap.Node.Content(src)
				refLine = int(cap.Node.StartPoint().Row) + 1
			}
		}
		if refName != "" {
			refs = append(refs, RawRef{Name: refName, Line: refLine})
			continue
		}
		if name == "" || defNode == nil {
			continue
		}
		syms = append(syms, &Node{
			Kind: kind, Name: name, File: rel,
			Line: int(defNode.StartPoint().Row) + 1,
			End:  int(defNode.EndPoint().Row) + 1,
			Sig:  sigLine(defNode.Content(src)),
		})
	}
	return syms, refs, nil
}
