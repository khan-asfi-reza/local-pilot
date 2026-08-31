package lang

import (
	"os"
	"path/filepath"
	"regexp"
)

// Compiler options that fail `tsc -b` over style, not correctness. A generated
// app's build gate must reject real type errors; an import that is not used yet,
// or a type imported without the `type` keyword, is not a reason to refuse to
// build - it just burns repair turns that should go to the actual feature.
var buildGateOnlyOptions = []string{
	"noUnusedLocals",
	"noUnusedParameters",
	"verbatimModuleSyntax",
	"erasableSyntaxOnly",
}

var tsconfigNames = []string{"tsconfig.json", "tsconfig.app.json", "tsconfig.node.json"}

// relaxTSBuildGate turns off the style-only strictness in a scaffold's tsconfig,
// leaving `strict` (and every real type check) on.
func relaxTSBuildGate(workDir string) {
	for _, name := range tsconfigNames {
		path := filepath.Join(workDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		out := string(data)
		changed := false
		for _, opt := range buildGateOnlyOptions {
			// A text edit, not a JSON round-trip: these files are JSONC (the
			// generators ship them with comments), which json.Unmarshal rejects.
			re := regexp.MustCompile(`("` + opt + `"\s*:\s*)true`)
			if re.MatchString(out) {
				out = re.ReplaceAllString(out, "${1}false")
				changed = true
			}
		}
		if changed {
			_ = os.WriteFile(path, []byte(out), 0o644)
		}
	}
}
