package tools

import (
	"encoding/json"
	"fmt"

	"harness/harness/events"
	"harness/harness/graph"
)

// queryGraphTool answers structural questions from the persisted code graph, so
// the model pulls a fact ("who calls X", "where is Y") instead of reading whole
// files. Read-only, so it is offered in every mode including plan.
func queryGraphTool() *Tool {
	return &Tool{
		Name: "query_graph",
		Description: "Query the repository's code graph instead of reading whole files. Ops: " +
			"definition (where a symbol is defined), callers (call sites of a symbol), references (all uses), " +
			"imports (files a given file imports), importers (files that import it), outline (symbols in a file, " +
			"ranked). Pass `symbol` for definition/callers/references, or `file` for imports/importers/outline. " +
			"Cheaper than read_file for \"who calls X\" / \"where is Y\" questions.",
		Params:  json.RawMessage(`{"type":"object","properties":{"op":{"type":"string","enum":["definition","callers","references","imports","importers","outline"],"description":"The query to run."},"symbol":{"type":"string","description":"Symbol name (for definition/callers/references)."},"file":{"type":"string","description":"Repo-relative file path (for imports/importers/outline)."},"max_results":{"type":"integer","description":"Cap on results.","default":30}},"required":["op"]}`),
		WebSafe: true,
		Run: func(env Env, args Args) (any, *events.Diff, error) {
			op := args.Str("op")
			if op == "" {
				return nil, nil, fmt.Errorf("op is required")
			}
			g := graph.Load(env.WorkDir)
			if g == nil {
				return map[string]any{"error": "The code graph is not available for this project yet. Use search or read_file instead."}, nil, nil
			}
			res := g.Query(op, args.Str("symbol"), args.Str("file"), args.Int("max_results", 30))
			return map[string]any{"op": op, "count": len(res), "results": res}, nil, nil
		},
	}
}
