package gprofiler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// realShapedPayload is a trimmed copy of a genuine g:GOSt response for
// organism=hsapiens, query=["TP53","MCM2"], sources=["GO:BP"] retrieved while
// building this client. Field names, nesting and value shapes are unchanged.
const realShapedPayload = `{
  "meta": {
    "genes_metadata": {
      "failed": ["PUMA"],
      "duplicates": ["TP53"],
      "ambiguous": {},
      "query": {
        "query_1": {
          "mapping": {
            "TP53": "ENSG00000141510",
            "MCM2": ["ENSG00000126698", "ENSG00000250092"]
          },
          "ensgs": ["ENSG00000141510", "ENSG00000126698", "ENSG00000250092"]
        }
      }
    },
    "result_metadata": {
      "GO:BP": {"domain_size": 17674, "number_of_terms": 7481, "threshold": 0.05}
    },
    "timestamp": "2026-10-30T11:18:53.746845+00:00",
    "version": "e114_eg62_p19_9dc92ac8"
  },
  "result": [
    {
      "description": "response to UV",
      "effective_domain_size": 17674,
      "intersection_size": 2,
      "intersections": [["TP53"], ["MCM2"]],
      "name": "response to UV",
      "native": "GO:0009411",
      "p_value": 3.4142e-08,
      "parents": ["GO:0009314"],
      "precision": 0.4,
      "query": "query_1",
      "query_size": 3,
      "recall": 0.09,
      "significant": true,
      "source": "GO:BP",
      "term_size": 341,
      "group_id": 0
    }
  ]
}`

func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL), &calls
}

func TestProfileSendsDocumentedWireFormat(t *testing.T) {
	var body map[string]any
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type = %q", ct)
		}
		if ua := r.Header.Get("User-Agent"); !strings.HasPrefix(ua, "GenePathRunner/") {
			t.Errorf("user-agent = %q", ua)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(realShapedPayload))
	})

	resp, err := client.Profile(context.Background(), Request{
		Organism:      "hsapiens",
		Query:         []string{"TP53", "MCM2"},
		Sources:       []string{"GO:BP"},
		UserThreshold: 0.05,
		Correction:    "g_SCS",
	})
	if err != nil {
		t.Fatalf("Profile: %v", err)
	}

	// The three parameters whose names are easy to guess wrong.
	if got := body["significance_threshold_method"]; got != "g_SCS" {
		t.Errorf("significance_threshold_method = %v, want g_SCS", got)
	}
	if got, ok := body["user_threshold"].(float64); !ok || got != 0.05 {
		t.Errorf("user_threshold = %#v, want the number 0.05", body["user_threshold"])
	}
	if got := body["domain_scope"]; got != "annotated" {
		t.Errorf("domain_scope = %v, want annotated", got)
	}
	if got := body["organism"]; got != "hsapiens" {
		t.Errorf("organism = %v", got)
	}
	if _, exists := body["correction_method"]; exists {
		t.Error("old, undocumented correction_method key must not be sent")
	}

	if resp.HTTPStatus != http.StatusOK {
		t.Errorf("HTTPStatus = %d", resp.HTTPStatus)
	}
	if resp.Meta.Version != "e114_eg62_p19_9dc92ac8" {
		t.Errorf("data version not parsed: %q", resp.Meta.Version)
	}
	if len(resp.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(resp.Rows))
	}
	row := resp.Rows[0]
	if row.Native != "GO:0009411" || row.Name != "response to UV" || row.Source != "GO:BP" || row.PValue != 3.4142e-08 {
		t.Errorf("row not decoded faithfully: %+v", row)
	}
	if !row.Significant || row.IntersectionSize != 2 {
		t.Errorf("row flags not decoded: %+v", row)
	}
	if len(resp.RequestRaw) == 0 || len(resp.ResponseRaw) == 0 {
		t.Error("raw request and response must be kept for auditing")
	}
	if resp.Duration <= 0 {
		t.Error("duration should be measured")
	}

	// Mapping report: one single-id symbol, one symbol that maps to two genes.
	genes := resp.Meta.QueryGenesFor("query_1")
	if got := genes.Mapping["TP53"]; len(got) != 1 || got[0] != "ENSG00000141510" {
		t.Errorf("TP53 mapping = %v", got)
	}
	if got := genes.Mapping["MCM2"]; len(got) != 2 {
		t.Errorf("MCM2 mapping must accept an array, got %v", got)
	}
	if got := resp.Meta.GenesMetadata.Failed; len(got) != 1 || got[0] != "PUMA" {
		t.Errorf("unmapped genes lost: %v", got)
	}

	// Hit genes are resolved through intersections + ensgs ordering.
	hit := HitGenes(row, genes)
	if len(hit) != 2 || hit[0] != "TP53" || hit[1] != "MCM2" {
		t.Errorf("HitGenes = %v, want [TP53 MCM2]", hit)
	}
}

