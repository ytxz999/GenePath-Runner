package service_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"genepath/internal/database"
	"genepath/internal/model"
	"genepath/internal/provider/gprofiler"
	"genepath/internal/repository"
	"genepath/internal/service"
)

// fakeProvider stands in for g:Profiler with a payload whose shape matches the
// live API (meta.genes_metadata.query + intersections indexed by ensgs).
const fakeProviderBody = `{
  "meta": {
    "genes_metadata": {
      "failed": ["PUMA"],
      "duplicates": [],
      "ambiguous": {},
      "query": {
        "query_1": {
          "mapping": {"TP53": "ENSG1", "BAX": "ENSG2", "MCM2": "ENSG3"},
          "ensgs": ["ENSG1", "ENSG2", "ENSG3"]
        }
      }
    },
    "result_metadata": {"GO:BP": {"domain_size": 17674, "number_of_terms": 7481, "threshold": 0.05}},
    "timestamp": "2026-10-30T11:18:53.746845+00:00",
    "version": "e114_eg62_p19_9dc92ac8"
  },
  "result": [
    {
      "description": "apoptotic process", "effective_domain_size": 17674,
      "intersection_size": 2, "intersections": [["TP53"], ["BAX"], []],
      "name": "apoptotic process", "native": "GO:0006915", "p_value": 1.2e-09,
      "parents": [], "precision": 0.6, "query": "query_1", "query_size": 3,
      "recall": 0.1, "significant": true, "source": "GO:BP", "term_size": 500,
      "group_id": 0
    },
    {
      "description": "DNA replication", "effective_domain_size": 17674,
      "intersection_size": 1, "intersections": [[], [], ["MCM2"]],
      "name": "DNA replication", "native": "GO:0006260", "p_value": 4.4e-05,
      "parents": [], "precision": 0.2, "query": "query_1", "query_size": 3,
      "recall": 0.02, "significant": true, "source": "GO:BP", "term_size": 300,
      "group_id": 0
    }
  ]
}`

type harness struct {
	svc    *service.RunService
	repo   *repository.RunRepository
	client *gprofiler.Client
}

func newHarness(t *testing.T, handler http.HandlerFunc) *harness {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"), false)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })

	repo := repository.NewRunRepository(db)
	client := gprofiler.New(srv.URL)
	svc := service.NewRunService(repo, service.Options{
		ProviderEndpoint: srv.URL,
		ProviderReady:    true,
		Client:           client,
		ExecTimeout:      5 * time.Second,
	})
	return &harness{svc: svc, repo: repo, client: client}
}

func okHandler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fakeProviderBody))
	}
}

