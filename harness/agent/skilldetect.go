package agent

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"harness/harness/model"
)

// internalTrigger declares when an internal skill applies: by marker files in the
// project (optionally containing a substring, e.g. package.json contains "next"),
// or by keywords in what the user asked. Frameworks carry a higher priority than
// languages so a Django task gets the django guidance rather than only python.
type internalTrigger struct {
	skill    string
	kind     string // "framework" or "language"
	markers  []marker
	keywords *regexp.Regexp
}

type marker struct {
	file     string // marker path relative to the working dir (glob-free)
	contains string // if set, the file must contain this substring
}

// triggers is the detection table. It references internal skills by name; a
// missing skill (not installed) is simply skipped. Order does not matter — the
// caller picks the best framework and the best language.
var triggers = []internalTrigger{
	// Frameworks. Order matters: a matched framework wins over later ones on a tie,
	// so specific meta-frameworks come before the base UI library they build on
	// (e.g. next/remix before react, which their package.json also lists).
	{"nextjs", "framework", []marker{{"next.config.js", ""}, {"next.config.mjs", ""}, {"next.config.ts", ""}, {"package.json", "\"next\""}}, kw(`next\.?js`)},
	{"remix", "framework", []marker{{"package.json", "@remix-run"}}, kw(`\bremix\b`)},
	{"astro", "framework", []marker{{"astro.config.mjs", ""}, {"astro.config.ts", ""}, {"package.json", "\"astro\""}}, kw(`\bastro\b`)},
	{"sveltekit", "framework", []marker{{"svelte.config.js", ""}, {"package.json", "@sveltejs/kit"}}, kw(`sveltekit`)},
	{"nestjs", "framework", []marker{{"nest-cli.json", ""}, {"package.json", "@nestjs/core"}}, kw(`nest\.?js`)},
	{"react-native", "framework", []marker{{"package.json", "react-native"}, {"app.json", "expo"}}, kw(`react[ -]native|\bexpo\b`)},
	{"react", "framework", []marker{{"package.json", "\"react\""}}, kw(`\breact\b|\bjsx\b`)},
	{"vue", "framework", []marker{{"vue.config.js", ""}, {"package.json", "\"vue\""}, {"nuxt.config.ts", ""}}, kw(`\bvue\b|\bnuxt\b`)},
	{"svelte", "framework", []marker{{"package.json", "\"svelte\""}}, kw(`\bsvelte\b`)},
	{"angular", "framework", []marker{{"angular.json", ""}, {"package.json", "@angular/core"}}, kw(`\bangular\b`)},
	{"flutter", "framework", []marker{{"pubspec.yaml", ""}}, kw(`\bflutter\b|\bdart\b`)},
	{"express", "framework", []marker{{"package.json", "\"express\""}}, kw(`\bexpress(\.js)?\b`)},
	{"fastapi", "framework", []marker{{"requirements.txt", "fastapi"}, {"pyproject.toml", "fastapi"}}, kw(`\bfastapi\b`)},
	{"flask", "framework", []marker{{"requirements.txt", "Flask"}, {"requirements.txt", "flask"}}, kw(`\bflask\b`)},
	{"django", "framework", []marker{{"manage.py", ""}, {"requirements.txt", "Django"}, {"pyproject.toml", "django"}}, kw(`\bdjango\b`)},
	{"laravel", "framework", []marker{{"artisan", ""}, {"composer.json", "laravel/framework"}}, kw(`\blaravel\b`)},
	{"rails", "framework", []marker{{"bin/rails", ""}, {"Gemfile", "rails"}}, kw(`\brails\b|ruby on rails`)},
	{"spring-boot", "framework", []marker{{"pom.xml", "spring-boot"}, {"build.gradle", "org.springframework.boot"}}, kw(`spring ?boot|\bspring\b`)},
	{"tailwind", "framework", []marker{{"tailwind.config.js", ""}, {"tailwind.config.ts", ""}, {"package.json", "tailwindcss"}}, kw(`tailwind`)},
	{"htmx", "framework", nil, kw(`\bhtmx\b`)},
	// Web game libraries (also "framework" kind — one is chosen alongside a language).
	{"phaser", "framework", []marker{{"package.json", "\"phaser\""}}, kw(`\bphaser\b`)},
	{"threejs", "framework", []marker{{"package.json", "\"three\""}}, kw(`three\.?js|threejs`)},
	{"babylonjs", "framework", []marker{{"package.json", "babylonjs"}, {"package.json", "@babylonjs"}}, kw(`babylon(\.?js)?\b`)},
	{"pixijs", "framework", []marker{{"package.json", "pixi.js"}}, kw(`\bpixi(\.js)?\b`)},
	{"kaplay", "framework", []marker{{"package.json", "kaplay"}, {"package.json", "kaboom"}}, kw(`\bkaplay\b|\bkaboom\b`)},
	{"webgame", "framework", nil, kw(`canvas game|game loop|requestanimationframe|html5 game|2d game|browser game|web game`)},
	// Languages.
	{"python", "language", []marker{{"requirements.txt", ""}, {"pyproject.toml", ""}, {"setup.py", ""}, {"manage.py", ""}}, kw(`\bpython\b|\bflask\b|\bfastapi\b`)},
	{"typescript", "language", []marker{{"tsconfig.json", ""}}, kw(`\btypescript\b|\.tsx?\b`)},
	{"javascript", "language", []marker{{"package.json", ""}}, kw(`\bjavascript\b|\bnode(\.js)?\b`)},
	{"go", "language", []marker{{"go.mod", ""}}, kw(`\bgolang\b|\bgo\b`)},
	{"rust", "language", []marker{{"Cargo.toml", ""}}, kw(`\brust\b|\bcargo\b`)},
	{"java", "language", []marker{{"pom.xml", ""}, {"build.gradle", ""}}, kw(`\bjava\b`)},
	{"cpp", "language", []marker{{"CMakeLists.txt", ""}}, kw(`\bc\+\+\b|\bcpp\b`)},
	{"csharp", "language", []marker{{"Program.cs", ""}}, kw(`\bc#\b|\bcsharp\b|\.net\b|dotnet`)},
	{"ruby", "language", []marker{{"Gemfile", ""}}, kw(`\bruby\b`)},
	{"php", "language", []marker{{"composer.json", ""}}, kw(`\bphp\b`)},
	{"lua", "language", []marker{{"init.lua", ""}}, kw(`\blua\b`)},
}

