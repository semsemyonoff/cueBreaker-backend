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
