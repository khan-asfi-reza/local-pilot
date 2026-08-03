package graph

import (
	"sort"
	"strings"
)

const (
	defaultDigestBytes = 6000
	maxSymbolsPerFile  = 25
)

// Digest renders a ranked, budget-bounded project map: files ordered by their
// top symbol's PageRank, each listing its highest-ranked symbols. It reads like
// the old repo map but ranked and structural, and stays within maxBytes.
func (g *Graph) Digest(maxBytes int, personalize map[NodeID]float64) string {
	if maxBytes <= 0 {
		maxBytes = defaultDigestBytes
	}
	rank := g.Rank(RankOpts{Personalize: personalize})

	type group struct {
		file string
		best float64
		syms []*Node
	}
	groups := map[string]*group{}
	for id, n := range g.Nodes {
		if n.Kind == KindFile {
			if groups[n.File] == nil {
				groups[n.File] = &group{file: n.File}
			}
			continue
		}
		fg := groups[n.File]
		if fg == nil {
			fg = &group{file: n.File}
			groups[n.File] = fg
		}
		fg.syms = append(fg.syms, n)
		if r := rank[id]; r > fg.best {
			fg.best = r
		}
	}

	order := make([]*group, 0, len(groups))
	for _, fg := range groups {
		order = append(order, fg)
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].best != order[j].best {
			return order[i].best > order[j].best
		}
		return order[i].file < order[j].file
	})

	var b strings.Builder
	for _, fg := range order {
		if b.Len()+len(fg.file)+1 > maxBytes {
			break
		}
		b.WriteString(fg.file + "\n")
		sort.Slice(fg.syms, func(i, j int) bool {
			ri, rj := rank[fg.syms[i].ID], rank[fg.syms[j].ID]
			if ri != rj {
				return ri > rj
			}
			return fg.syms[i].Line < fg.syms[j].Line
		})
		for i, s := range fg.syms {
			if i >= maxSymbolsPerFile {
				break
			}
			entry := "  " + symLabel(s) + "\n"
			if b.Len()+len(entry) > maxBytes {
				break
			}
			b.WriteString(entry)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// symLabel is the compact one-line label for a symbol in the digest.
func symLabel(n *Node) string {
	label := n.Sig
	if label == "" {
		label = string(n.Kind) + " " + n.Name
	}
	if n.Line > 0 {
		label += "  :" + itoa(n.Line)
	}
	return label
}

// QueryResult is one row returned by the query_graph tool.
type QueryResult struct {
	Name string `json:"name"`
	Kind string `json:"kind,omitempty"`
	File string `json:"file"`
	Line int    `json:"line,omitempty"`
	Sig  string `json:"sig,omitempty"`
}

// Query answers a structural question about the graph. Ops:
//
//	definition — where a symbol is defined
//	callers    — call sites of a symbol
//	references — call + reference sites of a symbol
//	imports    — files that file Y imports (its dependencies)
//	importers  — files that import file Y
//	outline    — symbols defined in file Y, ranked
func (g *Graph) Query(op, symbol, file string, max int) []QueryResult {
	if max <= 0 {
		max = 30
	}
	var out []QueryResult
	push := func(id NodeID) {
		if n := g.Nodes[id]; n != nil && len(out) < max {
			out = append(out, nodeResult(n))
		}
	}
	switch op {
	case "definition":
		for _, id := range g.Lookup(symbol) {
			push(id)
		}
	case "callers", "references":
		kinds := []EdgeKind{EdgeCalls}
		if op == "references" {
			kinds = []EdgeKind{EdgeCalls, EdgeReferences}
		}
		seen := map[NodeID]bool{}
		for _, defID := range g.Lookup(symbol) {
			for _, e := range g.inEdges(defID, kinds...) {
				if !seen[e.From] {
					seen[e.From] = true
					push(e.From)
				}
			}
		}
	case "imports":
		for _, e := range g.outEdges(fileID(file), EdgeImports) {
			push(e.To)
		}
	case "importers":
		for _, e := range g.inEdges(fileID(file), EdgeImports) {
			push(e.From)
		}
	case "outline":
		rank := g.Rank(RankOpts{})
		syms := g.symbolsInFile(file)
		sort.Slice(syms, func(i, j int) bool { return rank[syms[i].ID] > rank[syms[j].ID] })
		for i, s := range syms {
			if i >= max {
				break
			}
			out = append(out, nodeResult(s))
		}
	}
	return out
}

func nodeResult(n *Node) QueryResult {
	r := QueryResult{Name: n.Name, File: n.File, Line: n.Line, Sig: n.Sig}
	if n.Kind != KindFile {
		r.Kind = string(n.Kind)
	}
	if r.Name == "" {
		r.Name = n.File
	}
	return r
}
