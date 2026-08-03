// Package graph builds a persistent, tree-sitter-derived code graph of a
// repository — files and symbols as nodes, defines/reference/call/import edges —
// ranked by PageRank so the most important symbols surface first. It replaces the
// regex repo map with a structural one and backs the query_graph tool, so the
// model pulls facts instead of re-reading whole files. It is a leaf package
// (stdlib + tree-sitter only), so agent and tools can both use it.
package graph

import (
	"path/filepath"
	"sort"
)

// NodeKind classifies a node.
type NodeKind string

const (
	KindFile   NodeKind = "file"
	KindFunc   NodeKind = "func"
	KindMethod NodeKind = "method"
	KindClass  NodeKind = "class"
	KindType   NodeKind = "type" // struct/interface/enum/trait/type alias
)

// EdgeKind classifies a directed edge.
type EdgeKind string

const (
	EdgeContains   EdgeKind = "contains"   // file -> symbol
	EdgeReferences EdgeKind = "references" // symbol -> symbol (name used)
	EdgeCalls      EdgeKind = "calls"      // symbol -> symbol (call site)
	EdgeImports    EdgeKind = "imports"    // file -> file
)

// NodeID uniquely identifies a node: "f:<rel>" for a file, "s:<rel>#<name>@<line>"
// for a symbol.
type NodeID string

// Node is a file or a symbol.
type Node struct {
	ID   NodeID   `json:"id"`
	Kind NodeKind `json:"kind"`
	Name string   `json:"name,omitempty"`
	File string   `json:"file"`
	Line int      `json:"line,omitempty"` // 1-based
	End  int      `json:"end,omitempty"`  // 1-based, last line of the definition
	Sig  string   `json:"sig,omitempty"`  // bounded one-line signature
}

// Edge is a directed relationship.
type Edge struct {
	From NodeID   `json:"from"`
	To   NodeID   `json:"to"`
	Kind EdgeKind `json:"kind"`
}

// FileMeta caches a parsed file so unchanged files are not re-parsed. Refs and
// Imps are kept so edges can be rebuilt across the whole graph without re-reading
// any file when one file changes.
type FileMeta struct {
	Path string   `json:"path"`
	Lang string   `json:"lang"`
	Hash string   `json:"hash"`
	Syms []NodeID `json:"syms"` // symbol ids defined in this file (for incremental delete)
	Refs []RawRef `json:"refs,omitempty"`
	Imps []string `json:"imps,omitempty"` // raw import specifiers
}

// Graph is the whole code graph plus derived indexes (rebuilt on load, not
// persisted).
type Graph struct {
	Root   string              `json:"root"`
	Nodes  map[NodeID]*Node    `json:"nodes"`
	Edges  []Edge              `json:"edges"`
	Files  map[string]FileMeta `json:"files"`
	out    map[NodeID][]Edge   `json:"-"`
	in     map[NodeID][]Edge   `json:"-"`
	byName map[string][]NodeID `json:"-"` // symbol name -> definition node ids
}

func newGraph(root string) *Graph {
	return &Graph{
		Root:  root,
		Nodes: map[NodeID]*Node{},
		Files: map[string]FileMeta{},
	}
}

// index (re)builds the adjacency and name lookups from Nodes+Edges.
func (g *Graph) index() {
	g.out = map[NodeID][]Edge{}
	g.in = map[NodeID][]Edge{}
	g.byName = map[string][]NodeID{}
	for _, e := range g.Edges {
		g.out[e.From] = append(g.out[e.From], e)
		g.in[e.To] = append(g.in[e.To], e)
	}
	for id, n := range g.Nodes {
		if n.Kind != KindFile && n.Name != "" {
			g.byName[n.Name] = append(g.byName[n.Name], id)
		}
	}
}

func fileID(rel string) NodeID { return NodeID("f:" + rel) }

// Lookup returns the definition node ids for a symbol name.
func (g *Graph) Lookup(name string) []NodeID { return g.byName[name] }

// PersonalizeFiles returns a PageRank personalization vector that weights every
// node in the given files, so the ranking foregrounds the code the current task
// touches (grounding targets + recently changed files).
func (g *Graph) PersonalizeFiles(files []string) map[NodeID]float64 {
	if len(files) == 0 {
		return nil
	}
	want := map[string]bool{}
	for _, f := range files {
		want[filepath.ToSlash(f)] = true
	}
	m := map[NodeID]float64{}
	for id, n := range g.Nodes {
		if want[n.File] {
			m[id] = 1.0
		}
	}
	return m
}

// DefinesInFile reports whether a symbol with the given name is defined in file.
func (g *Graph) DefinesInFile(file, name string) bool {
	for _, id := range g.byName[name] {
		if n := g.Nodes[id]; n != nil && n.File == file {
			return true
		}
	}
	return false
}

// inEdges returns edges pointing at id of the given kinds (any kind if none).
func (g *Graph) inEdges(id NodeID, kinds ...EdgeKind) []Edge {
	return filterEdges(g.in[id], kinds)
}

// outEdges returns edges leaving id of the given kinds (any kind if none).
func (g *Graph) outEdges(id NodeID, kinds ...EdgeKind) []Edge {
	return filterEdges(g.out[id], kinds)
}

func filterEdges(edges []Edge, kinds []EdgeKind) []Edge {
	if len(kinds) == 0 {
		return edges
	}
	var out []Edge
	for _, e := range edges {
		for _, k := range kinds {
			if e.Kind == k {
				out = append(out, e)
				break
			}
		}
	}
	return out
}

// symbolsInFile returns the symbol nodes defined in a file, ordered by line.
func (g *Graph) symbolsInFile(rel string) []*Node {
	var out []*Node
	for _, id := range g.Files[rel].Syms {
		if n := g.Nodes[id]; n != nil {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Line < out[j].Line })
	return out
}
