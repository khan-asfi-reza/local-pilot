package lang

import (
	"context"
	"time"
)

// javascript scaffolds Node/TypeScript stacks with the real create-* generators
// where they exist (Next.js, Vite+React) and templates where a generator would be
// deprecated or heavier than the skeleton (Express).
type javascript struct{}

func (javascript) Lang() string        { return "javascript" }
func (javascript) Toolchain() []string { return []string{"node", "npm"} }

func (javascript) Frameworks() []Framework {
	return []Framework{
		{ID: "nextjs", Priority: 60, Keywords: kw(`next\.?js`), Markers: []Marker{
			{File: "next.config.js"}, {File: "next.config.mjs"}, {File: "next.config.ts"}, {File: "package.json", Contains: "\"next\""},
		}},
		{ID: "express", Priority: 30, Keywords: kw(`\bexpress(\.js)?\b`), Markers: []Marker{
			{File: "package.json", Contains: "\"express\""},
		}},
		{ID: "react", Priority: 10, Keywords: kw(`\breact\b|\bjsx\b|\bvite\b`), Markers: []Marker{
			{File: "package.json", Contains: "\"react\""},
		}},
	}
}

func (javascript) Scaffold(ctx context.Context, r Req) (Result, error) {
	res, err := jsRecipe(r.Framework).run(ctx, r)
	if err == nil {
		res.Lang = "javascript"
	}
	return res, err
}

// Install runs `npm install` (adding the given packages, or restoring deps if
// none) in the project directory.
func (javascript) Install(ctx context.Context, workDir string, pkgs []string) error {
	ctx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()
	return runCmd(ctx, workDir, "npm", append([]string{"install"}, pkgs...))
}

func jsRecipe(framework string) Recipe {
	switch framework {
	case "nextjs":
		return Recipe{
			Framework: "nextjs", Requires: []string{"node", "npm", "npx"}, Timeout: 420 * time.Second,
			Project: "web", App: "web", Settings: "next.config.ts",
			Stack: "Next.js (App Router, TypeScript)", Entry: "npm run dev",
			Generate: &Cmd{Bin: "npx", Args: []string{"--yes", "create-next-app@15", ".harness_scaffold",
				"--ts", "--app", "--use-npm", "--no-src-dir", "--no-tailwind", "--no-eslint", "--no-turbopack", "--import-alias", "@/*"}},
			Nest: NestTempMove, Verify: "package.json",
			Post: []Post{
				{Render: "js/next_db.ts.tmpl", To: "lib/db.ts", When: "has:postgres"},
			},
			Layout: []string{"package.json", "next.config.ts", "app/page.tsx", "app/layout.tsx"},
		}
	case "react":
		return Recipe{
			Framework: "react", Requires: []string{"node", "npm", "npx"}, Timeout: 420 * time.Second,
			Project: "web", App: "web", Settings: "vite.config.ts",
			Stack: "React + Vite (TypeScript)", Entry: "npm run dev",
			Generate: &Cmd{Bin: "npm", Args: []string{"create", "vite@latest", ".harness_scaffold", "--", "--template", "react-ts"}},
			Nest:     NestTempMove, Verify: "package.json",
			Post: []Post{
				{Run: &Cmd{Bin: "npm", Args: []string{"install"}}},
			},
			Layout: []string{"package.json", "vite.config.ts", "src/App.tsx", "index.html"},
		}
	default: // express — template-only skeleton, then a real npm install
		return Recipe{
			Framework: "express", Requires: []string{"node", "npm"}, Timeout: 240 * time.Second,
			Project: "api", App: "api", Settings: "index.js",
			Stack: "Express (Node.js)", Entry: "node index.js",
			Post: []Post{
				{Render: "js/express_package.json.tmpl", To: "package.json"},
				{Render: "js/express_index.js.tmpl", To: "index.js"},
				{Render: "js/express_db.js.tmpl", To: "db.js", When: "has:postgres"},
				{Run: &Cmd{Bin: "npm", Args: []string{"install"}}},
			},
			Layout: []string{"package.json", "index.js"},
		}
	}
}