func kw(pat string) *regexp.Regexp { return regexp.MustCompile(`(?i)` + pat) }

// detectInternalSkills picks the internal skills that fit this task and returns
// their bodies joined for silent injection. At most one framework and one
// language are chosen (framework first), to keep the injected guidance small for
// a small model's context. A file marker is a stronger signal than a keyword.
func detectInternalSkills(workDir string, messages []model.Message, set skillSet) string {
	ask := recentUserText(messages)
	var framework, language string
	var fwFile, langFile bool

	for _, t := range triggers {
		if set.bodies[t.skill] == "" || !set.internal[t.skill] {
			continue // skill not installed, or not marked internal
		}
		fileHit := anyMarker(workDir, t.markers)
		kwHit := t.keywords != nil && ask != "" && t.keywords.MatchString(ask)
		if !fileHit && !kwHit {
			continue
		}
		if t.kind == "framework" {
			// Prefer a file-detected framework; otherwise take the first keyword hit.
			if framework == "" || (fileHit && !fwFile) {
				framework, fwFile = t.skill, fileHit
			}
		} else {
			if language == "" || (fileHit && !langFile) {
				language, langFile = t.skill, fileHit
			}
		}
	}

	var parts []string
	if framework != "" {
		parts = append(parts, set.bodies[framework])
	}
	// Skip the language when it is redundant with the framework's base language.
	if language != "" && !languageImpliedBy(framework, language) {
		parts = append(parts, set.bodies[language])
	}
	return strings.Join(parts, "\n\n")
}

// languageImpliedBy reports whether a framework already covers a language, so the
// separate language skill would be redundant context.
func languageImpliedBy(framework, language string) bool {
	base := map[string]string{
		"nextjs": "typescript", "react": "javascript", "react-native": "javascript",
		"vue": "javascript", "angular": "typescript", "django": "python", "laravel": "php",
		"remix": "typescript", "astro": "javascript", "sveltekit": "typescript", "svelte": "javascript",
		"nestjs": "typescript", "express": "javascript", "fastapi": "python", "flask": "python",
		"rails": "ruby", "spring-boot": "java",
		"phaser": "javascript", "threejs": "javascript", "babylonjs": "javascript",
		"pixijs": "javascript", "kaplay": "javascript", "webgame": "javascript",
	}
	if b, ok := base[framework]; ok {
		return b == language || (language == "javascript" && b == "typescript")
	}
	return false
}

func anyMarker(workDir string, markers []marker) bool {
	if workDir == "" {
		return false
	}
	for _, m := range markers {
		p := filepath.Join(workDir, m.file)
		if m.contains == "" {
			if _, err := os.Stat(p); err == nil {
				return true
			}
			continue
		}
		if b, err := os.ReadFile(p); err == nil && strings.Contains(string(b), m.contains) {
			return true
		}
	}
	return false
}

// recentUserText joins the content of the most recent user messages, which is
// where the task lives, for keyword detection.
func recentUserText(messages []model.Message) string {
	var b strings.Builder
	for i := len(messages) - 1; i >= 0 && i >= len(messages)-3; i-- {
		if messages[i].Role == "user" {
			b.WriteString(messages[i].Content)
			b.WriteString("\n")
		}
	}
	return b.String()
}
