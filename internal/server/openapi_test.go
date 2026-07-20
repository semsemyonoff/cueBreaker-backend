package server

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	specpkg "git.horn/cueBreaker/backend/internal/server/openapi"
)

// spec is the shape of openapi.yaml this package cares about: which methods
// each path documents.
type spec struct {
	OpenAPI string                          `yaml:"openapi"`
	Paths   map[string]map[string]yaml.Node `yaml:"paths"`
}

func parseSpec(t *testing.T) spec {
	t.Helper()
	var s spec
	if err := yaml.Unmarshal(specpkg.Spec, &s); err != nil {
		t.Fatalf("parse openapi.yaml: %v", err)
	}
	return s
}

// wildcard matches a ServeMux wildcard segment, greedy or not: {path...} and
// {job_id} both reduce to the OpenAPI form {path} / {job_id}.
var wildcard = regexp.MustCompile(`\{([^{}]+?)\.\.\.\}`)

// httpMethods is the set of path-item keys that are operations (OpenAPI 3.1 §4.8.9).
var httpMethods = map[string]bool{
	"get": true, "put": true, "post": true, "delete": true,
	"options": true, "head": true, "patch": true, "trace": true,
}

// specForm converts a ServeMux pattern ("GET /api/status/{job_id...}") into
// the method and OpenAPI path ("get", "/api/status/{job_id}") that document
// it.
func specForm(pattern string) (method, path string) {
	method, path, _ = strings.Cut(pattern, " ")
	return strings.ToLower(method), wildcard.ReplaceAllString(path, "{$1}")
}

// TestSpecMatchesRoutes is the drift test. A hand-written spec rots silently,
// so this asserts both directions: every route the server registers is
// documented, and every documented path is registered.
func TestSpecMatchesRoutes(t *testing.T) {
	s, _, _ := testServer(t, nil)
	doc := parseSpec(t)

	documented := map[string]bool{}
	for path, ops := range doc.Paths {
		for method := range ops {
			// A path item's children are not all operations: OpenAPI also allows
			// summary/description/servers/parameters there. Without this filter,
			// hoisting a shared `parameters:` up to the path item would fail the
			// test with "no route registers parameters /api/…" — a spec-shape
			// change reported as route drift.
			if !httpMethods[method] {
				continue
			}
			documented[method+" "+path] = true
		}
	}

	registered := map[string]bool{}
	for _, route := range s.apiRoutes() {
		method, path := specForm(route.pattern)
		registered[method+" "+path] = true
	}

	for op := range registered {
		if !documented[op] {
			t.Errorf("route %q is registered but missing from openapi.yaml", op)
		}
	}
	for op := range documented {
		if !registered[op] {
			t.Errorf("openapi.yaml documents %q, which no route registers", op)
		}
	}

	if t.Failed() {
		t.Logf("registered: %v", sortedKeys(registered))
		t.Logf("documented: %v", sortedKeys(documented))
	}
}

