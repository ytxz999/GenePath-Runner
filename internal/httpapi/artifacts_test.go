package httpapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// executedRun submits one run through the public API and waits for the
// synchronous execution to finish, so artifacts exist for the assertions below.
func createdRun(t *testing.T, f *apiFixture, genes string) string {
	t.Helper()
	rec := f.do(t, http.MethodPost, "/api/v1/runs", map[string]any{
		"genes":        genes,
		"organism":     "hsapiens",
		"sources":      []string{"GO:BP"},
		"significance": "0.05",
		"correction":   "g_SCS",
		"background":   "annotated",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", rec.Code, rec.Body.String())
	}
	body := decode[map[string]any](t, rec)
	id, _ := body["id"].(string)
	if id == "" {
		t.Fatalf("create response has no id: %s", rec.Body.String())
	}
	if err := f.svc.ExecuteRun(context.Background(), id); err != nil {
		t.Fatalf("ExecuteRun: %v", err)
	}
	return id
}

func TestArtifactListReturnsHashedFiles(t *testing.T) {
	f := newFixture(t, true)
	id := createdRun(t, f, "TP53, BAX, MCM2, PUMA")

	rec := f.do(t, http.MethodGet, "/api/v1/runs/"+id+"/artifacts", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	body := decode[struct {
		Items []struct {
			Kind   string `json:"kind"`
			Name   string `json:"name"`
			Path   string `json:"path"`
			Bytes  int64  `json:"bytes"`
			SHA256 string `json:"sha256"`
			Href   string `json:"href"`
		} `json:"items"`
	}](t, rec)

	if len(body.Items) != 5 {
		t.Fatalf("expected 5 artifacts, got %d", len(body.Items))
	}
	for _, item := range body.Items {
		if len(item.SHA256) != 64 {
			t.Fatalf("%s sha256 = %q", item.Kind, item.SHA256)
		}
		if item.Bytes <= 0 || !strings.HasSuffix(item.Href, "/artifacts/"+item.Kind) {
			t.Fatalf("bad artifact: %+v", item)
		}
		if strings.Contains(item.Path, "\\") {
			t.Fatalf("artifact path must be stored as a slash path: %q", item.Path)
		}
	}
}

func TestHTMLReportIsServedInlineWithATightCSP(t *testing.T) {
	f := newFixture(t, true)
	id := createdRun(t, f, "TP53, BAX")

	rec := f.do(t, http.MethodGet, "/api/v1/runs/"+id+"/report.html", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("content type = %q", ct)
	}
	if cp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(cp, "default-src 'none'") {
		t.Fatalf("report CSP = %q", cp)
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Disposition"), "inline") {
		t.Fatalf("disposition = %q", rec.Header().Get("Content-Disposition"))
	}
	if !strings.Contains(rec.Body.String(), "apoptotic process") {
		t.Fatal("report body is missing the stored rows")
	}
}

func TestManifestEndpointIsDownloadableJSON(t *testing.T) {
	f := newFixture(t, true)
	id := createdRun(t, f, "TP53, BAX")

	rec := f.do(t, http.MethodGet, "/api/v1/runs/"+id+"/manifest.json", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "json") {
		t.Fatalf("content type = %q", ct)
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "manifest.json") {
		t.Fatalf("disposition = %q", rec.Header().Get("Content-Disposition"))
	}
	man := decode[struct {
		SchemaVersion string   `json:"schema_version"`
		RunID         string   `json:"run_id"`
		InputHash     string   `json:"input_hash"`
		HashAlgorithm string   `json:"hash_algorithm"`
		DataVersion   string   `json:"data_version"`
		InputGenes    []string `json:"input_genes"`
	}](t, rec)
	if man.RunID != id || len(man.InputGenes) != 2 {
		t.Fatalf("manifest = %+v", man)
	}
	if man.SchemaVersion == "" || man.HashAlgorithm != "sha256" || man.DataVersion == "" {
		t.Fatalf("manifest header = %+v", man)
	}
}

func TestGenericArtifactDownloadAndUnknownKind(t *testing.T) {
	f := newFixture(t, true)
	id := createdRun(t, f, "TP53")

	rec := f.do(t, http.MethodGet, "/api/v1/runs/"+id+"/artifacts/report_md", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "markdown") {
		t.Fatalf("content type = %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "g:Profiler") {
		t.Fatal("markdown report is missing its provenance section")
	}

	bad := f.do(t, http.MethodGet, "/api/v1/runs/"+id+"/artifacts/secrets", nil)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("unknown kind status = %d", bad.Code)
	}
	problem := decode[map[string]any](t, bad)
	if problem["code"] != "unknown_artifact" {
		t.Fatalf("problem = %v", problem)
	}
}

func TestRerunEndpointCreatesASecondRun(t *testing.T) {
	f := newFixture(t, true)
	id := createdRun(t, f, "TP53, BAX")

	rec := f.do(t, http.MethodPost, "/api/v1/runs/"+id+"/rerun", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	body := decode[map[string]any](t, rec)
	rerunID, _ := body["id"].(string)
	if rerunID == "" || rerunID == id {
		t.Fatalf("rerun id = %q", rerunID)
	}
	if loc := rec.Header().Get("Location"); loc != "/api/v1/runs/"+rerunID {
		t.Fatalf("location = %q", loc)
	}
}

func TestArtifactsForUnknownRunReturn404(t *testing.T) {
	f := newFixture(t, true)
	for _, path := range []string{
		"/api/v1/runs/run_missing/artifacts",
		"/api/v1/runs/run_missing/report.html",
		"/api/v1/runs/run_missing/manifest.json",
	} {
		rec := f.do(t, http.MethodGet, path, nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d", path, rec.Code)
		}
	}
}
