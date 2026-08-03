package graph

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

var graphLocks sync.Map // path -> *sync.Mutex

func lockFor(path string) *sync.Mutex {
	m, _ := graphLocks.LoadOrStore(path, &sync.Mutex{})
	return m.(*sync.Mutex)
}

// gitRoot walks up from workDir to the directory containing .git, or returns
// workDir when there is none — matching where .pilot/ lives for memory/state.
func gitRoot(workDir string) string {
	dir := workDir
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return workDir
		}
		dir = parent
	}
}

func graphPath(workDir string) string {
	return filepath.Join(gitRoot(workDir), ".pilot", "graph.json")
}

// Load reads and indexes the persisted graph, or returns nil if none exists.
func Load(workDir string) *Graph {
	raw, err := os.ReadFile(graphPath(workDir))
	if err != nil {
		return nil
	}
	var g Graph
	if json.Unmarshal(raw, &g) != nil {
		return nil
	}
	if g.Nodes == nil {
		g.Nodes = map[NodeID]*Node{}
	}
	if g.Files == nil {
		g.Files = map[string]FileMeta{}
	}
	g.index()
	return &g
}

// Save atomically writes the graph (tmp + rename), serialized per path.
func Save(workDir string, g *Graph) error {
	path := graphPath(workDir)
	mu := lockFor(path)
	mu.Lock()
	defer mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(g)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func itoa(i int) string { return strconv.Itoa(i) }
