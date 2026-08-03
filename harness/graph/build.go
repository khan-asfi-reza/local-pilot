package graph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ignoredDirs mirrors agent/repomap.go so the graph and the repo map skip the
// same trees.
var ignoredDirs = map[string]bool{
	".git": true, "node_modules": true, ".venv": true, "venv": true,
	"__pycache__": true, "dist": true, "build": true, "target": true,
	".harness": true, ".pilot": true, ".next": true,
}

const (
	maxGraphFiles = 4000
	maxParseBytes = 1 << 20 // 1 MB — skip generated/minified megafiles
)

// Build produces the code graph for workDir, reusing cached parses from prev for
// files whose content hash is unchanged (so only changed/new files are parsed).
// Pass prev=nil for a full build. It never calls the model.
func Build(ctx context.Context, workDir string, prev *Graph) (*Graph, error) {
	p := newParser()
	g := newGraph(workDir)
	count := 0

	err := filepath.WalkDir(workDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if path != workDir && (ignoredDirs[name] || strings.HasPrefix(name, ".")) {
				return fs.SkipDir
			}
			return nil
		}
		if count >= maxGraphFiles {
			return fs.SkipAll
		}
		lang := langForPath(path)
		if lang == "" || !p.Supports(lang) {
			return nil
		}
		rel, e := filepath.Rel(workDir, path)
		if e != nil {
			return nil
		}
		info, e := d.Info()
		if e != nil || info.Size() > maxParseBytes {
			return nil
		}
		src, e := os.ReadFile(path)
		if e != nil {
			return nil
		}
		h := hashBytes(src)
		count++

		// Reuse an unchanged file's cached parse.
		if prev != nil {
			if fm, ok := prev.Files[rel]; ok && fm.Hash == h {
				g.Files[rel] = fm
				g.Nodes[fileID(rel)] = &Node{ID: fileID(rel), Kind: KindFile, File: rel}
				for _, id := range fm.Syms {
					if n := prev.Nodes[id]; n != nil {
						g.Nodes[id] = n
					}
				}
				return nil
			}
		}

		syms, refs, perr := p.Parse(ctx, lang, rel, src)
		if perr != nil {
			return nil
		}
		fm := FileMeta{Path: rel, Lang: lang, Hash: h, Refs: refs, Imps: importSpecs(lang, src)}
		g.Nodes[fileID(rel)] = &Node{ID: fileID(rel), Kind: KindFile, File: rel}
		for _, s := range syms {
			s.ID = symID(rel, s.Name, s.Line)
			g.Nodes[s.ID] = s
			fm.Syms = append(fm.Syms, s.ID)
		}
		g.Files[rel] = fm
		return nil
	})
	if err != nil {
		return nil, err
	}

	g.rebuildEdges()
	return g, nil
}

func symID(rel, name string, line int) NodeID {
	return NodeID("s:" + rel + "#" + name + "@" + itoa(line))
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}
