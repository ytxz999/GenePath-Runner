package service_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"genepath/internal/database"
	"genepath/internal/dto"
	"genepath/internal/gene"
	"genepath/internal/model"
	"genepath/internal/provider/gprofiler"
	"genepath/internal/repository"
	"genepath/internal/service"
)

// artifactHarness is the service harness plus a real artifact directory, so the
// report/manifest pipeline is exercised end to end.
type artifactHarness struct {
	*harness
	dir string
}

func newArtifactHarness(t *testing.T, handler http.HandlerFunc) *artifactHarness {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	db, err := database.Open(filepath.Join(dir, "test.db"), false)
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
		ArtifactDir:      filepath.Join(dir, "artifacts"),
	})
	h := &harness{svc: svc, repo: repo, client: client}
	return &artifactHarness{harness: h, dir: filepath.Join(dir, "artifacts")}
}

// run executes one analysis synchronously and returns its id.
func (h *artifactHarness) run(t *testing.T, genes string) string {
	t.Helper()
	h.svc.AttachQueue(func(string) error { return nil })
	id := h.submit(t, genes)
	if err := h.svc.ExecuteRun(context.Background(), id); err != nil {
		t.Fatalf("ExecuteRun: %v", err)
	}
	return id
}

func TestSucceededRunWritesCompleteArtifactBundle(t *testing.T) {
	h := newArtifactHarness(t, okHandler(t))
	id := h.run(t, "TP53, BAX, MCM2, PUMA")

	items, err := h.svc.Artifacts(context.Background(), id)
	if err != nil {
		t.Fatalf("Artifacts: %v", err)
	}
	want := map[string]bool{
		model.ArtifactHTML:     false,
		model.ArtifactMarkdown: false,
		model.ArtifactManifest: false,
		model.ArtifactRequest:  false,
		model.ArtifactResponse: false,
	}
	for _, it := range items {
		if _, ok := want[it.Kind]; !ok {
			t.Fatalf("unexpected artifact kind %q", it.Kind)
		}
		want[it.Kind] = true
		if it.Bytes == 0 {
			t.Fatalf("%s artifact is empty", it.Kind)
		}
		if len(it.SHA256) != 64 {
			t.Fatalf("%s hash = %q", it.Kind, it.SHA256)
		}
		if !strings.HasPrefix(it.Href, "/api/v1/runs/"+id+"/artifacts/") {
			t.Fatalf("%s href = %q", it.Kind, it.Href)
		}
	}
	for kind, seen := range want {
		if !seen {
			t.Fatalf("missing artifact %q", kind)
		}
	}

	// The bytes on disk must hash to the value stored in the database.
	for _, it := range items {
		name := service.ArtifactFileName(it.Kind)
		raw, err := os.ReadFile(filepath.Join(h.dir, id, name))
		if err != nil {
			t.Fatalf("read %s: %v", it.Kind, err)
		}
		sum := sha256.Sum256(raw)
		if hex.EncodeToString(sum[:]) != it.SHA256 {
			t.Fatalf("%s hash mismatch", it.Kind)
		}
	}
}

func TestManifestDescribesInputsAndSnapshots(t *testing.T) {
	h := newArtifactHarness(t, okHandler(t))
	id := h.run(t, "TP53, BAX, MCM2, PUMA")

	detail, err := h.svc.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	man, err := h.svc.Manifest(context.Background(), id)
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}

	if man.RunID != id || man.InputHash != detail.InputHash {
		t.Fatalf("manifest identity mismatch: %+v", man)
	}
	if man.Organism != "hsapiens" || man.OrganismLabel == "" {
		t.Fatalf("organism = %q / %q", man.Organism, man.OrganismLabel)
	}
	if len(man.InputGenes) != 4 {
		t.Fatalf("input genes = %v", man.InputGenes)
	}
	if man.DataVersion != "e114_eg62_p19_9dc92ac8" {
		t.Fatalf("data version = %q", man.DataVersion)
	}
	if man.Parameters.Correction != "g_SCS" || len(man.Parameters.Sources) != 1 {
		t.Fatalf("parameters = %+v", man.Parameters)
	}
	if man.ResultCount != 2 {
		t.Fatalf("result count = %d", man.ResultCount)
	}
	if man.Mapping.Mapped != 3 || len(man.Mapping.Unmapped) != 1 || man.Mapping.Unmapped[0] != "PUMA" {
		t.Fatalf("mapping = %+v", man.Mapping)
	}
	if man.HTTPStatus != http.StatusOK {
		t.Fatalf("http status = %d", man.HTTPStatus)
	}

	req, ok := man.RequestSnapshot.(map[string]any)
	if !ok {
		t.Fatalf("request snapshot = %T", man.RequestSnapshot)
	}
	if req["organism"] != "hsapiens" {
		t.Fatalf("request organism = %v", req["organism"])
	}
	resp, ok := man.ResponseSnapshot.(map[string]any)
	if !ok {
		t.Fatalf("response snapshot = %T", man.ResponseSnapshot)
	}
	meta, _ := resp["meta"].(map[string]any)
	if meta["version"] != "e114_eg62_p19_9dc92ac8" {
		t.Fatalf("response meta = %v", meta)
	}

	notes := strings.Join(man.Notes, " ")
	if !strings.Contains(notes, "不含时间戳") || !strings.Contains(notes, "未重新计算") {
		t.Fatalf("notes lost their guarantees: %q", notes)
	}
}

