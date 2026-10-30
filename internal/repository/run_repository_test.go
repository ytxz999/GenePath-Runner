package repository_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"genepath/internal/database"
	"genepath/internal/model"
	"genepath/internal/repository"
)

// TestRecordsSurviveAReopen is the store-level half of the phase-3 acceptance
// check: stop the process, start it again, and the run must still be there.
func TestRecordsSurviveAReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "persist.db")
	ctx := context.Background()

	db, err := database.Open(path, false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	repo := repository.NewRunRepository(db)

	started := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	finished := started.Add(2 * time.Second)
	run := &model.Run{
		ID:          "run_persist1",
		Label:       "persistence probe",
		Status:      model.StatusSucceeded,
		Outcome:     model.OutcomeEnriched,
		Organism:    "hsapiens",
		GeneCount:   2,
		Genes:       "TP53\nBAX",
		InputHash:   "abc123",
		Params:      model.Params{Significance: "0.05", Correction: "g_SCS", Background: "annotated", Sources: []string{"GO:BP"}},
		DataVersion: "e114_eg62_p19_9dc92ac8",
		ResultCount: 1,
		MappedCount: 2,
		RawRequest:  `{"organism":"hsapiens"}`,
		RawResponse: `{"result":[]}`,
		HTTPStatus:  200,
		CreatedAt:   started,
		StartedAt:   &started,
		FinishedAt:  &finished,
	}
	if err := repo.Create(ctx, run, []model.RunEvent{{RunID: run.ID, Kind: "created", Message: "submitted", At: started}}); err != nil {
		t.Fatalf("create: %v", err)
	}
	terms := []model.Term{{
		RunID: run.ID, TermID: "GO:0006915", Name: "apoptotic process", Source: "GO:BP",
		Description: "apoptotic process",
		PValue:      1.2e-9, Significant: true, QuerySize: 2, SourceSize: 500,
		EffectiveDomain: 17674, Intersection: 2, GroupID: 0,
		Genes: "TP53,BAX", Rank: 1,
	}}
	if err := repo.Finish(ctx, run, terms, nil); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if err := repo.UpsertArtifact(ctx, &model.Artifact{
		RunID: run.ID, Kind: model.ArtifactHTML,
		Path: run.ID + "/report.html", Bytes: 2048, SHA256: "deadbeef",
	}); err != nil {
		t.Fatalf("artifact: %v", err)
	}
	if err := database.Close(db); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Reopen the same file: nothing below may depend on in-memory state.
	db2, err := database.Open(path, false)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db2) })
	repo2 := repository.NewRunRepository(db2)

	got, err := repo2.Get(ctx, run.ID)
	if err != nil {
		t.Fatalf("get after reopen: %v", err)
	}
	if got.InputHash != run.InputHash || got.Genes != run.Genes || got.Status != model.StatusSucceeded {
		t.Fatalf("run changed across restart: %+v", got)
	}
	if got.Params.Correction != "g_SCS" || len(got.Params.Sources) != 1 {
		t.Fatalf("params lost their structure: %+v", got.Params)
	}
	if got.DataVersion != run.DataVersion || got.RawRequest == "" || got.RawResponse == "" {
		t.Fatal("provenance fields were not persisted")
	}

	stored, err := repo2.Terms(ctx, run.ID)
	if err != nil {
		t.Fatalf("terms: %v", err)
	}
	if len(stored) != 1 || stored[0].Name != "apoptotic process" || stored[0].TermID != "GO:0006915" {
		t.Fatalf("terms = %+v", stored)
	}

	artifact, err := repo2.Artifact(ctx, run.ID, model.ArtifactHTML)
	if err != nil {
		t.Fatalf("artifact: %v", err)
	}
	if artifact.SHA256 != "deadbeef" || artifact.Bytes != 2048 {
		t.Fatalf("artifact = %+v", artifact)
	}

	events, err := repo2.Events(ctx, run.ID)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("audit trail disappeared")
	}
}