func (h *harness) submit(t *testing.T, genes string) string {
	t.Helper()
	detail, err := h.svc.Create(context.Background(), service.CreateInput{
		Label:        "apoptosis panel",
		Genes:        genes,
		Organism:     "hsapiens",
		Sources:      []string{"GO:BP"},
		Significance: "0.05",
		Correction:   "g_SCS",
		Background:   "annotated",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return detail.ID
}

func TestCreateRequiresAnExecutor(t *testing.T) {
	h := newHarness(t, okHandler(t))
	_, err := h.svc.Create(context.Background(), service.CreateInput{
		Genes: "TP53", Organism: "hsapiens", Sources: []string{"GO:BP"},
	})
	if !errors.Is(err, service.ErrNotReady) {
		t.Fatalf("expected ErrNotReady before AttachQueue, got %v", err)
	}
	var verr *service.ValidationError
	if errors.As(err, &verr) {
		t.Fatal("a missing executor is not a validation error")
	}
}

func TestEndToEndSuccessStoresRealTermsAndAudit(t *testing.T) {
	h := newHarness(t, okHandler(t))
	h.svc.AttachQueue(func(string) error { return nil })

	id := h.submit(t, "TP53, BAX, MCM2, PUMA")

	if err := h.svc.ExecuteRun(context.Background(), id); err != nil {
		t.Fatalf("ExecuteRun: %v", err)
	}

	detail, err := h.svc.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if detail.Status != model.StatusSucceeded {
		t.Fatalf("status = %s (%s)", detail.Status, detail.ErrorMessage)
	}
	if detail.Outcome != model.OutcomeEnriched {
		t.Fatalf("outcome = %s, want enriched", detail.Outcome)
	}
	if detail.DataVersion != "e114_eg62_p19_9dc92ac8" {
		t.Fatalf("data version not persisted: %q", detail.DataVersion)
	}
	if detail.TermCount != 2 || detail.ResultCount != 2 {
		t.Fatalf("term counts = %d/%d, want 2/2", detail.TermCount, detail.ResultCount)
	}
	if detail.GeneCount != 4 || detail.MappedCount != 3 {
		t.Fatalf("gene counts = %d/%d, want 4/3", detail.GeneCount, detail.MappedCount)
	}
	if len(detail.Unmapped) != 1 || detail.Unmapped[0] != "PUMA" {
		t.Fatalf("unmapped genes = %v", detail.Unmapped)
	}
	if detail.HTTPStatus != http.StatusOK || detail.ResponseBytes == 0 {
		t.Fatalf("raw exchange not recorded: http=%d bytes=%d", detail.HTTPStatus, detail.ResponseBytes)
	}
	if detail.DurationMS < 0 || detail.ProviderMS < 0 {
		t.Fatalf("timings not recorded: %+v", detail)
	}
	if len(detail.Events) < 3 {
		t.Fatalf("expected submit/request/succeeded events, got %d", len(detail.Events))
	}

	results, err := h.svc.Results(context.Background(), id)
	if err != nil {
		t.Fatalf("Results: %v", err)
	}
	if len(results.Terms) != 2 {
		t.Fatalf("expected 2 stored terms, got %d", len(results.Terms))
	}
	first := results.Terms[0]
	if first.TermID != "GO:0006915" || first.Source != "GO:BP" {
		t.Fatalf("term identity lost: %+v", first)
	}
	if first.PValue != 1.2e-09 {
		t.Fatalf("p-value must be stored unchanged, got %v", first.PValue)
	}
	if len(first.Genes) != 2 || first.Genes[0] != "TP53" || first.Genes[1] != "BAX" {
		t.Fatalf("hit genes not resolved: %v", first.Genes)
	}
	if results.Provider.DataVersion != "e114_eg62_p19_9dc92ac8" {
		t.Fatalf("provider info missing data version: %+v", results.Provider)
	}

	raw, err := h.svc.RawExchange(context.Background(), id)
	if err != nil {
		t.Fatalf("RawExchange: %v", err)
	}
	if !strings.Contains(raw.Request, "significance_threshold_method") {
		t.Fatalf("stored request is not the upstream body: %s", raw.Request)
	}
	if !strings.Contains(raw.Response, "e114_eg62_p19_9dc92ac8") {
		t.Fatal("stored response is not the upstream body")
	}

	report, err := h.svc.Report(context.Background(), id)
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if !strings.Contains(report, "GO:0006915") || !strings.Contains(report, "e114_eg62_p19_9dc92ac8") {
		t.Fatalf("report is missing terms or provenance:\n%s", report)
	}
}

func TestProviderFailureIsRecordedWithoutLosingTheRun(t *testing.T) {
	h := newHarness(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"database unavailable"}`))
	})
	h.svc.AttachQueue(func(string) error { return nil })

	id := h.submit(t, "TP53, BAX")
	if err := h.svc.ExecuteRun(context.Background(), id); err == nil {
		t.Fatal("ExecuteRun must report the provider failure")
	}

	detail, err := h.svc.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("the failed run must still be readable: %v", err)
	}
	if detail.Status != model.StatusFailed {
		t.Fatalf("status = %s, want failed", detail.Status)
	}
	if detail.ErrorCode != model.ErrUpstreamUnavailable {
		t.Fatalf("error code = %q, want %s", detail.ErrorCode, model.ErrUpstreamUnavailable)
	}
	if !strings.Contains(detail.ErrorMessage, "database unavailable") {
		t.Fatalf("upstream message lost: %q", detail.ErrorMessage)
	}
	if detail.HTTPStatus != http.StatusInternalServerError {
		t.Fatalf("http status not persisted: %d", detail.HTTPStatus)
	}
}

func TestNoGenesMappedOutcome(t *testing.T) {
	h := newHarness(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"meta":{"genes_metadata":{"failed":["NOTAGENE"],"query":{}},"version":"e114"},"result":[]}`))
	})
	h.svc.AttachQueue(func(string) error { return nil })

	id := h.submit(t, "NOTAGENE")
	if err := h.svc.ExecuteRun(context.Background(), id); err != nil {
		t.Fatalf("ExecuteRun: %v", err)
	}
	detail, err := h.svc.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if detail.Status != model.StatusSucceeded || detail.Outcome != model.OutcomeNoGenesMapped {
		t.Fatalf("status/outcome = %s/%s, want succeeded/%s", detail.Status, detail.Outcome, model.OutcomeNoGenesMapped)
	}
	if detail.MappedCount != 0 || detail.ResultCount != 0 {
		t.Fatalf("counts should be zero: mapped=%d results=%d", detail.MappedCount, detail.ResultCount)
	}
}

