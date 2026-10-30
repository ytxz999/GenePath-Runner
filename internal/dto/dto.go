package dto

import "time"

// Term is one enrichment row as exposed by the API. Every numeric field is the
// value g:Profiler returned; the API never recomputes them.
type Term struct {
	Source          string   `json:"source"`
	TermID          string   `json:"termId"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	PValue          float64  `json:"pValue"`
	SourceSize      int      `json:"sourceSize"`
	QuerySize       int      `json:"querySize"`
	Intersection    int      `json:"intersection"`
	EffectiveDomain int      `json:"effectiveDomain"`
	Significant     bool     `json:"significant"`
	GroupID         int      `json:"groupId"`
	Genes           []string `json:"genes"`
	Rank            int      `json:"rank"`
}

// Event is one audit-trail entry.
type Event struct {
	Kind    string    `json:"kind"`
	Message string    `json:"message"`
	At      time.Time `json:"at"`
}

// Params mirrors model.Params on the wire.
type Params struct {
	Significance string   `json:"significance"`
	Correction   string   `json:"correction"`
	Background   string   `json:"background"`
	Sources      []string `json:"sources"`
}

// RunSummary is the list/history representation of a run.
type RunSummary struct {
	ID            string     `json:"id"`
	Label         string     `json:"label"`
	Status        string     `json:"status"`
	Outcome       string     `json:"outcome"`
	Organism      string     `json:"organism"`
	GeneCount     int        `json:"geneCount"`
	MappedCount   int        `json:"mappedCount"`
	ResultCount   int        `json:"resultCount"`
	TermCount     int        `json:"termCount"`
	InputHash     string     `json:"inputHash"`
	DurationMS    int64      `json:"durationMs"`
	ProviderMS    int64      `json:"providerMs"`
	DataVersion   string     `json:"dataVersion"`
	HTTPStatus    int        `json:"httpStatus"`
	RequestBytes  int        `json:"requestBytes"`
	ResponseBytes int        `json:"responseBytes"`
	ErrorCode     string     `json:"errorCode"`
	ErrorMessage  string     `json:"errorMessage"`
	IsDemo        bool       `json:"isDemo"`
	Params        Params     `json:"params"`
	CreatedAt     time.Time  `json:"createdAt"`
	StartedAt     *time.Time `json:"startedAt"`
	FinishedAt    *time.Time `json:"finishedAt"`
}

// Threshold is one annotation source's statistical domain, echoed from the
// provider so the results page can explain the correction that was applied.
type Threshold struct {
	DomainSize    int     `json:"domainSize"`
	NumberOfTerms int     `json:"numberOfTerms"`
	Threshold     float64 `json:"threshold"`
}

// ArtifactInfo describes one stored file so the UI can link to it and show the
// hash that makes the record auditable.
type ArtifactInfo struct {
	Kind      string    `json:"kind"`
	Label     string    `json:"label"`
	Bytes     int       `json:"bytes"`
	SHA256    string    `json:"sha256"`
	Href      string    `json:"href"`
	CreatedAt time.Time `json:"createdAt"`
}

// RunDetail adds the gene list, enrichment rows and audit trail.
type RunDetail struct {
	RunSummary
	Genes       []string             `json:"genes"`
	Unmapped    []string             `json:"unmapped"`
	Duplicates  []string             `json:"duplicates"`
	Terms       []Term               `json:"terms"`
	Events      []Event              `json:"events"`
	Artifacts   []ArtifactInfo       `json:"artifacts"`
	Thresholds  map[string]Threshold `json:"thresholds"`
	DuplicateOf string               `json:"duplicateOf,omitempty"`
}

// RunResults is the payload of GET /api/v1/runs/{id}/results: what the results
// screen needs in a single round trip.
type RunResults struct {
	Run        RunSummary           `json:"run"`
	Genes      []string             `json:"genes"`
	Terms      []Term               `json:"terms"`
	Unmapped   []string             `json:"unmapped"`
	Duplicates []string             `json:"duplicates"`
	Events     []Event              `json:"events"`
	Artifacts  []ArtifactInfo       `json:"artifacts"`
	Thresholds map[string]Threshold `json:"thresholds"`
	Provider   ProviderInfo         `json:"provider"`
}

// Manifest is the reproducibility record written next to every run. It contains
// everything needed to explain how a result was produced: inputs, parameters,
// provider version and the exact bytes that were sent and received.
type Manifest struct {
	Project          string          `json:"project"`
	SchemaVersion    string          `json:"schema_version"`
	RunID            string          `json:"run_id"`
	Label            string          `json:"label"`
	Status           string          `json:"status"`
	Outcome          string          `json:"outcome"`
	InputHash        string          `json:"input_hash"`
	HashAlgorithm    string          `json:"hash_algorithm"`
	Organism         string          `json:"organism"`
	OrganismLabel    string          `json:"organism_label"`
	InputGenes       []string        `json:"input_genes"`
	InputGeneCount   int             `json:"input_gene_count"`
	Parameters       Params          `json:"parameters"`
	DataSource       string          `json:"data_source"`
	DataSourceURL    string          `json:"data_source_endpoint"`
	DataVersion      string          `json:"data_version"`
	CreatedAt        time.Time       `json:"created_at"`
	StartedAt        *time.Time      `json:"started_at,omitempty"`
	FinishedAt       *time.Time      `json:"finished_at,omitempty"`
	DurationMS       int64           `json:"duration_ms"`
	HTTPStatus       int             `json:"upstream_http_status"`
	RequestSnapshot  any             `json:"request_snapshot"`
	ResponseSnapshot any             `json:"response_snapshot"`
	ResultCount      int             `json:"result_count"`
	Mapping          ManifestMapping `json:"mapping"`
	Artifacts        []ArtifactInfo  `json:"artifacts,omitempty"`
	Notes            []string        `json:"notes"`
}

// ManifestMapping records what the provider did with the submitted symbols.
type ManifestMapping struct {
	Submitted  int      `json:"submitted"`
	Mapped     int      `json:"mapped"`
	Unmapped   []string `json:"unmapped"`
	Duplicates []string `json:"duplicates"`
	Message    string   `json:"message"`
}

// RawExchange is the stored upstream request/response pair for auditing.
type RawExchange struct {
	RunID    string `json:"runId"`
	Request  string `json:"request"`
	Response string `json:"response"`
}

// Paged wraps a list response with enough metadata for the history table.
type Paged struct {
	Items  []RunSummary `json:"items"`
	Total  int64        `json:"total"`
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
}

// DayCount is one activity-chart column.
type DayCount struct {
	Day   string `json:"day"`
	Count int    `json:"count"`
}

// StatusCount is one status-breakdown slice.
type StatusCount struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

// SourceCount is one annotation-source slice.
type SourceCount struct {
	Source string `json:"source"`
	Count  int    `json:"count"`
}

// Stats are the four dashboard headline numbers.
type Stats struct {
	Total     int64 `json:"total"`
	Succeeded int64 `json:"succeeded"`
	Failed    int64 `json:"failed"`
	Running   int64 `json:"running"`
}

// Dashboard is the single payload the home page needs.
type Dashboard struct {
	Stats           Stats         `json:"stats"`
	Statuses        []StatusCount `json:"statuses"`
	Activity        []DayCount    `json:"activity"`
	Sources         []SourceCount `json:"sources"`
	RecentRuns      []RunSummary  `json:"recentRuns"`
	AvgDurationMS   int64         `json:"avgDurationMs"`
	SuccessRate     float64       `json:"successRate"`
	Provider        ProviderInfo  `json:"provider"`
	DemoDataPresent bool          `json:"demoDataPresent"`
}

// ProviderInfo describes the upstream annotation source shown in the UI.
type ProviderInfo struct {
	Name        string `json:"name"`
	Endpoint    string `json:"endpoint"`
	DataVersion string `json:"dataVersion"`
	Configured  bool   `json:"configured"`
	Ready       bool   `json:"ready"`
}

// ValidateResponse is returned by POST /api/v1/runs/validate so the
// new-analysis form can show per-line feedback without creating a run.
type ValidateResponse struct {
	Accepted      []string `json:"accepted"`
	Rejected      []string `json:"rejected"`
	RejectedCount int      `json:"rejectedCount"`
	Duplicates    int      `json:"duplicates"`
	Count         int      `json:"count"`
	InputHash     string   `json:"inputHash"`
	DuplicateOf   string   `json:"duplicateOf,omitempty"`
}

// Problem is the single error envelope used by every endpoint.
type Problem struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
	Details any               `json:"details,omitempty"`
}