// TestSpecDocumentsLogSchemas asserts the schemas backing the split/scan log
// feature exist and are referenced from the right places — a hand-written
// spec drifting silently out of sync with the Go JSON shapes is exactly what
// this file exists to catch.
func TestSpecDocumentsLogSchemas(t *testing.T) {
	var doc map[string]any
	if err := yaml.Unmarshal(specpkg.Spec, &doc); err != nil {
		t.Fatalf("parse openapi.yaml: %v", err)
	}

	schemas, ok := lookup[map[string]any](doc, "components", "schemas")
	if !ok {
		t.Fatal("components.schemas missing")
	}

	logEntry, ok := lookup[map[string]any](schemas, "LogEntry")
	if !ok {
		t.Fatal("components.schemas.LogEntry missing")
	}
	wantRequired := []string{"seq", "time", "level", "text"}
	gotRequired, _ := lookup[[]any](logEntry, "required")
	for _, want := range wantRequired {
		if !containsAny(gotRequired, want) {
			t.Errorf("LogEntry.required = %v, missing %q", gotRequired, want)
		}
	}

	if _, ok := lookup[map[string]any](schemas, "ScanSummary"); !ok {
		t.Error("components.schemas.ScanSummary missing")
	}

	scanResponse, ok := lookup[map[string]any](schemas, "ScanResponse")
	if !ok {
		t.Fatal("components.schemas.ScanResponse missing")
	}
	for _, want := range []string{"items", "log", "summary"} {
		gotRequired, _ := lookup[[]any](scanResponse, "required")
		if !containsAny(gotRequired, want) {
			t.Errorf("ScanResponse.required = %v, missing %q", gotRequired, want)
		}
	}

	scanRef, ok := lookup[string](doc, "paths", "/api/scan", "get", "responses", "200",
		"content", "application/json", "schema", "$ref")
	if !ok || scanRef != "#/components/schemas/ScanResponse" {
		t.Errorf("GET /api/scan 200 schema $ref = %q, want %q", scanRef, "#/components/schemas/ScanResponse")
	}

	status, ok := lookup[map[string]any](schemas, "Status")
	if !ok {
		t.Fatal("components.schemas.Status missing")
	}
	statusRequired, _ := lookup[[]any](status, "required")
	for _, want := range []string{"log", "log_next"} {
		if !containsAny(statusRequired, want) {
			t.Errorf("Status.required = %v, missing %q", statusRequired, want)
		}
	}

	params, ok := lookup[[]any](doc, "paths", "/api/status/{job_id}", "get", "parameters")
	if !ok {
		t.Fatal("GET /api/status/{job_id} parameters missing")
	}
	found := false
	for _, p := range params {
		if m, ok := p.(map[string]any); ok && m["name"] == "log_since" {
			found = true
		}
	}
	if !found {
		t.Error("GET /api/status/{job_id} does not document the log_since query parameter")
	}
}

// lookup walks a chain of nested map keys and type-asserts the result to T.
func lookup[T any](doc map[string]any, keys ...string) (T, bool) {
	var cur any = doc
	for _, k := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			var zero T
			return zero, false
		}
		cur, ok = m[k]
		if !ok {
			var zero T
			return zero, false
		}
	}
	v, ok := cur.(T)
	return v, ok
}

func containsAny(xs []any, want string) bool {
	for _, x := range xs {
		if s, ok := x.(string); ok && s == want {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestHandleOpenAPISpec(t *testing.T) {
	s, _, _ := testServer(t, nil)

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, specpkg.SpecURL, nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/yaml" {
		t.Errorf("Content-Type = %q, want %q", got, "application/yaml")
	}

	var doc spec
	if err := yaml.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatalf("served spec is not parseable YAML: %v", err)
	}
	if !strings.HasPrefix(doc.OpenAPI, "3.") {
		t.Errorf("openapi = %q, want a 3.x document", doc.OpenAPI)
	}
	if len(doc.Paths) == 0 {
		t.Error("served spec documents no paths")
	}
}

func TestHandleDocs(t *testing.T) {
	s, _, _ := testServer(t, nil)

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/docs", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", got)
	}

	body := rr.Body.String()
	for _, want := range []string{specpkg.SpecURL, specpkg.BundleURL, `id="api-reference"`} {
		if !strings.Contains(body, want) {
			t.Errorf("docs page does not reference %q", want)
		}
	}
}

func TestHandleScalarBundle(t *testing.T) {
	s, _, _ := testServer(t, nil)

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, specpkg.BundleURL, nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/javascript") {
		t.Errorf("Content-Type = %q, want text/javascript", got)
	}
	if rr.Body.Len() == 0 {
		t.Error("scalar bundle is empty")
	}
}

// TestDocsRoutesNotShadowedBySPA guards the interaction with static.go: the
// docs routes live under /api/, where an unmatched path 404s rather than
// falling back to index.html. A near-miss must 404, not render the SPA.
func TestDocsRoutesNotShadowedBySPA(t *testing.T) {
	s, _, _ := testServer(t, nil)

	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/docs/nope.js", nil))

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}
