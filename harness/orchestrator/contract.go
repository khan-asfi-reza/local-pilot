package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// loadContract reads apispec.json from the app dir or the monorepo root one level up
// (a backend/ app's contract lives at the project root), or nil if none.
func loadContract(dir string) *APIContract {
	for _, p := range []string{filepath.Join(dir, "apispec.json"), filepath.Join(filepath.Dir(dir), "apispec.json")} {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var c APIContract
		if json.Unmarshal(b, &c) == nil && len(c.Endpoints) > 0 {
			return &c
		}
	}
	return nil
}

// probeContract tests the running backend for CONFORMANCE to the contract: every
// declared endpoint must exist for its method (not 404/405) and not 5xx, and a GET
// that the contract says returns a list must return a JSON array. A 400/401/403/422
// passes — the route exists and is validating/guarding. Returns human-readable
// failures for the repair child.
func probeContract(c APIContract, port string) []string {
	client := &http.Client{Timeout: 5 * time.Second}
	var fails []string
	for _, e := range c.Endpoints {
		path := substParams(e.Path)
		url := "http://localhost:" + port + path
		var body io.Reader
		if hasBody(e.Method) {
			body = bytes.NewReader(sampleBody(e.Request))
		}
		req, err := http.NewRequest(e.Method, url, body)
		if err != nil {
			continue
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(req)
		if err != nil {
			fails = append(fails, e.Method+" "+path+" -> connection failed")
			continue
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 200_000))
		code := resp.StatusCode
		_ = resp.Body.Close()
		if code == 404 || code == 405 || code >= 500 {
			fails = append(fails, fmt.Sprintf("%s %s -> %d (contract endpoint missing or erroring — implement it exactly)", e.Method, path, code))
			continue
		}
		// Shape: a contract list endpoint must return a JSON array on success.
		if e.Method == "GET" && code == 200 && e.ResponseList && !looksJSONArray(raw) {
			fails = append(fails, fmt.Sprintf("GET %s -> 200 but body is not a JSON array (contract says it returns a list)", path))
		}
	}
	sort.Strings(fails)
	return fails
}

// substParams replaces {id}/:id path params with a sample value so the path is
// concrete (seeded data means id 1 usually exists).
func substParams(path string) string {
	segs := strings.Split(path, "/")
	for i, s := range segs {
		if isParamSeg(s) {
			segs[i] = "1"
		}
	}
	return strings.Join(segs, "/")
}

// sampleBody builds a plausible JSON body from request fields by type.
func sampleBody(fields []APIField) []byte {
	m := map[string]any{}
	for _, f := range fields {
		switch tsType(f.Type) {
		case "number":
			m[f.Name] = 1
		case "boolean":
			m[f.Name] = true
		default:
			if strings.Contains(strings.ToLower(f.Type), "date") {
				m[f.Name] = "2024-01-01T00:00:00Z"
			} else {
				m[f.Name] = "test"
			}
		}
	}
	b, _ := json.Marshal(m)
	return b
}

func looksJSONArray(b []byte) bool {
	s := strings.TrimSpace(string(b))
	return strings.HasPrefix(s, "[")
}

// writeTSClient writes the generated typed client to the frontend's src/lib/api.ts
// (monorepo frontend/ or a monolith src/), so components import typed functions.
func writeTSClient(workDir string, c APIContract) {
	for _, base := range []string{filepath.Join(workDir, "frontend", "src"), filepath.Join(workDir, "src")} {
		if _, err := os.Stat(base); err != nil {
			continue
		}
		dir := filepath.Join(base, "lib")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			continue
		}
		_ = os.WriteFile(filepath.Join(dir, "api.ts"), []byte(c.generateTSClient()), 0o644)
		return
	}
}

// APIField is one field of a request body or response object (name + a coarse type:
// string|int|number|bool|date|id).
type APIField struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// APIEndpoint is one REST endpoint in the contract — the single source of truth both
// the backend (implements it) and the frontend (calls it via a generated client) use.
type APIEndpoint struct {
	Method       string     `json:"method"`
	Path         string     `json:"path"`
	Summary      string     `json:"summary"`
	Auth         bool       `json:"auth"`
	Request      []APIField `json:"request"`
	Response     []APIField `json:"response"`
	ResponseList bool       `json:"response_list"`
}

// APIContract is the whole app's REST surface.
type APIContract struct {
	Endpoints []APIEndpoint `json:"endpoints"`
}

