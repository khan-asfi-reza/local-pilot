package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeApp(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// The exact shape that shipped as a blank page: the nav is a sibling of
// <RouterProvider>, so its <Link>s have no router context and React unmounts.
func TestRouterOutsideProviderIsCaught(t *testing.T) {
	dir := writeApp(t, map[string]string{
		"package.json": `{"dependencies":{"react-router-dom":"^6"}}`,
		"src/App.tsx": `import { RouterProvider } from 'react-router-dom'
import { NavigationBar } from './components/Navigation'
import router from './routes'
function App() {
  return (<div><NavigationBar /><RouterProvider router={router} /></div>)
}
export default App`,
		"src/components/Navigation.tsx": `import { Link } from 'react-router-dom';
export function NavigationBar() { return <nav><Link to="/">Home</Link></nav>; }`,
	})

	issues := routerOutsideProviderIssues(dir)
	if len(issues) != 1 {
		t.Fatalf("got %d issues, want 1: %v", len(issues), issues)
	}
	if !strings.Contains(issues[0], "NavigationBar") || !strings.Contains(issues[0], "<Link>") {
		t.Errorf("issue should name the component and the primitive, got %q", issues[0])
	}
}

// The corrected shape: the nav sits inside a layout route, not beside the provider.
func TestLayoutRouteIsNotFlagged(t *testing.T) {
	dir := writeApp(t, map[string]string{
		"package.json": `{"dependencies":{"react-router-dom":"^6"}}`,
		"src/App.tsx": `import { RouterProvider } from 'react-router-dom'
import router from './routes'
function App() { return <RouterProvider router={router} /> }
export default App`,
		"src/components/Layout.tsx": `import { Outlet } from 'react-router-dom';
import { NavigationBar } from './Navigation';
export function Layout() { return <div><NavigationBar /><Outlet /></div>; }`,
		"src/components/Navigation.tsx": `import { Link } from 'react-router-dom';
export function NavigationBar() { return <nav><Link to="/">Home</Link></nav>; }`,
	})

	if issues := routerOutsideProviderIssues(dir); len(issues) != 0 {
		t.Errorf("a layout route must not be flagged, got %v", issues)
	}
}

// A sibling that does not touch the router is fine where it is.
func TestNonRouterSiblingIsNotFlagged(t *testing.T) {
	dir := writeApp(t, map[string]string{
		"package.json": `{"dependencies":{"react-router-dom":"^6"}}`,
		"src/App.tsx": `import { RouterProvider } from 'react-router-dom'
import { Toaster } from './components/Toaster'
import router from './routes'
function App() { return (<div><Toaster /><RouterProvider router={router} /></div>) }
export default App`,
		"src/components/Toaster.tsx": `export function Toaster() { return <div id="toasts" />; }`,
	})

	if issues := routerOutsideProviderIssues(dir); len(issues) != 0 {
		t.Errorf("a router-free sibling must not be flagged, got %v", issues)
	}
}

func TestMissingPublicAssetsAreCaught(t *testing.T) {
	dir := writeApp(t, map[string]string{
		"public/favicon.svg": "<svg/>",
		"src/data/games.ts": `export const games = [
  { id: '1', image: '/assets/games/cyber-odyssey.png' },
  { id: '2', image: '/assets/games/forest-whisperer.png' },
  { id: '3', image: '/favicon.svg' },
];`,
	})

	issues := missingPublicAssets(dir)
	if len(issues) != 1 {
		t.Fatalf("got %d issues, want 1: %v", len(issues), issues)
	}
	if !strings.Contains(issues[0], "cyber-odyssey.png") || !strings.Contains(issues[0], "forest-whisperer.png") {
		t.Errorf("issue should list the missing paths, got %q", issues[0])
	}
	if strings.Contains(issues[0], "favicon.svg") {
		t.Errorf("an asset that exists must not be reported, got %q", issues[0])
	}
}

func TestPresentAssetsProduceNoIssue(t *testing.T) {
	dir := writeApp(t, map[string]string{
		"public/hero.png":  "\x89PNG",
		"src/App.tsx":      `export default () => <img src="/hero.png" />`,
	})

	if issues := missingPublicAssets(dir); len(issues) != 0 {
		t.Errorf("got %v, want no issues", issues)
	}
}

// A tag or asset path merely described in a comment is documentation, not code.
func TestCommentsAreNotReadAsCode(t *testing.T) {
	dir := writeApp(t, map[string]string{
		"package.json": `{"dependencies":{"react-router-dom":"^6"}}`,
		"public/ok.png": "\x89PNG",
		"src/components/Layout.tsx": `import { Outlet } from 'react-router-dom';
import { NavigationBar } from './Navigation';
// Lives inside the router; rendering it beside <RouterProvider> blanks the page.
/* see also: <RouterProvider router={router} /> and '/assets/not-real.png' */
export function Layout() { return <div><NavigationBar /><Outlet /><img src="/ok.png" /></div>; }`,
		"src/components/Navigation.tsx": `import { Link } from 'react-router-dom';
export function NavigationBar() { return <nav><Link to="/">Home</Link></nav>; }`,
	})

	if issues := routerOutsideProviderIssues(dir); len(issues) != 0 {
		t.Errorf("a commented-out tag must not be flagged, got %v", issues)
	}
	if issues := missingPublicAssets(dir); len(issues) != 0 {
		t.Errorf("a commented-out asset path must not be flagged, got %v", issues)
	}
}

func TestStripCommentsKeepsURLsInStrings(t *testing.T) {
	src := `const api = "https://example.com/x"; // trailing note`
	got := stripComments(src)
	if !strings.Contains(got, "https://example.com/x") {
		t.Errorf("a URL inside a string must survive, got %q", got)
	}
	if strings.Contains(got, "trailing note") {
		t.Errorf("the trailing comment should be gone, got %q", got)
	}
}