func TestNoSignificantTermsOutcome(t *testing.T) {
	h := newHarness(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"meta":{"genes_metadata":{"failed":[],"query":{"query_1":{"mapping":{"MCM2":"ENSG3"},"ensgs":["ENSG3"]}}},"version":"e114"},"result":[]}`))
	})
	h.svc.AttachQueue(func(string) error { return nil })

	id := h.submit(t, "MCM2")
	if err := h.svc.ExecuteRun(context.Background(), id); err != nil {
		t.Fatalf("ExecuteRun: %v", err)
	}
	detail, _ := h.svc.Get(context.Background(), id)
	if detail.Outcome != model.OutcomeNoSignificant {
		t.Fatalf("outcome = %s, want %s", detail.Outcome, model.OutcomeNoSignificant)
	}
	if detail.MappedCount != 1 {
		t.Fatalf("mapped count = %d, want 1", detail.MappedCount)
	}
}

func TestValidationRejectsBadInput(t *testing.T) {
	h := newHarness(t, okHandler(t))
	h.svc.AttachQueue(func(string) error { return nil })

	cases := []struct {
		name  string
		input service.CreateInput
		field string
	}{
		{"empty genes", service.CreateInput{Genes: "  \n\t\n ", Organism: "hsapiens", Sources: []string{"GO:BP"}}, "genes"},
		{"illegal symbol", service.CreateInput{Genes: "TP53, BAX!", Organism: "hsapiens", Sources: []string{"GO:BP"}}, "genes"},
		{"unknown organism", service.CreateInput{Genes: "TP53", Organism: "klingon", Sources: []string{"GO:BP"}}, "organism"},
		{"unknown source", service.CreateInput{Genes: "TP53", Organism: "hsapiens", Sources: []string{"GO:XX"}}, "sources"},
		{"no source selected", service.CreateInput{Genes: "TP53", Organism: "hsapiens", Sources: []string{}}, "sources"},
		{"bad threshold", service.CreateInput{Genes: "TP53", Organism: "hsapiens", Sources: []string{"GO:BP"}, Significance: "2"}, "significance"},
		{"bad correction", service.CreateInput{Genes: "TP53", Organism: "hsapiens", Sources: []string{"GO:BP"}, Correction: "bogus"}, "correction"},
		{"bad background", service.CreateInput{Genes: "TP53", Organism: "hsapiens", Sources: []string{"GO:BP"}, Background: "universe"}, "background"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.svc.Create(context.Background(), tc.input)
			var verr *service.ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("expected a validation error, got %v", err)
			}
			if _, ok := verr.Fields[tc.field]; !ok {
				t.Fatalf("expected field %q in %v", tc.field, verr.Fields)
			}
		})
	}
}

func TestGeneLimitIsEnforced(t *testing.T) {
	h := newHarness(t, okHandler(t))
	h.svc.AttachQueue(func(string) error { return nil })

	symbols := make([]string, 0, 2100)
	for i := 0; i < 2100; i++ {
		symbols = append(symbols, "GENE"+strings.Repeat("X", 1)+itoa(i))
	}
	_, err := h.svc.Create(context.Background(), service.CreateInput{
		Genes: strings.Join(symbols, "\n"), Organism: "hsapiens", Sources: []string{"GO:BP"},
	})
	var verr *service.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a validation error for >2000 genes, got %v", err)
	}
}

func TestDuplicateSubmissionIsFlagged(t *testing.T) {
	h := newHarness(t, okHandler(t))
	h.svc.AttachQueue(func(string) error { return nil })

	first := h.submit(t, "TP53, BAX, MCM2")
	second, err := h.svc.Create(context.Background(), service.CreateInput{
		Genes: "MCM2\nBAX\nTP53", Organism: "hsapiens", Sources: []string{"GO:BP"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if second.DuplicateOf != first {
		t.Fatalf("duplicateOf = %q, want %q", second.DuplicateOf, first)
	}
}

func TestExecuteRunOnMissingRunReturnsNotFound(t *testing.T) {
	h := newHarness(t, okHandler(t))
	h.svc.AttachQueue(func(string) error { return nil })

	err := h.svc.ExecuteRun(context.Background(), "run_missing")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var digits []byte
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	return string(digits)
}