// contractSchema is deliberately FLAT (no nested arrays of objects): request and
// response are simple "name:type, name:type" strings. Deeply-nested schemas make a
// small model's grammar-constrained decoder return empty; flat fields are reliable.
const contractSchema = `{"type":"object","properties":{"endpoints":{"type":"array","items":{"type":"object","properties":{` +
	`"method":{"type":"string","enum":["GET","POST","PUT","PATCH","DELETE"]},` +
	`"path":{"type":"string"},` +
	`"summary":{"type":"string"},` +
	`"auth":{"type":"boolean"},` +
	`"request":{"type":"string"},` +
	`"response":{"type":"string"},` +
	`"response_list":{"type":"boolean"}` +
	`},"required":["method","path","summary","auth","request","response","response_list"]}}},"required":["endpoints"]}`

type rawEndpoint struct {
	Method       string `json:"method"`
	Path         string `json:"path"`
	Summary      string `json:"summary"`
	Auth         bool   `json:"auth"`
	Request      string `json:"request"`
	Response     string `json:"response"`
	ResponseList bool   `json:"response_list"`
}

// GenerateContract designs the app's REST API from the PRD as one structured call —
// the contract both sides build against, so they cannot drift.
func GenerateContract(ctx context.Context, p Planner, prd string) (APIContract, error) {
	sys := "You are an API designer. From the product spec, design the COMPLETE REST API the frontend needs, as a " +
		"contract. List EVERY endpoint required to satisfy the features. Rules: every path starts with /api/ and uses " +
		"consistent, plural, snake/kebab resource names with {id} for a single item (e.g. GET /api/doctors, GET " +
		"/api/doctors/{id}, POST /api/appointments, GET /api/doctors/{id}/availability). method is the HTTP verb. auth " +
		"is true if the endpoint requires a logged-in user. request = the JSON body fields for POST/PUT/PATCH as a " +
		"comma-separated \"name:type\" string (empty \"\" for GET/DELETE). response = the returned object's fields as a " +
		"\"name:type\" string (for a list, the fields of ONE item). type is one of string|int|number|bool|date|id. " +
		"response_list = true if the endpoint returns an array. Example request: \"doctor_id:int, slot:date\". Example " +
		"response: \"id:int, name:string, specialty:string\". Use snake_case field names matching the data model. Do " +
		"NOT include /api/auth/register, /api/auth/login, /api/auth/me — they already exist. Cover the whole spec. " +
		"Output ONLY the JSON."
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		raw, err := p.PlanJSON(ctx, sys, clip(prd, 8000), json.RawMessage(contractSchema))
		if err != nil {
			last = err
			continue
		}
		var out struct {
			Endpoints []rawEndpoint `json:"endpoints"`
		}
		if json.Unmarshal([]byte(raw), &out) != nil || len(out.Endpoints) == 0 {
			last = fmt.Errorf("contract came back empty/unparseable")
			continue
		}
		c := APIContract{}
		for _, r := range out.Endpoints {
			c.Endpoints = append(c.Endpoints, APIEndpoint{
				Method: r.Method, Path: r.Path, Summary: r.Summary, Auth: r.Auth,
				Request: parseFieldStr(r.Request), Response: parseFieldStr(r.Response), ResponseList: r.ResponseList,
			})
		}
		return normalizeContract(c), nil
	}
	if last == nil {
		last = fmt.Errorf("contract came back empty")
	}
	return APIContract{}, last
}

// errText renders a contract-generation error for the log ("no endpoints" if nil).
func errText(err error) string {
	if err == nil {
		return "no endpoints"
	}
	return err.Error()
}

// parseFieldStr parses "name:type, name2:type2" into []APIField (tolerant of spaces,
// missing types, and stray separators).
func parseFieldStr(s string) []APIField {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" || strings.EqualFold(s, "none") {
		return nil
	}
	var out []APIField
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == '\n' }) {
		part = strings.TrimSpace(strings.Trim(part, "{}[]() "))
		if part == "" {
			continue
		}
		name, typ := part, "string"
		if i := strings.IndexAny(part, ":="); i >= 0 {
			name = strings.TrimSpace(part[:i])
			typ = strings.TrimSpace(part[i+1:])
		}
		name = strings.Fields(name)[0] // first token, drop trailing words
		if name == "" {
			continue
		}
		out = append(out, APIField{Name: name, Type: typ})
	}
	return out
}