func TestProfileMapsProviderErrorBody(t *testing.T) {
	client, calls := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"Invalid organism: notanorganism"}`))
	})

	_, err := client.Profile(context.Background(), Request{Organism: "notanorganism", Query: []string{"TP53"}, Sources: []string{"GO:BP"}})
	var pe *ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ProviderError, got %T (%v)", err, err)
	}
	if pe.Kind != KindRejected || pe.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected error classification: %+v", pe)
	}
	if !strings.Contains(pe.Message, "Invalid organism") {
		t.Fatalf("provider message lost: %q", pe.Message)
	}
	if n := atomic.LoadInt32(calls); n != 1 {
		t.Fatalf("a 4xx must not be retried, saw %d calls", n)
	}
}

func TestProfileRetriesOnceOnProviderSideFailure(t *testing.T) {
	client, calls := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"message":"upstream busy"}`))
	})

	_, err := client.Profile(context.Background(), Request{Organism: "hsapiens", Query: []string{"TP53"}, Sources: []string{"GO:BP"}})
	var pe *ProviderError
	if !errors.As(err, &pe) || pe.Kind != KindUnavailable {
		t.Fatalf("expected unavailable error, got %v", err)
	}
	if n := atomic.LoadInt32(calls); n != 2 {
		t.Fatalf("expected exactly one retry, saw %d calls", n)
	}
}

func TestProfileTimeoutIsClassified(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(400 * time.Millisecond)
		_, _ = w.Write([]byte(`{}`))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	_, err := client.Profile(ctx, Request{Organism: "hsapiens", Query: []string{"TP53"}, Sources: []string{"GO:BP"}})
	var pe *ProviderError
	if !errors.As(err, &pe) || pe.Kind != KindTimeout {
		t.Fatalf("expected timeout classification, got %v", err)
	}
}

func TestProfileInvalidJSONIsClassified(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>gateway says hi</html>`))
	})

	_, err := client.Profile(context.Background(), Request{Organism: "hsapiens", Query: []string{"TP53"}, Sources: []string{"GO:BP"}})
	var pe *ProviderError
	if !errors.As(err, &pe) || pe.Kind != KindInvalidJSON {
		t.Fatalf("expected invalid_json classification, got %v", err)
	}
}

func TestProfileRejectsEmptyQueryWithoutCallingUpstream(t *testing.T) {
	client, calls := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream must not be called for an empty query")
	})

	_, err := client.Profile(context.Background(), Request{Organism: "hsapiens", Sources: []string{"GO:BP"}})
	var pe *ProviderError
	if !errors.As(err, &pe) || pe.Kind != KindRejected {
		t.Fatalf("expected rejection, got %v", err)
	}
	if atomic.LoadInt32(calls) != 0 {
		t.Fatal("unexpected upstream call")
	}
}

func TestMappedIDsAcceptsBothShapes(t *testing.T) {
	cases := map[string]int{
		`"ENSG00000141510"`:                     1,
		`["ENSG00000126698","ENSG00000250092"]`: 2,
		`[]`:                                    0,
	}
	for raw, want := range cases {
		var ids MappedIDs
		if err := json.Unmarshal([]byte(raw), &ids); err != nil {
			t.Fatalf("unmarshal %s: %v", raw, err)
		}
		if len(ids) != want {
			t.Fatalf("unmarshal %s gave %d ids, want %d", raw, len(ids), want)
		}
	}
}

func TestHitGenesFallsBackToIdentifier(t *testing.T) {
	row := Row{
		IntersectionSize: 2,
		Intersections:    [][]string{{"TP53"}, {"MCM2"}},
	}
	genes := QueryGenes{
		Mapping: map[string]MappedIDs{"TP53": {"ENSG1"}},
		Ensgs:   []string{"ENSG1", "ENSG-unknown"},
	}
	got := HitGenes(row, genes)
	if len(got) != 2 || got[0] != "TP53" || got[1] != "ENSG-unknown" {
		t.Fatalf("HitGenes = %v", got)
	}

	if HitGenes(row, QueryGenes{}) != nil {
		t.Fatal("empty mapping must yield no hit genes")
	}
}
