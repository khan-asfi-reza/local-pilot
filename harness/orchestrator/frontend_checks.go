package orchestrator

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	jsxComponentRe   = regexp.MustCompile(`<([A-Z][A-Za-z0-9_]*)\b`)
	namedImportRe    = regexp.MustCompile(`import\s+(?:type\s+)?\{([^}]*)\}\s+from\s+['"]([^'"]+)['"]`)
	defaultImportRe  = regexp.MustCompile(`import\s+([A-Z][A-Za-z0-9_]*)\s*(?:,\s*\{[^}]*\})?\s+from\s+['"]([^'"]+)['"]`)
	publicAssetRe    = regexp.MustCompile(`['"](/[A-Za-z0-9_\-./]+\.(?:png|jpe?g|gif|svg|webp|avif|mp4|webm|mp3|wav))['"]`)
	routerPrimitives = []string{"Link", "NavLink", "Outlet", "useNavigate", "useLocation", "useParams", "useSearchParams", "useRoutes", "useMatch"}
	componentExts    = []string{".tsx", ".jsx", ".ts", ".js"}
)

var (
	blockCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)
	lineCommentRe  = regexp.MustCompile(`(?m)(^|[^:"'\\])//.*$`)
)

// stripComments removes comment text so a tag or path merely DESCRIBED in a
// comment is never read as code. The line rule keeps a "//" that follows a colon
// or quote, so a URL inside a string survives.
func stripComments(src string) string {
	src = blockCommentRe.ReplaceAllString(src, " ")
	return lineCommentRe.ReplaceAllString(src, "$1")
}

// routerOutsideProviderIssues reports a component rendered as a sibling of
// <RouterProvider> that itself uses <Link> or a router hook.
//
// React Router reads its context from the provider, so such a component throws on
// its first render, React unmounts the whole tree, and the page goes blank with an
// empty #root. The build still passes — it is a runtime fault, not a type error —
// which is exactly how a "finished" app ships showing nothing at all.
func routerOutsideProviderIssues(dir string) []string {
	var issues []string
	for _, file := range componentFiles(dir) {
		body, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		src := stripComments(string(body))
		if !strings.Contains(src, "<RouterProvider") {
			continue
		}
		for _, name := range siblingComponents(src) {
			target := resolveLocalImport(file, importSourceFor(src, name))
			if target == "" {
				continue
			}
			if prim := routerPrimitiveUsed(target); prim != "" {
				issues = append(issues, "in "+filepath.Base(file)+", <"+name+"> is rendered outside <RouterProvider> but uses "+prim+
					" — React Router throws there and the page renders blank. Move it inside the router (a parent route whose element renders it around an <Outlet />)")
			}
		}
	}
	sort.Strings(issues)
	return issues
}

// siblingComponents lists the component tags in a RouterProvider file other than
// RouterProvider itself — the ones rendered beside it rather than through a route.
func siblingComponents(src string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range jsxComponentRe.FindAllStringSubmatch(src, -1) {
		name := m[1]
		if name == "RouterProvider" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// importSourceFor returns the module specifier `name` is imported from, or "".
func importSourceFor(src, name string) string {
	for _, m := range namedImportRe.FindAllStringSubmatch(src, -1) {
		for _, part := range strings.Split(m[1], ",") {
			part = strings.TrimSpace(part)
			if i := strings.Index(part, " as "); i >= 0 {
				part = strings.TrimSpace(part[i+4:])
			}
			if part == name {
				return m[2]
			}
		}
	}
	for _, m := range defaultImportRe.FindAllStringSubmatch(src, -1) {
		if m[1] == name {
			return m[2]
		}
	}
	return ""
}

// resolveLocalImport turns a relative import into the file it points at, or "".
func resolveLocalImport(fromFile, spec string) string {
	if spec == "" || !strings.HasPrefix(spec, ".") {
		return "" // a package, not a file in this app
	}
	base := filepath.Join(filepath.Dir(fromFile), spec)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	for _, ext := range componentExts {
		for _, cand := range []string{base + ext, filepath.Join(base, "index"+ext)} {
			if fileExists(cand) {
				return cand
			}
		}
	}
	// The spec may already carry its extension (e.g. './App.tsx').
	if orig := filepath.Join(filepath.Dir(fromFile), spec); fileExists(orig) {
		return orig
	}
	return ""
}

// routerPrimitiveUsed names a react-router primitive the file imports and uses.
func routerPrimitiveUsed(file string) string {
	body, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	src := stripComments(string(body))
	for _, m := range namedImportRe.FindAllStringSubmatch(src, -1) {
		if !strings.HasPrefix(m[2], "react-router") {
			continue
		}
		for _, part := range strings.Split(m[1], ",") {
			part = strings.TrimSpace(part)
			for _, prim := range routerPrimitives {
				if part == prim {
					return "<" + prim + ">"
				}
			}
		}
	}
	return ""
}

// missingPublicAssets reports absolute asset paths the UI renders that have no
// file to serve. A dev server answers them with index.html, so the page loads but
// every image is broken — a defect no build or type check can see.
func missingPublicAssets(dir string) []string {
	publicDir := filepath.Join(dir, "public")
	if info, err := os.Stat(publicDir); err != nil || !info.IsDir() {
		return nil
	}
	missing := map[string]bool{}
	for _, file := range componentFiles(dir) {
		body, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		for _, m := range publicAssetRe.FindAllStringSubmatch(stripComments(string(body)), -1) {
			ref := m[1]
			if fileExists(filepath.Join(publicDir, filepath.FromSlash(strings.TrimPrefix(ref, "/")))) {
				continue
			}
			if fileExists(filepath.Join(dir, "src", filepath.FromSlash(strings.TrimPrefix(ref, "/")))) {
				continue
			}
			missing[ref] = true
		}
	}
	if len(missing) == 0 {
		return nil
	}
	var refs []string
	for ref := range missing {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	if len(refs) > 6 {
		refs = refs[:6]
	}
	return []string{"these asset paths are rendered but no file exists to serve them, so every one shows as a broken image: " +
		strings.Join(refs, ", ") + " — create the files under public/, or point the code at images that exist"}
}

// componentFiles lists an app's own source files, skipping vendored trees.
func componentFiles(dir string) []string {
	var out []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", ".git", "dist", "build", ".next":
				return fs.SkipDir
			}
			return nil
		}
		switch filepath.Ext(p) {
		case ".tsx", ".jsx", ".ts", ".js":
			if len(out) < 400 {
				out = append(out, p)
			}
		}
		return nil
	})
	return out
}
