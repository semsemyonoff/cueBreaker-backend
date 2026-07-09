package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStaticHandler_ServesIndexAsset(t *testing.T) {
	h, err := newStaticHandler()
	if err != nil {
		t.Fatalf("newStaticHandler: %v", err)
	}

	// "/" resolves directly to an existing file (index.html) via httpFS.Open,
	// so this exercises the "asset found" branch rather than the fallback
	// rewrite exercised by TestStaticHandler_UnknownPathFallsBackToIndex.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "cueBreaker") {
		t.Errorf("body = %q, want it to contain %q", rec.Body.String(), "cueBreaker")
	}
}

func TestStaticHandler_UnknownPathFallsBackToIndex(t *testing.T) {
	h, err := newStaticHandler()
	if err != nil {
		t.Fatalf("newStaticHandler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/library/some/album", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "cueBreaker") {
		t.Errorf("body = %q, want the SPA fallback (index.html)", rec.Body.String())
	}
}

func TestStaticHandler_APIPathNeverFallsBack(t *testing.T) {
	h, err := newStaticHandler()
	if err != nil {
		t.Fatalf("newStaticHandler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/does-not-exist", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "cueBreaker") {
		t.Errorf("body = %q, want plain 404, not the SPA fallback", rec.Body.String())
	}
}

func TestServer_UnknownAPIRouteReturns404NotSPA(t *testing.T) {
	s, _, _ := testServer(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/does-not-exist", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestServer_RootServesSPA(t *testing.T) {
	s, _, _ := testServer(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "cueBreaker") {
		t.Errorf("body = %q, want it to contain %q", rec.Body.String(), "cueBreaker")
	}
}
