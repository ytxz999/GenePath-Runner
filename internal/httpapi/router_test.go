package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"genepath/internal/database"
	"genepath/internal/httpapi"
	"genepath/internal/provider/gprofiler"
	"genepath/internal/repository"
	"genepath/internal/service"
)

const fakeProviderBody = `{
  "meta": {
    "genes_metadata": {
      "failed": [],
      "duplicates": [],
      "ambiguous": {},
      "query": {"query_1": {"mapping": {"TP53": "ENSG1"}, "ensgs": ["ENSG1"]}}
    },
    "result_metadata": {"GO:BP": {"domain_size": 17674, "number_of_terms": 7481, "threshold": 0.05}},
    "timestamp": "2026-10-30T11:18:53.746845+00:00",
    "version": "e114_eg62_p19_9dc92ac8"
  },
  "result": [{
    "description": "apoptotic process", "effective_domain_size": 17674,
    "intersection_size": 1, "intersections": [["TP53"]], "name": "apoptotic process",
    "native": "GO:0006915", "p_value": 1.2e-09, "parents": [],
    "precision": 1, "query": "query_1", "query_size": 1, "recall": 1,
    "significant": true, "source": "GO:BP", "term_size": 500, "group_id": 0
  }]
}`

type apiFixture struct {
	router   http.Handler
	svc      *service.RunService
	artifact string
	upstream string
}

func newFixture(t *testing.T, attachExecutor bool) *apiFixture {
	t.Helper()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fakeProviderBody))
	}))
	t.Cleanup(upstream.Close)

	db, err := database.Open(filepath.Join(t.TempDir(), "api.db"), false)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })

	repo := repository.NewRunRepository(db)
	artifactDir := filepath.Join(t.TempDir(), "artifacts")
	svc := service.NewRunService(repo, service.Options{
		ProviderEndpoint: upstream.URL,
		ProviderReady:    true,
		Client:           gprofiler.New(upstream.URL),
		ExecTimeout:      5 * time.Second,
		ArtifactDir:      artifactDir,
	})
	if attachExecutor {
		svc.AttachQueue(func(string) error { return nil })
	}

	pages := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<!doctype html><title>GenePath</title>")},
	}
	api := httpapi.NewAPI(db, svc, repo, "0.2.0", upstream.URL, time.Now())
	router := httpapi.NewRouter(api, httpapi.RouterOptions{
		Service:   svc,
		Repo:      repo,
		Version:   "0.2.0",
		PageFiles: pages,
	})
	return &apiFixture{router: router, svc: svc, artifact: artifactDir, upstream: upstream.URL}
}

func (f *apiFixture) do(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return out
}

