package model

import (
	"strings"
	"time"
)

// Run status values. Stored as short strings so the JSON API and the database
// agree without an extra lookup table.
const (
	StatusQueued      = "queued"
	StatusRunning     = "running"
	StatusSucceeded   = "succeeded"
	StatusFailed      = "failed"
	StatusCanceled    = "canceled"
	StatusInterrupted = "interrupted"
)

// Run outcomes refine a succeeded run. They exist because "the request worked"
// and "we found enrichment" are different facts, and the results page has to
// say which one happened.
const (
	OutcomeEnriched      = "enriched"             // terms passed the threshold
	OutcomeNoSignificant = "no_significant_terms" // genes mapped, nothing significant
	OutcomeNoGenesMapped = "no_genes_mapped"      // provider could not map any gene
)

// Error codes attached to failed runs. The frontend maps each code to a
// human-readable explanation and a suggested next step.
const (
	ErrUpstreamTimeout     = "upstream_timeout"
	ErrUpstreamRejected    = "upstream_rejected"
	ErrUpstreamUnavailable = "upstream_unavailable"
	ErrUpstreamInvalidJSON = "upstream_invalid_json"
	ErrNetwork             = "network_error"
	ErrInternal            = "internal_error"
	ErrInterrupted         = "executor_stopped"
)

// IsTerminal reports whether a run can no longer change state on its own.
func IsTerminal(status string) bool {
	switch status {
	case StatusSucceeded, StatusFailed, StatusCanceled, StatusInterrupted:
		return true
	}
	return false
}

// Params are the enrichment settings captured with every run. They are stored
// verbatim so a historical result can always be traced back to the request that
// produced it.
type Params struct {
	Significance string   `json:"significance"`
	Correction   string   `json:"correction"`
	Background   string   `json:"background"`
	Sources      []string `json:"sources"`
}

// Run is one submitted gene-list analysis.
type Run struct {
	ID        string `gorm:"primaryKey;size:32" json:"id"`
	Label     string `gorm:"size:120" json:"label"`
	Status    string `gorm:"size:16;index" json:"status"`
	Outcome   string `gorm:"size:32" json:"outcome"`
	Organism  string `gorm:"size:32" json:"organism"`
	GeneCount int    `json:"geneCount"`
	Genes     string `gorm:"type:text" json:"-"`
	InputHash string `gorm:"size:64;index" json:"inputHash"`
	Params    Params `gorm:"serializer:json;type:text" json:"params"`

	// Provider interaction. RawRequest/RawResponse keep exactly what was sent
	// and received so a result can be audited without calling g:Profiler again.
	RawRequest     string `gorm:"type:text" json:"-"`
	RawResponse    string `gorm:"type:text" json:"-"`
	RequestBytes   int    `json:"requestBytes"`
	ResponseBytes  int    `json:"responseBytes"`
	HTTPStatus     int    `json:"httpStatus"`
	ProviderMS     int64  `json:"providerMs"`
	DataVersion    string `gorm:"size:96" json:"dataVersion"`
	MappedCount    int    `json:"mappedCount"`
	UnmappedGenes  string `gorm:"type:text" json:"-"`
	DuplicateGenes string `gorm:"type:text" json:"-"`
	Thresholds     string `gorm:"type:text" json:"-"`

	ResultCount int   `json:"resultCount"`
	TermCount   int   `json:"termCount"`
	DurationMS  int64 `json:"durationMs"`

	ErrorCode    string `gorm:"size:32" json:"errorCode"`
	ErrorMessage string `gorm:"size:512" json:"errorMessage"`

	// IsDemo marks rows shipped with the app so the UI can badge them. Demo
	// seeding is disabled by default from phase 2 on.
	IsDemo bool `gorm:"index" json:"isDemo"`

	CreatedAt  time.Time  `gorm:"index" json:"createdAt"`
	StartedAt  *time.Time `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt"`

	Terms []Term `gorm:"foreignKey:RunID;constraint:OnDelete:CASCADE" json:"-"`
}

// TableName keeps the SQL readable in logs.
func (Run) TableName() string { return "runs" }

// GeneList returns the canonical gene symbols stored with the run.
func (r *Run) GeneList() []string {
	return splitLines(r.Genes)
}

// UnmappedList returns the genes g:Profiler could not map to an identifier.
func (r *Run) UnmappedList() []string { return splitLines(r.UnmappedGenes) }

// DuplicateList returns the symbols the provider reported as duplicates.
func (r *Run) DuplicateList() []string { return splitLines(r.DuplicateGenes) }

// Term is one enrichment row, stored exactly as the provider returned it.
// GenePath never recomputes the statistics shown here.
type Term struct {
	ID              uint    `gorm:"primaryKey" json:"-"`
	RunID           string  `gorm:"size:32;index;not null" json:"-"`
	Source          string  `gorm:"size:16" json:"source"`
	TermID          string  `gorm:"size:64" json:"termId"`
	Name            string  `gorm:"size:255" json:"name"`
	Description     string  `gorm:"type:text" json:"description"`
	PValue          float64 `json:"pValue"`
	SourceSize      int     `json:"sourceSize"`
	QuerySize       int     `json:"querySize"`
	Intersection    int     `json:"intersection"`
	EffectiveDomain int     `json:"effectiveDomain"`
	Significant     bool    `json:"significant"`
	GroupID         int     `json:"groupId"`
	Genes           string  `gorm:"type:text" json:"-"`
	Rank            int     `json:"rank"`
}

// TableName keeps the SQL readable in logs.
func (Term) TableName() string { return "terms" }

// GeneList returns the genes that hit this term.
func (t *Term) GeneList() []string {
	if t.Genes == "" {
		return nil
	}
	return strings.Split(t.Genes, ",")
}

// RunEvent is an append-only audit trail entry: submission, provider call,
// mapping statistics and completion all land here.
type RunEvent struct {
	ID      uint      `gorm:"primaryKey" json:"-"`
	RunID   string    `gorm:"size:32;index;not null" json:"-"`
	Kind    string    `gorm:"size:32" json:"kind"`
	Message string    `gorm:"size:512" json:"message"`
	At      time.Time `json:"at"`
}

// TableName keeps the SQL readable in logs.
func (RunEvent) TableName() string { return "run_events" }

func splitLines(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
