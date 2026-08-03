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
		{ID: "nestjs", Priority: 45, Keywords: kw(`nest\.?js|@nestjs`), Markers: []Marker{
			{File: "nest-cli.json"}, {File: "package.json", Contains: "@nestjs/core"},
		}},
		{ID: "express", Priority: 30, Keywords: kw(`\bexpress(\.js)?\b`), Markers: []Marker{
			{File: "package.json", Contains: "\"express\""},
		}},
		{ID: "react", Priority: 10, Keywords: kw(`\breact\b|\bjsx\b|\bvite\b`), Markers: []Marker{
			{File: "package.json", Contains: "\"react\""},
		}},
		// Generic Node/TypeScript catch: fires for a Node backend / REST API that
		// names no specific framework, so it still gets a real npm-installed skeleton
		// instead of the model hand-writing one. Lowest priority so a named framework
		// always wins.
		{ID: "node", Priority: 3, Keywords: kw(`\bnode(\.?js)?\b|\btypescript\b|\brest api\b|\bapi server\b|\bbackend\b`), Markers: []Marker{
			{File: "tsconfig.json"},
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

// uiLibGuard makes a step apply only when the spec names NO competing UI/design
// library, so a React/Next app with no library specified defaults to Tailwind +
// lucide-react (one that names MUI/Chakra/etc. is left for that library).
const uiLibGuard = "!kw:material-ui|@mui|mui|chakra|ant design|antd|bootstrap|mantine|shadcn|styled-components|styled components|emotion|bulma|daisyui|primereact|vuetify"

func jsRecipe(framework string) Recipe {
	// tailwindSteps is the default Tailwind v3 + lucide-react setup, shared by
	// nextjs (app/globals.css) and vite (src/index.css). Tailwind v3 is pinned so
	// the postcss plugin config stays stable.
	tailwindSteps := func(cssPath, twConfig string) []Post {
		return []Post{
			{Run: &Cmd{Bin: "npm", Args: []string{"install", "-D", "tailwindcss@3", "postcss", "autoprefixer"}}, When: uiLibGuard},
			{Run: &Cmd{Bin: "npm", Args: []string{"install", "lucide-react"}}, When: uiLibGuard},
			{Render: twConfig, To: "tailwind.config.ts", When: uiLibGuard},
			{Render: "js/postcss.config.mjs.tmpl", To: "postcss.config.mjs", When: uiLibGuard},
			{Render: "js/tailwind_globals.css.tmpl", To: cssPath, When: uiLibGuard},
		}
	}
	switch framework {
	case "nextjs":
		return Recipe{
			Framework: "nextjs", Requires: []string{"node", "npm", "npx"}, Timeout: 420 * time.Second,
			Project: "web", App: "web", Settings: "next.config.ts",
			Stack: "Next.js (App Router, TypeScript)", Entry: "npm run dev",
			Generate: &Cmd{Bin: "npx", Args: []string{"--yes", "create-next-app@15", ".harness_scaffold",
				"--ts", "--app", "--use-npm", "--no-src-dir", "--no-tailwind", "--no-eslint", "--no-turbopack", "--import-alias", "@/*"}},
			Nest: NestTempMove, Verify: "package.json",
			Post: append(
				tailwindSteps("app/globals.css", "js/tailwind_next.config.ts.tmpl"),
				Post{Render: "js/next_db.ts.tmpl", To: "lib/db.ts", When: "has:postgres"},
			),
			Layout: []string{"package.json", "next.config.ts", "app/page.tsx", "app/layout.tsx"},
		}
	case "react":
		return Recipe{
			Framework: "react", Requires: []string{"node", "npm", "npx"}, Timeout: 420 * time.Second,
			Project: "web", App: "web", Settings: "vite.config.ts",
			Stack: "React + Vite (TypeScript)", Entry: "npm run dev",
			Generate: &Cmd{Bin: "npm", Args: []string{"create", "vite@latest", ".harness_scaffold", "--", "--template", "react-ts"}},
			Nest:     NestTempMove, Verify: "package.json",
			Post: append(
				[]Post{{Run: &Cmd{Bin: "npm", Args: []string{"install"}}}},
				tailwindSteps("src/index.css", "js/tailwind_vite.config.ts.tmpl")...,
			),
			Layout: []string{"package.json", "vite.config.ts", "src/App.tsx", "index.html"},
		}
	case "nestjs":
		return Recipe{
			Framework: "nestjs", Requires: []string{"node", "npm", "npx"}, Timeout: 600 * time.Second,
			Project: "api", App: "api", Settings: "src/app.module.ts",
			Stack: "NestJS (TypeScript)", Entry: "npm run start:dev",
			Generate: &Cmd{Bin: "npx", Args: []string{"--yes", "@nestjs/cli@latest", "new", ".harness_scaffold",
				"--package-manager", "npm", "--skip-git"}},
			Nest: NestTempMove, Verify: "package.json",
			Layout: []string{"package.json", "src/main.ts", "src/app.module.ts", "src/app.controller.ts"},
		}
	default: // express (TS) and the generic node catch share a TS skeleton + npm install
		stack := "Express (TypeScript)"
		if framework == "node" {
			stack = "Node.js + TypeScript"
		}
		return Recipe{
			Framework: framework, Requires: []string{"node", "npm"}, Timeout: 300 * time.Second,
			Project: "api", App: "api", Settings: "src/index.ts",
			Stack: stack, Entry: "npm run dev",
			Post: []Post{
				{Render: "js/ts_package.json.tmpl", To: "package.json"},
				{Render: "js/tsconfig.json.tmpl", To: "tsconfig.json"},
				{Render: "js/express_index.ts.tmpl", To: "src/index.ts"},
				{Render: "js/express_db.ts.tmpl", To: "src/db.ts", When: "has:postgres"},
				{Run: &Cmd{Bin: "npm", Args: []string{"install"}}},
			},
			Layout: []string{"package.json", "tsconfig.json", "src/index.ts"},
		}
	}
}