func TestHealthzReportsDatabase(t *testing.T) {
	f := newFixture(t, true)
	rec := f.do(t, http.MethodGet, "/healthz", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := decode[map[string]any](t, rec)
	if body["status"] != "ok" || body["database"] != "ok" {
		t.Fatalf("unexpected health body: %v", body)
	}
	if body["version"] != "0.2.0" {
		t.Fatalf("version = %v", body["version"])
	}
}

func TestReadyzFailsWithoutExecutor(t *testing.T) {
	f := newFixture(t, false)
	rec := f.do(t, http.MethodGet, "/readyz", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestEmptyDashboardIsWellFormed(t *testing.T) {
	f := newFixture(t, true)
	rec := f.do(t, http.MethodGet, "/api/v1/dashboard", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	body := decode[map[string]any](t, rec)
	stats, ok := body["stats"].(map[string]any)
	if !ok {
		t.Fatalf("missing stats: %v", body)
	}
	if stats["total"].(float64) != 0 {
		t.Fatalf("expected an empty dashboard, got %v", stats)
	}
	if runs, ok := body["recentRuns"].([]any); !ok || len(runs) != 0 {
		t.Fatalf("recentRuns should be an empty array, got %v", body["recentRuns"])
	}
	provider, _ := body["provider"].(map[string]any)
	if provider["ready"] != true {
		t.Fatalf("provider.ready = %v", provider["ready"])
	}
}

func TestMetaDescribesCatalogue(t *testing.T) {
	f := newFixture(t, true)
	rec := f.do(t, http.MethodGet, "/api/v1/meta", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := decode[map[string]any](t, rec)
	organisms, ok := body["organisms"].([]any)
	if !ok || len(organisms) < 5 {
		t.Fatalf("organisms missing: %v", body["organisms"])
	}
	sources, ok := body["sources"].([]any)
	if !ok || len(sources) < 5 {
		t.Fatalf("sources missing: %v", body["sources"])
	}
}

func TestCreateRunAcceptsQuotedAndBareThreshold(t *testing.T) {
	cases := []struct {
		name string
		body map[string]any
	}{
		{name: "quoted", body: map[string]any{"genes": "TP53, BAX", "organism": "hsapiens", "sources": []string{"GO:BP"}, "significance": "0.01"}},
		{name: "number", body: map[string]any{"genes": "TP53, BAX", "organism": "hsapiens", "sources": []string{"GO:BP"}, "significance": 0.01}},
		{name: "omitted", body: map[string]any{"genes": "TP53, BAX", "organism": "hsapiens", "sources": []string{"GO:BP"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, true)
			rec := f.do(t, http.MethodPost, "/api/v1/runs", tc.body)
			if rec.Code != http.StatusCreated {
				t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
			}
			run := decode[struct {
				Params struct {
					Significance string `json:"significance"`
				} `json:"params"`
			}](t, rec)
			want := "0.05"
			if tc.name != "omitted" {
				want = "0.01"
			}
			if run.Params.Significance != want {
				t.Fatalf("significance = %q, want %q", run.Params.Significance, want)
			}
		})
	}
}

func TestCreateRunRejectsMalformedJSON(t *testing.T) {
	f := newFixture(t, true)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runs", strings.NewReader("{not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	body := decode[map[string]any](t, rec)
	if body["code"] != "bad_request" {
		t.Fatalf("code = %v", body["code"])
	}
}

func TestCreateRunReturnsFieldLevelValidation(t *testing.T) {
	f := newFixture(t, true)
	rec := f.do(t, http.MethodPost, "/api/v1/runs", map[string]any{
		"genes": "TP53, BAX!", "organism": "hsapiens", "sources": []string{"GO:BP"},
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422: %s", rec.Code, rec.Body.String())
	}
	body := decode[map[string]any](t, rec)
	if body["code"] != "validation_failed" {
		t.Fatalf("code = %v", body["code"])
	}
	details, ok := body["details"].(map[string]any)
	if !ok || details["genes"] == nil {
		t.Fatalf("expected per-field details, got %v", body["details"])
	}
}

func TestCreateRunAnswers503WhenExecutorIsMissing(t *testing.T) {
	f := newFixture(t, false)
	rec := f.do(t, http.MethodPost, "/api/v1/runs", map[string]any{
		"genes": "TP53", "organism": "hsapiens", "sources": []string{"GO:BP"},
	})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	body := decode[map[string]any](t, rec)
	if body["code"] != "executor_not_ready" {
		t.Fatalf("code = %v", body["code"])
	}
}

func TestRunLifecycleOverHTTP(t *testing.T) {
	f := newFixture(t, true)

	rec := f.do(t, http.MethodPost, "/api/v1/runs", map[string]any{
		"label":    "apoptosis over HTTP",
		"genes":    "TP53",
		"organism": "hsapiens",
		"sources":  []string{"GO:BP"},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	created := decode[map[string]any](t, rec)
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatalf("no run id in %v", created)
	}
	if created["status"] != "queued" {
		t.Fatalf("new run must be queued, got %v", created["status"])
	}
	hash, _ := created["inputHash"].(string)
	if len(hash) != 64 {
		t.Fatalf("inputHash = %q", hash)
	}

	if err := f.svc.ExecuteRun(t.Context(), id); err != nil {
		t.Fatalf("ExecuteRun: %v", err)
	}

	rec = f.do(t, http.MethodGet, "/api/v1/runs/"+id, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d", rec.Code)
	}
	detail := decode[map[string]any](t, rec)
	if detail["status"] != "succeeded" || detail["outcome"] != "enriched" {
		t.Fatalf("status/outcome = %v/%v", detail["status"], detail["outcome"])
	}

	rec = f.do(t, http.MethodGet, "/api/v1/runs/"+id+"/results", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("results status = %d", rec.Code)
	}
	results := decode[map[string]any](t, rec)
	terms, ok := results["terms"].([]any)
	if !ok || len(terms) != 1 {
		t.Fatalf("terms = %v", results["terms"])
	}
	term := terms[0].(map[string]any)
	if term["termId"] != "GO:0006915" || term["name"] != "apoptotic process" {
		t.Fatalf("term identity swapped: %v", term)
	}

	rec = f.do(t, http.MethodGet, "/api/v1/runs/"+id+"/report.md", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("report status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/markdown") {
		t.Fatalf("report content-type = %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "GO:0006915") {
		t.Fatal("report body is missing the term")
	}

	rec = f.do(t, http.MethodGet, "/api/v1/runs/"+id+"/raw", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("raw status = %d", rec.Code)
	}
	raw := decode[map[string]any](t, rec)
	if !strings.Contains(raw["response"].(string), "e114_eg62_p19_9dc92ac8") {
		t.Fatal("raw response is not the stored upstream body")
	}
}

func TestListRunsFiltersAndPaginates(t *testing.T) {
	f := newFixture(t, true)
	for i := 0; i < 3; i++ {
		rec := f.do(t, http.MethodPost, "/api/v1/runs", map[string]any{
			"genes": "TP53", "organism": "hsapiens", "sources": []string{"GO:BP"},
		})
		if rec.Code != http.StatusCreated {
			t.Fatalf("create %d: %d %s", i, rec.Code, rec.Body.String())
		}
	}

	rec := f.do(t, http.MethodGet, "/api/v1/runs?limit=2&offset=0", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	page := decode[map[string]any](t, rec)
	if page["total"].(float64) != 3 {
		t.Fatalf("total = %v", page["total"])
	}
	if items := page["items"].([]any); len(items) != 2 {
		t.Fatalf("limit ignored: %d items", len(items))
	}

	rec = f.do(t, http.MethodGet, "/api/v1/runs?status=succeeded", nil)
	page = decode[map[string]any](t, rec)
	if len(page["items"].([]any)) != 0 {
		t.Fatal("queued runs must not match ?status=succeeded")
	}
}

func TestUnknownRunAndUnknownRouteAreJSON(t *testing.T) {
	f := newFixture(t, true)

	rec := f.do(t, http.MethodGet, "/api/v1/runs/run_missing", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if body := decode[map[string]any](t, rec); body["code"] != "not_found" {
		t.Fatalf("code = %v", body["code"])
	}

	rec = f.do(t, http.MethodGet, "/api/v1/nope", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown route status = %d", rec.Code)
	}
	if body := decode[map[string]any](t, rec); body["code"] != "not_found" {
		t.Fatalf("unknown route must answer JSON, got %s", rec.Body.String())
	}
}

func TestSecurityHeadersAreSet(t *testing.T) {
	f := newFixture(t, true)
	rec := f.do(t, http.MethodGet, "/api/v1/meta", nil)
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src") {
		t.Fatalf("CSP missing script-src: %q", csp)
	}
}

func TestPagesAreServed(t *testing.T) {
	f := newFixture(t, true)
	rec := f.do(t, http.MethodGet, "/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "GenePath") {
		t.Fatalf("unexpected index body: %s", rec.Body.String())
	}
}
