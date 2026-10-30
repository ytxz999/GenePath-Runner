// Package gprofiler is a small client for the public g:Profiler g:GOSt
// enrichment REST API.
//
// The wire format below was taken from the official API reference
// (https://biit.cs.ut.ee/gprofiler/page/apis) and verified against the live
// service: request field names, the significance_threshold_method parameter and
// the response layout including meta.version and meta.genes_metadata.
//
// The client keeps the raw request and response bytes so a run can be audited
// later without calling the provider again.
package gprofiler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// DefaultEndpoint is the documented g:GOSt profile endpoint.
const DefaultEndpoint = "https://biit.cs.ut.ee/gprofiler/api/gost/profile/"

// Request is one enrichment query, already validated by the caller.
type Request struct {
	Organism      string
	Query         []string
	Sources       []string
	UserThreshold float64
	Correction    string
	DomainScope   string
}

// Row is one term exactly as the provider returned it.
type Row struct {
	Description         string     `json:"description"`
	EffectiveDomainSize int        `json:"effective_domain_size"`
	IntersectionSize    int        `json:"intersection_size"`
	Intersections       [][]string `json:"intersections"`
	Name                string     `json:"name"`
	Native              string     `json:"native"`
	PValue              float64    `json:"p_value"`
	Parents             []string   `json:"parents"`
	Precision           float64    `json:"precision"`
	Query               string     `json:"query"`
	QuerySize           int        `json:"query_size"`
	Recall              float64    `json:"recall"`
	Significant         bool       `json:"significant"`
	Source              string     `json:"source"`
	TermSize            int        `json:"term_size"`
	GroupID             int        `json:"group_id"`
}

// SourceMeta describes one annotation source's statistical domain.
type SourceMeta struct {
	DomainSize    int     `json:"domain_size"`
	NumberOfTerms int     `json:"number_of_terms"`
	Threshold     float64 `json:"threshold"`
}

// GenesMetadata is the mapping report for the submitted symbols.
type GenesMetadata struct {
	Failed     []string                   `json:"failed"`
	Ambiguous  map[string]json.RawMessage `json:"ambiguous"`
	Duplicates []string                   `json:"duplicates"`
	Query      map[string]QueryGenes      `json:"query"`
}

// MappedIDs is the identifier list for one submitted symbol. g:Profiler returns
// a bare string for a unique match and an array when several genes share the
// symbol, so the type accepts both shapes instead of failing the whole response.
type MappedIDs []string

// UnmarshalJSON accepts "ENSG…" as well as ["ENSG…","ENSG…"].
func (m *MappedIDs) UnmarshalJSON(b []byte) error {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var list []string
		if err := json.Unmarshal(trimmed, &list); err != nil {
			return err
		}
		*m = list
		return nil
	}
	var one string
	if err := json.Unmarshal(trimmed, &one); err != nil {
		return err
	}
	*m = []string{one}
	return nil
}

// QueryGenes holds one query's mapping (symbol -> identifier) and the ordered
// identifier list the intersections structure refers to.
type QueryGenes struct {
	Mapping map[string]MappedIDs `json:"mapping"`
	Ensgs   []string             `json:"ensgs"`
}

// Meta is the response metadata block.
type Meta struct {
	QueryMetadata  json.RawMessage       `json:"query_metadata"`
	ResultMetadata map[string]SourceMeta `json:"result_metadata"`
	GenesMetadata  GenesMetadata         `json:"genes_metadata"`
	Timestamp      string                `json:"timestamp"`
	Version        string                `json:"version"`
}

// Response is the normalised result of a successful call.
type Response struct {
	Rows        []Row
	Meta        Meta
	RequestRaw  []byte
	ResponseRaw []byte
	HTTPStatus  int
	Duration    time.Duration
}

// ErrorKind classifies a failed call so the service can store a stable code.
type ErrorKind string

const (
	KindRejected    ErrorKind = "rejected"     // 4xx: the request itself was refused
	KindUnavailable ErrorKind = "unavailable"  // 429 / 5xx: provider side problem
	KindTimeout     ErrorKind = "timeout"      // context deadline or transport timeout
	KindInvalidJSON ErrorKind = "invalid_json" // 2xx but unparsable body
	KindNetwork     ErrorKind = "network"      // DNS / connection failures
)

// ProviderError describes a failed upstream call.
type ProviderError struct {
	Kind       ErrorKind
	StatusCode int
	Message    string
}

func (e *ProviderError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("g:Profiler HTTP %d (%s): %s", e.StatusCode, e.Kind, e.Message)
	}
	return fmt.Sprintf("g:Profiler (%s): %s", e.Kind, e.Message)
}

// Client talks to the g:Profiler REST API.
type Client struct {
	Endpoint  string
	UserAgent string
	HTTP      *http.Client
}