func TestInputHashIgnoresOrderCaseAndSubmissionTime(t *testing.T) {
	h := newArtifactHarness(t, okHandler(t))

	first := h.run(t, "TP53, BAX, MCM2")
	time.Sleep(10 * time.Millisecond)
	second := h.run(t, "mcm2\nbax\ntp53\n")

	a, err := h.svc.Get(context.Background(), first)
	if err != nil {
		t.Fatalf("Get first: %v", err)
	}
	b, err := h.svc.Get(context.Background(), second)
	if err != nil {
		t.Fatalf("Get second: %v", err)
	}
	if a.InputHash != b.InputHash {
		t.Fatalf("hash changed with order/case: %s vs %s", a.InputHash, b.InputHash)
	}
	if !a.CreatedAt.Before(b.CreatedAt) {
		t.Fatalf("expected the second run to be created later")
	}
}

func TestReportHTMLIsSelfContainedAndEscapesInput(t *testing.T) {
	h := newArtifactHarness(t, okHandler(t))
	h.svc.AttachQueue(func(string) error { return nil })

	detail, err := h.svc.Create(context.Background(), service.CreateInput{
		Label:        `<script>alert("x")</script>`,
		Genes:        "TP53, BAX, MCM2, PUMA",
		Organism:     "hsapiens",
		Sources:      []string{"GO:BP"},
		Significance: "0.05",
		Correction:   "g_SCS",
		Background:   "annotated",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := h.svc.ExecuteRun(context.Background(), detail.ID); err != nil {
		t.Fatalf("ExecuteRun: %v", err)
	}

	raw, err := h.svc.ArtifactBytes(context.Background(), detail.ID, model.ArtifactHTML)
	if err != nil {
		t.Fatalf("ArtifactBytes: %v", err)
	}
	body := string(raw)

	if !strings.HasPrefix(strings.TrimSpace(body), "<!doctype html>") {
		t.Fatal("report is not a standalone HTML document")
	}
	if strings.Contains(body, `<script>alert`) {
		t.Fatal("label was not HTML-escaped")
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Fatal("expected the escaped label to be present")
	}
	if !strings.Contains(body, "apoptotic process") || !strings.Contains(body, "1.20e-09") {
		t.Fatal("report is missing the stored enrichment rows")
	}
	if strings.Contains(body, `<link`) || strings.Contains(body, `src="http`) {
		t.Fatal("report must not depend on external assets")
	}
	if !strings.Contains(body, "科学局限") || !strings.Contains(body, "g:Profiler") {
		t.Fatal("report is missing the limitations or the citation")
	}
}

func TestArtifactBytesRegeneratesADeletedFile(t *testing.T) {
	h := newArtifactHarness(t, okHandler(t))
	id := h.run(t, "TP53, BAX, MCM2")

	target := filepath.Join(h.dir, id, "report.html")
	if err := os.Remove(target); err != nil {
		t.Fatalf("remove: %v", err)
	}

	raw, err := h.svc.ArtifactBytes(context.Background(), id, model.ArtifactHTML)
	if err != nil {
		t.Fatalf("ArtifactBytes: %v", err)
	}
	if len(raw) < 1000 {
		t.Fatalf("regenerated report looks too small: %d bytes", len(raw))
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("artifact was not written back: %v", err)
	}
}

func TestUnknownArtifactKindIsRejected(t *testing.T) {
	h := newArtifactHarness(t, okHandler(t))
	id := h.run(t, "TP53")

	if _, err := h.svc.ArtifactBytes(context.Background(), id, "secrets"); !errors.Is(err, service.ErrUnknownArtifact) {
		t.Fatalf("expected ErrUnknownArtifact, got %v", err)
	}
}

func TestRerunKeepsTheOriginalRecordIntact(t *testing.T) {
	h := newArtifactHarness(t, okHandler(t))
	original := h.run(t, "TP53, BAX, MCM2")

	before, err := h.svc.Get(context.Background(), original)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	rerun, err := h.svc.Rerun(context.Background(), original)
	if err != nil {
		t.Fatalf("Rerun: %v", err)
	}
	if rerun.ID == original {
		t.Fatal("rerun reused the original id")
	}
	if rerun.InputHash != before.InputHash {
		t.Fatal("rerun changed the input hash")
	}
	if !strings.HasSuffix(rerun.Label, "重跑") {
		t.Fatalf("rerun label = %q", rerun.Label)
	}
	if len(rerun.Genes) != len(before.Genes) {
		t.Fatalf("rerun lost genes: %v", rerun.Genes)
	}
	if err := h.svc.ExecuteRun(context.Background(), rerun.ID); err != nil {
		t.Fatalf("ExecuteRun rerun: %v", err)
	}

	after, err := h.svc.Get(context.Background(), original)
	if err != nil {
		t.Fatalf("Get after: %v", err)
	}
	if after.ResultCount != before.ResultCount || after.InputHash != before.InputHash {
		t.Fatal("rerun mutated the historical record")
	}
}

func TestManifestIsDeterministicJSON(t *testing.T) {
	h := newArtifactHarness(t, okHandler(t))
	id := h.run(t, "TP53, BAX, MCM2")

	raw, err := h.svc.ArtifactBytes(context.Background(), id, model.ArtifactManifest)
	if err != nil {
		t.Fatalf("ArtifactBytes: %v", err)
	}
	var decoded dto.Manifest
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}
	if decoded.RunID != id {
		t.Fatalf("manifest run id = %q", decoded.RunID)
	}
	if decoded.SchemaVersion == "" || decoded.HashAlgorithm != "sha256" {
		t.Fatalf("manifest header = %+v", decoded)
	}
}

func TestRehashRepairsLegacyHashes(t *testing.T) {
	h := newArtifactHarness(t, okHandler(t))
	ctx := context.Background()
	id := h.run(t, "TP53, BAX, MCM2")

	if err := h.repo.UpdateInputHash(ctx, id, "legacy-hash-value"); err != nil {
		t.Fatalf("seed legacy hash: %v", err)
	}
	fixed, err := h.svc.Rehash(ctx)
	if err != nil {
		t.Fatalf("Rehash: %v", err)
	}
	if fixed != 1 {
		t.Fatalf("rehash count = %d, want 1", fixed)
	}
	detail, err := h.svc.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(detail.InputHash) != 64 || detail.InputHash == "legacy-hash-value" {
		t.Fatalf("hash was not repaired: %q", detail.InputHash)
	}
	if again, err := h.svc.Rehash(ctx); err != nil || again != 0 {
		t.Fatalf("rehash is not idempotent: %d, %v", again, err)
	}

	raw, err := h.svc.ArtifactBytes(ctx, id, model.ArtifactHTML)
	if err != nil || len(raw) < 1000 {
		t.Fatalf("artifacts did not come back after rehash: %v", err)
	}
}

func TestHashKeepsTheLegacyValueForUpperCaseSymbols(t *testing.T) {
	// The hash used to be sha256("hsapiens\0" + sorted original spelling).
	// Upper-casing normalisation must not change it for already-upper symbols,
	// otherwise every stored run would silently get a new identity.
	const legacy = "6c34f8a395fc4f3a8238b27f443b348932fa344caa7e1c1058dd834f52c594b9"
	if got := gene.Hash([]string{"TP53", "BAX"}, "hsapiens"); got != legacy {
		t.Fatalf("legacy hash drifted: %s", got)
	}
}
