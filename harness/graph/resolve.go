package graph

import "sort"

// rebuildEdges recomputes every edge from the per-file metadata (symbols, refs,
// imports) already in the graph. It runs after any parse so a single changed file
// correctly updates cross-file edges, without re-reading unchanged files.
func (g *Graph) rebuildEdges() {
	g.Edges = g.Edges[:0]

	// Name -> definition ids, and a per-file symbol list for enclosure lookup.
	byName := map[string][]NodeID{}
	for id, n := range g.Nodes {
		if n.Kind != KindFile && n.Name != "" {
			byName[n.Name] = append(byName[n.Name], id)
		}
	}
	known := map[string]bool{}
	for rel := range g.Files {
		known[rel] = true
	}

	for rel, fm := range g.Files {
		fid := fileID(rel)
		syms := g.symbolsInFile(rel)

		// file contains its symbols
		for _, s := range syms {
			g.Edges = append(g.Edges, Edge{From: fid, To: s.ID, Kind: EdgeContains})
		}

		// references: from the enclosing symbol (or the file) to each resolved target
		for _, ref := range fm.Refs {
			from := enclosing(syms, ref.Line)
			fromID := fid
			if from != nil {
				fromID = from.ID
			}
			target := pickTarget(byName[ref.Name], rel, fromID)
			if target == "" {
				continue
			}
			g.Edges = append(g.Edges, Edge{From: fromID, To: target, Kind: EdgeCalls})
		}

		// imports: file -> file
		for _, spec := range fm.Imps {
			if to := resolveSpec(fm.Lang, rel, spec, known); to != "" && to != rel {
				g.Edges = append(g.Edges, Edge{From: fid, To: fileID(to), Kind: EdgeImports})
			}
		}
	}
	g.index()
}

// enclosing returns the innermost symbol whose line range contains line.
func enclosing(syms []*Node, line int) *Node {
	var best *Node
	for _, s := range syms {
		if line >= s.Line && (s.End == 0 || line <= s.End) {
			if best == nil || s.Line >= best.Line {
				best = s // deeper/nearer definition wins
			}
		}
	}
	return best
}

// pickTarget chooses one definition for a referenced name: prefer a definition in
// the same file, else the lexically-first, and never the referrer itself.
func pickTarget(cands []NodeID, sameFile string, self NodeID) NodeID {
	if len(cands) == 0 {
		return ""
	}
	sorted := append([]NodeID(nil), cands...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	var first NodeID
	for _, c := range sorted {
		if c == self {
			continue
		}
		if first == "" {
			first = c
		}
	}
	// prefer same-file
	for _, c := range sorted {
		if c == self {
			continue
		}
		if n := nodeFileOf(c); n == sameFile {
			return c
		}
	}
	return first
}

// nodeFileOf extracts the file component from a symbol id ("s:<file>#name@line").
func nodeFileOf(id NodeID) string {
	s := string(id)
	if len(s) < 2 || s[:2] != "s:" {
		return ""
	}
	s = s[2:]
	for i := 0; i < len(s); i++ {
		if s[i] == '#' {
			return s[:i]
		}
	}
	return s
}