// normalizeContract cleans method casing and ensures every path is under /api.
func normalizeContract(c APIContract) APIContract {
	seen := map[string]bool{}
	var out []APIEndpoint
	for _, e := range c.Endpoints {
		e.Method = strings.ToUpper(strings.TrimSpace(e.Method))
		e.Path = strings.TrimSpace(e.Path)
		// A query string is NOT part of the REST path — strip it (the model sometimes
		// writes ?a={x}&b={y}); a client passes query params separately.
		if i := strings.IndexByte(e.Path, '?'); i >= 0 {
			e.Path = e.Path[:i]
		}
		e.Path = strings.TrimRight(e.Path, "/")
		if e.Path == "" {
			e.Path = "/api"
		}
		if !strings.HasPrefix(e.Path, "/") {
			e.Path = "/" + e.Path
		}
		if !strings.HasPrefix(e.Path, "/api/") && e.Path != "/api" {
			e.Path = "/api" + e.Path
		}
		key := e.Method + " " + e.Path
		if e.Method == "" || e.Path == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, e)
	}
	return APIContract{Endpoints: out}
}

// renderForPrompt is the compact contract text injected into decompose and child
// prompts so both sides implement/consume the SAME endpoints.
func (c APIContract) renderForPrompt() string {
	if len(c.Endpoints) == 0 {
		return ""
	}
	var b strings.Builder
	for _, e := range c.Endpoints {
		b.WriteString(e.Method + " " + e.Path)
		if e.Auth {
			b.WriteString("  [auth]")
		}
		if e.Summary != "" {
			b.WriteString("  — " + e.Summary)
		}
		if len(e.Request) > 0 {
			b.WriteString("\n    body: " + fieldList(e.Request))
		}
		shape := "{" + fieldList(e.Response) + "}"
		if e.ResponseList {
			shape = "[" + shape + "]"
		}
		b.WriteString("\n    returns: " + shape + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func fieldList(fs []APIField) string {
	parts := make([]string, 0, len(fs))
	for _, f := range fs {
		parts = append(parts, f.Name+":"+f.Type)
	}
	return strings.Join(parts, ", ")
}

// openAPIYAML renders the contract as a readable OpenAPI 3 YAML doc (for humans and
// as the artifact the user asked for). Hand-rolled so we add no YAML dependency.
func (c APIContract) openAPIYAML() string {
	var b strings.Builder
	b.WriteString("openapi: 3.0.0\ninfo:\n  title: Generated API\n  version: 1.0.0\npaths:\n")
	// group by path
	byPath := map[string][]APIEndpoint{}
	var paths []string
	for _, e := range c.Endpoints {
		if _, ok := byPath[e.Path]; !ok {
			paths = append(paths, e.Path)
		}
		byPath[e.Path] = append(byPath[e.Path], e)
	}
	sort.Strings(paths)
	for _, p := range paths {
		b.WriteString("  " + p + ":\n")
		for _, e := range byPath[p] {
			b.WriteString("    " + strings.ToLower(e.Method) + ":\n")
			b.WriteString("      summary: " + yamlStr(e.Summary) + "\n")
			if e.Auth {
				b.WriteString("      security: [{ bearerAuth: [] }]\n")
			}
			b.WriteString("      responses:\n        '200':\n          description: ok\n")
		}
	}
	return b.String()
}

func yamlStr(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if s == "" {
		return "\"\""
	}
	return "\"" + strings.ReplaceAll(s, "\"", "'") + "\""
}

// generateTSClient emits a typed TS API client from the contract. The frontend
// imports these functions instead of hardcoding fetch paths, so paths and types can
// never drift from the backend — both derive from the same contract.
func (c APIContract) generateTSClient() string {
	var b strings.Builder
	b.WriteString(`// AUTO-GENERATED from the API contract (openapi.yaml). Do NOT edit by hand.
// Import these typed functions in your components — never hardcode fetch('/api/...').
// Every request goes through the Vite /api proxy to the backend and attaches the
// JWT from localStorage (set it after login: localStorage.setItem('token', token)).

export function authHeaders(): Record<string, string> {
  const t = typeof localStorage !== 'undefined' ? localStorage.getItem('token') : null;
  return t ? { Authorization: ` + "`Bearer ${t}`" + ` } : {};
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: { 'Content-Type': 'application/json', ...authHeaders() },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) throw new Error(` + "`${method} ${path} -> ${res.status}`" + `);
  const text = await res.text();
  return (text ? JSON.parse(text) : (null as unknown)) as T;
}

function qs(q?: Record<string, string | number | boolean | undefined>): string {
  if (!q) return '';
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(q)) if (v !== undefined) p.set(k, String(v));
  const s = p.toString();
  return s ? ` + "`?${s}`" + ` : '';
}

`)
	names := map[string]int{}
	for _, e := range c.Endpoints {
		op := uniqueName(opName(e), names)
		resultT := op + "Result"
		// response interface
		if len(e.Response) > 0 {
			b.WriteString("export interface " + resultT + " {\n")
			for _, f := range e.Response {
				b.WriteString("  " + f.Name + ": " + tsType(f.Type) + ";\n")
			}
			b.WriteString("}\n")
		}
		ret := "void"
		switch {
		case len(e.Response) == 0:
			ret = "void"
		case e.ResponseList:
			ret = resultT + "[]"
		default:
			ret = resultT
		}
		// function signature: path params become args, body for write methods, and an
		// optional query object for GET (covers filter/search endpoints).
		params, pathExpr := clientParams(e.Path)
		args := params
		callBody := "undefined"
		if hasBody(e.Method) && len(e.Request) > 0 {
			bodyT := "{ " + tsFields(e.Request) + " }"
			if args != "" {
				args += ", "
			}
			args += "body: " + bodyT
			callBody = "body"
		}
		if e.Method == "GET" {
			if args != "" {
				args += ", "
			}
			args += "query?: Record<string, string | number | boolean | undefined>"
			pathExpr = pathExpr + " + qs(query)"
		}
		b.WriteString("export function " + op + "(" + args + "): Promise<" + ret + "> {\n")
		b.WriteString("  return request<" + ret + ">('" + e.Method + "', " + pathExpr + ", " + callBody + ");\n")
		b.WriteString("}\n\n")
	}
	return b.String()
}

func hasBody(method string) bool {
	return method == "POST" || method == "PUT" || method == "PATCH"
}

// opName derives a stable client function name from an endpoint.
func opName(e APIEndpoint) string {
	verb := map[string]string{"GET": "get", "POST": "create", "PUT": "update", "PATCH": "update", "DELETE": "remove"}[e.Method]
	if verb == "" {
		verb = strings.ToLower(e.Method)
	}
	if e.Method == "GET" && e.ResponseList {
		verb = "list"
	}
	name := verb
	segs := strings.Split(strings.TrimPrefix(e.Path, "/api/"), "/")
	trailingParam := ""
	for _, s := range segs {
		if s == "" {
			continue
		}
		if isParamSeg(s) {
			trailingParam = paramName(s)
			continue
		}
		trailingParam = ""
		name += pascal(s)
	}
	if trailingParam != "" {
		name += "By" + pascal(trailingParam)
	}
	return sanitizeIdent(name)
}

// sanitizeIdent forces a string into a valid JS identifier (letters/digits, leading
// letter) so a stray path character can never produce uncompilable client code.
func sanitizeIdent(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		return "op"
	}
	if out[0] >= '0' && out[0] <= '9' {
		out = "op" + out
	}
	return out
}

func uniqueName(n string, seen map[string]int) string {
	seen[n]++
	if seen[n] == 1 {
		return n
	}
	return fmt.Sprintf("%s%d", n, seen[n])
}

// clientParams returns the TS arg list for path params and the template-literal path.
func clientParams(path string) (args string, pathExpr string) {
	segs := strings.Split(path, "/")
	var params []string
	var out []string
	for _, s := range segs {
		if isParamSeg(s) {
			p := paramName(s)
			params = append(params, p+": string | number")
			out = append(out, "${"+p+"}")
		} else {
			out = append(out, s)
		}
	}
	pathExpr = "`" + strings.Join(out, "/") + "`"
	return strings.Join(params, ", "), pathExpr
}

func isParamSeg(s string) bool {
	return strings.HasPrefix(s, "{") || strings.HasPrefix(s, ":")
}

func paramName(s string) string {
	s = strings.TrimPrefix(strings.TrimSuffix(strings.TrimPrefix(s, "{"), "}"), ":")
	if s == "" {
		return "id"
	}
	return s
}

func tsFields(fs []APIField) string {
	parts := make([]string, 0, len(fs))
	for _, f := range fs {
		parts = append(parts, f.Name+": "+tsType(f.Type))
	}
	return strings.Join(parts, "; ")
}

func tsType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "int", "integer", "number", "float", "id", "decimal":
		return "number"
	case "bool", "boolean":
		return "boolean"
	case "date", "datetime", "time", "timestamp", "string", "text", "uuid", "email":
		return "string"
	default:
		return "any"
	}
}

func pascal(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "-", "_"), ".", "_")
	var b strings.Builder
	for _, part := range strings.Split(s, "_") {
		if part == "" {
			continue
		}
		b.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	return b.String()
}