// New builds a client. The transport timeout is a safety net: the executor also
// applies a per-run deadline.
func New(endpoint string) *Client {
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	return &Client{
		Endpoint:  endpoint,
		UserAgent: "GenePathRunner/0.2 (+https://github.com/ytxz999/GenePath-Runner)",
		HTTP: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

// wireRequest mirrors the documented JSON body. Field names and types are
// deliberately explicit: g:Profiler answers 400 with a JSON message when a
// parameter name is wrong, which is how the first draft of this client was
// caught using the wrong significance-threshold key.
type wireRequest struct {
	Organism                    string   `json:"organism"`
	Query                       []string `json:"query"`
	Sources                     []string `json:"sources"`
	UserThreshold               float64  `json:"user_threshold"`
	SignificanceThresholdMethod string   `json:"significance_threshold_method"`
	DomainScope                 string   `json:"domain_scope"`
	NoEvidences                 bool     `json:"no_evidences"`
	AllResults                  bool     `json:"all_results"`
}

type wireResponse struct {
	Result []Row `json:"result"`
	Meta   Meta  `json:"meta"`
}

// Profile performs one enrichment call.
func (c *Client) Profile(ctx context.Context, req Request) (*Response, error) {
	if len(req.Query) == 0 {
		return nil, &ProviderError{Kind: KindRejected, Message: "empty gene query"}
	}
	domainScope := req.DomainScope
	if domainScope == "" {
		domainScope = "annotated"
	}
	body, err := json.Marshal(wireRequest{
		Organism:                    req.Organism,
		Query:                       req.Query,
		Sources:                     req.Sources,
		UserThreshold:               req.UserThreshold,
		SignificanceThresholdMethod: req.Correction,
		DomainScope:                 domainScope,
		// Evidence codes are requested because they are the only documented way
		// to learn which submitted genes hit a term: `intersections` has one
		// entry per submitted gene, and a non-empty entry means "this gene is in
		// the intersection" (see the API reference for `intersections`).
		NoEvidences: false,
		AllResults:  false,
	})
	if err != nil {
		return nil, fmt.Errorf("g:Profiler: encode request: %w", err)
	}

	started := time.Now()
	resp, err := c.call(ctx, body)
	if err != nil {
		return nil, err
	}
	resp.RequestRaw = body
	resp.Duration = time.Since(started)
	return resp, nil
}

// call performs the HTTP round trip with a single retry for provider-side
// failures, which keeps a transient 502 from failing a user-visible run.
func (c *Client) call(ctx context.Context, body []byte) (*Response, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctxErr(ctx)
			case <-time.After(600 * time.Millisecond):
			}
		}
		resp, err := c.once(ctx, body)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		var pe *ProviderError
		if !errors.As(err, &pe) || pe.Kind != KindUnavailable {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *Client) once(ctx context.Context, body []byte) (*Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("g:Profiler: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, transportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()

	payload, readErr := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if readErr != nil {
		return nil, transportError(ctx, readErr)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &ProviderError{
			Kind:       kindForStatus(resp.StatusCode),
			StatusCode: resp.StatusCode,
			Message:    errorMessage(payload),
		}
	}

	var wire wireResponse
	if err := json.Unmarshal(payload, &wire); err != nil {
		return nil, &ProviderError{
			Kind:       KindInvalidJSON,
			StatusCode: resp.StatusCode,
			Message:    "cannot decode response: " + err.Error(),
		}
	}

	return &Response{
		Rows:        wire.Result,
		Meta:        wire.Meta,
		ResponseRaw: payload,
		HTTPStatus:  resp.StatusCode,
	}, nil
}

// transportError classifies a transport-level failure, keeping the distinction
// between "we gave up too early" (timeout) and "the network is broken".
func transportError(ctx context.Context, err error) error {
	if ctxErr(ctx) != nil {
		return ctxErr(ctx)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &ProviderError{Kind: KindTimeout, Message: err.Error()}
	}
	return &ProviderError{Kind: KindNetwork, Message: err.Error()}
}

func ctxErr(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return &ProviderError{Kind: KindTimeout, Message: "request deadline exceeded"}
	}
	if err := ctx.Err(); err != nil {
		return &ProviderError{Kind: KindNetwork, Message: err.Error()}
	}
	return nil
}

func kindForStatus(code int) ErrorKind {
	if code == http.StatusTooManyRequests || code >= 500 {
		return KindUnavailable
	}
	return KindRejected
}

// errorMessage extracts the documented {"message": "..."} error body, falling
// back to a trimmed raw body when the provider answers with HTML.
func errorMessage(payload []byte) string {
	var e struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(payload, &e); err == nil && e.Message != "" {
		return e.Message
	}
	trimmed := strings.TrimSpace(string(payload))
	trimmed = strings.Join(strings.Fields(trimmed), " ")
	if len(trimmed) > 240 {
		trimmed = trimmed[:240] + "…"
	}
	if trimmed == "" {
		return "empty response body"
	}
	return trimmed
}

// HitGenes maps one result row back to the submitted symbols. `intersections`
// has exactly one entry per submitted gene, in the same order as
// genes_metadata.query[query].ensgs, so a non-empty entry means the gene is part
// of the term. This is a mapping, not a re-computation of any statistic.
func HitGenes(row Row, genes QueryGenes) []string {
	if len(row.Intersections) == 0 || len(genes.Ensgs) == 0 {
		return nil
	}
	symbols := make(map[string]string, len(genes.Ensgs))
	for symbol, ids := range genes.Mapping {
		for _, id := range ids {
			if _, taken := symbols[id]; !taken {
				symbols[id] = symbol
			}
		}
	}
	limit := len(row.Intersections)
	if len(genes.Ensgs) < limit {
		limit = len(genes.Ensgs)
	}
	out := make([]string, 0, row.IntersectionSize)
	for i := 0; i < limit; i++ {
		if len(row.Intersections[i]) == 0 {
			continue
		}
		id := genes.Ensgs[i]
		if symbol, ok := symbols[id]; ok {
			out = append(out, symbol)
		} else {
			out = append(out, id)
		}
	}
	return out
}

// QueryGenesFor returns the mapping block belonging to a row.
func (m Meta) QueryGenesFor(query string) QueryGenes {
	if m.GenesMetadata.Query == nil {
		return QueryGenes{}
	}
	if q, ok := m.GenesMetadata.Query[query]; ok {
		return q
	}
	for _, q := range m.GenesMetadata.Query {
		return q
	}
	return QueryGenes{}
}
