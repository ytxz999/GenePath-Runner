package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"genepath/internal/catalog"
	"genepath/internal/dto"
	"genepath/internal/gene"
	"genepath/internal/model"
	"genepath/internal/repository"
)

// ErrArtifactDisabled is returned when the service was built without an
// artifact directory (unit tests, mostly).
var ErrArtifactDisabled = errors.New("artifact storage is not configured")

// ErrUnknownArtifact is returned for an artifact kind the API does not serve.
var ErrUnknownArtifact = errors.New("unknown artifact kind")

// artifactSpec ties a logical kind to the file name and download metadata.
type artifactSpec struct {
	kind        string
	name        string
	label       string
	contentType string
}

var artifactSpecs = map[string]artifactSpec{
	model.ArtifactRequest:  {model.ArtifactRequest, "request.json", "原始请求", "application/json; charset=utf-8"},
	model.ArtifactResponse: {model.ArtifactResponse, "response.json", "原始响应", "application/json; charset=utf-8"},
	model.ArtifactManifest: {model.ArtifactManifest, "manifest.json", "运行清单", "application/json; charset=utf-8"},
	model.ArtifactHTML:     {model.ArtifactHTML, "report.html", "HTML 报告", "text/html; charset=utf-8"},
	model.ArtifactMarkdown: {model.ArtifactMarkdown, "report.md", "Markdown 报告", "text/markdown; charset=utf-8"},
}

// manifestInlineLimit keeps a runaway upstream payload out of the manifest.
const manifestInlineLimit = 512 * 1024

// ArtifactContentType maps a kind to its MIME type.
func ArtifactContentType(kind string) string {
	if spec, ok := artifactSpecs[kind]; ok {
		return spec.contentType
	}
	return "application/octet-stream"
}

// ArtifactFileName maps a kind to the suggested download name.
func ArtifactFileName(kind string) string {
	if spec, ok := artifactSpecs[kind]; ok {
		return spec.name
	}
	return kind
}

// ArtifactLabel maps a kind to the Chinese label used in the UI.
func ArtifactLabel(kind string) string {
	if spec, ok := artifactSpecs[kind]; ok {
		return spec.label
	}
	return kind
}

// Manifest builds the reproducibility record for one run. It is derived from
// stored rows only: no provider call happens here.
func (s *RunService) Manifest(ctx context.Context, id string) (*dto.Manifest, error) {
	detail, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	run, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	m := &dto.Manifest{
		Project:          "GenePath Runner",
		SchemaVersion:    "1.0",
		RunID:            detail.ID,
		Label:            detail.Label,
		Status:           detail.Status,
		Outcome:          detail.Outcome,
		InputHash:        detail.InputHash,
		HashAlgorithm:    "sha256",
		Organism:         detail.Organism,
		OrganismLabel:    catalog.OrganismLabel(detail.Organism),
		InputGenes:       detail.Genes,
		InputGeneCount:   detail.GeneCount,
		Parameters:       detail.Params,
		DataSource:       "g:Profiler",
		DataSourceURL:    s.endpoint,
		DataVersion:      detail.DataVersion,
		CreatedAt:        detail.CreatedAt,
		StartedAt:        detail.StartedAt,
		FinishedAt:       detail.FinishedAt,
		DurationMS:       detail.DurationMS,
		HTTPStatus:       detail.HTTPStatus,
		ResultCount:      detail.ResultCount,
		RequestSnapshot:  snapshot(run.RawRequest, model.ArtifactRequest),
		ResponseSnapshot: snapshot(run.RawResponse, model.ArtifactResponse),
		Mapping: dto.ManifestMapping{
			Submitted:  detail.GeneCount,
			Mapped:     detail.MappedCount,
			Unmapped:   detail.Unmapped,
			Duplicates: detail.Duplicates,
			Message:    mappingMessage(detail.GeneCount, detail.MappedCount, detail.Unmapped),
		},
		Artifacts: withoutKind(s.artifactInfos(ctx, id), model.ArtifactManifest),
		Notes: []string{
			"input_hash 只覆盖规范化后的基因符号与物种，不含时间戳，因此同一基因集重复粘贴会得到相同哈希。",
			"本文件是历史存档：重新打开是回看当时的结果；重新执行会生成新的运行记录。",
			"g:Profiler 的数据版本会更新，数据源更新后不保证得到完全相同的统计结果。",
			"所有 p 值、Term 规模等统计量均来自 g:Profiler 返回值，GenePath 未重新计算。",
		},
	}
	if m.InputGenes == nil {
		m.InputGenes = []string{}
	}
	if m.Mapping.Unmapped == nil {
		m.Mapping.Unmapped = []string{}
	}
	if m.Mapping.Duplicates == nil {
		m.Mapping.Duplicates = []string{}
	}
	return m, nil
}

// snapshot embeds the parsed JSON when it is small enough, and falls back to a
// pointer record when a response is too large. Both shapes are stable so a
// manifest stays readable either way.
func snapshot(raw string, kind string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		parsed = raw
	}
	if len(raw) <= manifestInlineLimit {
		return parsed
	}
	sum := sha256.Sum256([]byte(raw))
	return map[string]any{
		"embedded": false,
		"artifact": ArtifactFileName(kind),
		"sha256":   hex.EncodeToString(sum[:]),
		"bytes":    len(raw),
		"note":     "响应体积超过 512 KiB，完整内容见同目录下的原始文件",
	}
}

// withoutKind drops one artifact kind from a list. The manifest never lists
// itself: its own bytes would change every time the list changed.
func withoutKind(items []dto.ArtifactInfo, kind string) []dto.ArtifactInfo {
	out := make([]dto.ArtifactInfo, 0, len(items))
	for _, it := range items {
		if it.Kind == kind {
			continue
		}
		out = append(out, it)
	}
	return out
}

// Artifacts lists the stored files for a run and their hashes.
func (s *RunService) Artifacts(ctx context.Context, id string) ([]dto.ArtifactInfo, error) {
	if _, err := s.repo.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.artifactInfos(ctx, id), nil
}

func (s *RunService) artifactInfos(ctx context.Context, id string) []dto.ArtifactInfo {
	rows, err := s.repo.Artifacts(ctx, id)
	if err != nil || len(rows) == 0 {
		return nil
	}
	out := make([]dto.ArtifactInfo, 0, len(rows))
	for _, row := range rows {
		out = append(out, dto.ArtifactInfo{
			Kind:      row.Kind,
			Label:     ArtifactLabel(row.Kind),
			Bytes:     row.Bytes,
			SHA256:    row.SHA256,
			Href:      fmt.Sprintf("/api/v1/runs/%s/artifacts/%s", row.RunID, row.Kind),
			CreatedAt: row.CreatedAt,
		})
	}
	return out
}

// EnsureArtifacts renders and stores the full bundle for a run. It is
// idempotent: calling it twice rewrites the same files and refreshes the rows.
func (s *RunService) EnsureArtifacts(ctx context.Context, id string) ([]dto.ArtifactInfo, error) {
	if s.artifactDir == "" {
		return nil, ErrArtifactDisabled
	}
	detail, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	run, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	html, err := s.renderHTML(detail)
	if err != nil {
		return nil, err
	}
	markdown, err := s.Report(ctx, id)
	if err != nil {
		return nil, err
	}

	type item struct {
		kind string
		data []byte
	}
	// Store the payload files first, then build the manifest so it can list
	// them with their final hashes.
	bundle := []item{
		{model.ArtifactHTML, html},
		{model.ArtifactMarkdown, []byte(markdown)},
	}
	if run.RawRequest != "" {
		bundle = append(bundle, item{model.ArtifactRequest, []byte(run.RawRequest)})
	}
	if run.RawResponse != "" {
		bundle = append(bundle, item{model.ArtifactResponse, []byte(run.RawResponse)})
	}

	for _, it := range bundle {
		if _, err := s.storeArtifact(ctx, id, it.kind, it.data); err != nil {
			return nil, err
		}
	}

	manifest, err := s.Manifest(ctx, id)
	if err != nil {
		return nil, err
	}
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	if _, err := s.storeArtifact(ctx, id, model.ArtifactManifest, manifestJSON); err != nil {
		return nil, err
	}
	return s.artifactInfos(ctx, id), nil
}

// ArtifactBytes returns the stored bytes for one kind, generating and persisting
// the file when it is missing. Reports are therefore always consistent with the
// database at the moment they are first requested.
func (s *RunService) ArtifactBytes(ctx context.Context, id, kind string) ([]byte, error) {
	spec, ok := artifactSpecs[kind]
	if !ok {
		return nil, ErrUnknownArtifact
	}
	if row, err := s.repo.Artifact(ctx, id, kind); err == nil && row != nil && row.Path != "" && s.artifactDir != "" {
		if data, err := os.ReadFile(filepath.Join(s.artifactDir, row.Path)); err == nil {
			if kind != model.ArtifactManifest {
				return data, nil
			}
		}
	}
	// The manifest is a view over the other artifacts, so it is always rendered
	// from the database and then re-persisted. A stale copy would describe an
	// older file set.
	if kind == model.ArtifactManifest {
		data, err := s.generate(ctx, id, kind)
		if err != nil {
			return nil, err
		}
		if s.artifactDir != "" {
			if _, err := s.storeArtifact(ctx, id, kind, data); err != nil {
				return nil, err
			}
		}
		return data, nil
	}
	if s.artifactDir == "" {
		return s.generate(ctx, id, kind)
	}
	if _, err := s.EnsureArtifacts(ctx, id); err != nil {
		return nil, err
	}
	if row, err := s.repo.Artifact(ctx, id, spec.kind); err == nil && row != nil {
		if data, err := os.ReadFile(filepath.Join(s.artifactDir, row.Path)); err == nil {
			return data, nil
		}
	}
	return s.generate(ctx, id, kind)
}

// generate renders one artifact in memory without touching the filesystem.
func (s *RunService) generate(ctx context.Context, id, kind string) ([]byte, error) {
	detail, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	run, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	switch kind {
	case model.ArtifactManifest:
		manifest, err := s.Manifest(ctx, id)
		if err != nil {
			return nil, err
		}
		return json.MarshalIndent(manifest, "", "  ")
	case model.ArtifactHTML:
		return s.renderHTML(detail)
	case model.ArtifactMarkdown:
		md, err := s.Report(ctx, id)
		return []byte(md), err
	case model.ArtifactRequest:
		return []byte(run.RawRequest), nil
	case model.ArtifactResponse:
		return []byte(run.RawResponse), nil
	}
	return nil, repository.ErrNotFound
}

func (s *RunService) storeArtifact(ctx context.Context, runID, kind string, data []byte) (dto.ArtifactInfo, error) {
	spec, ok := artifactSpecs[kind]
	if !ok {
		return dto.ArtifactInfo{}, repository.ErrNotFound
	}
	rel := filepath.ToSlash(filepath.Join(runID, spec.name))
	abs := filepath.Join(s.artifactDir, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return dto.ArtifactInfo{}, err
	}
	if err := os.WriteFile(abs, data, 0o644); err != nil {
		return dto.ArtifactInfo{}, err
	}
	sum := sha256.Sum256(data)
	now := s.now()
	row := &model.Artifact{
		RunID:     runID,
		Kind:      kind,
		Path:      rel,
		SHA256:    hex.EncodeToString(sum[:]),
		Bytes:     len(data),
		CreatedAt: now,
	}
	if err := s.repo.UpsertArtifact(ctx, row); err != nil {
		return dto.ArtifactInfo{}, err
	}
	return dto.ArtifactInfo{
		Kind:      kind,
		Label:     spec.label,
		Bytes:     len(data),
		SHA256:    row.SHA256,
		Href:      fmt.Sprintf("/api/v1/runs/%s/artifacts/%s", runID, kind),
		CreatedAt: now,
	}, nil
}

// Rerun creates a fresh run from a stored one. It exists so "回看存档" and
// "重新执行" are two clearly different actions in the UI.
func (s *RunService) Rerun(ctx context.Context, id string) (*dto.RunDetail, error) {
	run, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	genes := run.GeneList()
	if len(genes) == 0 {
		return nil, validationErr("genes", "原运行没有保存输入基因，无法重跑")
	}
	label := strings.TrimSpace(run.Label)
	if label == "" {
		label = "基因列表"
	}
	if runes := []rune(label); len(runes) > 90 {
		label = string(runes[:90])
	}
	return s.Create(ctx, CreateInput{
		Label:        label + " · 重跑",
		Genes:        strings.Join(genes, "\n"),
		Organism:     run.Organism,
		Sources:      run.Params.Sources,
		Significance: run.Params.Significance,
		Correction:   run.Params.Correction,
		Background:   run.Params.Background,
	})
}

// ---- HTML report -----------------------------------------------------------

type reportView struct {
	Title       string
	Label       string
	RunID       string
	Status      string
	Outcome     string
	IsDemo      bool
	Organism    string
	OrganismRaw string
	GeneCount   int
	InputGenes  []string
	Mapped      int
	Unmapped    []string
	Duplicates  []string
	MappingText string
	InputHash   string
	Terms       []reportTerm
	HasTerms    bool
	Params      dto.Params
	DataVersion string
	Endpoint    string
	Created     string
	Finished    string
	Duration    string
	ResultCount int
	HTTPStatus  int
	Generated   string
	Bars        []reportBar
}

type reportTerm struct {
	Rank   int
	Source string
	TermID string
	Name   string
	PValue string
	Hits   string
}

type reportBar struct {
	Name   string
	Alt    bool
	Width  int
	PValue string
}

func (s *RunService) renderHTML(detail *dto.RunDetail) ([]byte, error) {
	view := reportView{
		Title:       "GenePath 分析报告",
		Label:       detail.Label,
		RunID:       detail.ID,
		Status:      statusLabel(detail.Status),
		Outcome:     detail.Outcome,
		IsDemo:      detail.IsDemo,
		Organism:    catalog.OrganismLabel(detail.Organism),
		OrganismRaw: detail.Organism,
		GeneCount:   detail.GeneCount,
		InputGenes:  detail.Genes,
		Mapped:      detail.MappedCount,
		Unmapped:    detail.Unmapped,
		Duplicates:  detail.Duplicates,
		MappingText: mappingMessage(detail.GeneCount, detail.MappedCount, detail.Unmapped),
		InputHash:   detail.InputHash,
		Params:      detail.Params,
		DataVersion: orDash(detail.DataVersion),
		Endpoint:    s.endpoint,
		Created:     detail.CreatedAt.Format("2006-01-02 15:04:05"),
		Duration:    formatDuration(detail.DurationMS),
		ResultCount: detail.ResultCount,
		HTTPStatus:  detail.HTTPStatus,
		Generated:   s.now().Format("2006-01-02 15:04:05 MST"),
		HasTerms:    len(detail.Terms) > 0,
	}
	if detail.FinishedAt != nil {
		view.Finished = detail.FinishedAt.Format("2006-01-02 15:04:05")
	}
	if view.InputGenes == nil {
		view.InputGenes = []string{}
	}

	for i, t := range detail.Terms {
		view.Terms = append(view.Terms, reportTerm{
			Rank:   i + 1,
			Source: t.Source,
			TermID: t.TermID,
			Name:   t.Name,
			PValue: fmt.Sprintf("%.2e", t.PValue),
			Hits:   fmt.Sprintf("%d/%d", t.Intersection, t.QuerySize),
		})
	}

	if top := detail.Terms; len(top) > 0 {
		if len(top) > 10 {
			top = top[:10]
		}
		sorted := make([]dto.Term, len(top))
		copy(sorted, top)
		sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].PValue < sorted[j].PValue })
		maxScore := 0.0
		for _, t := range sorted {
			if score := -math.Log10(t.PValue); score > maxScore {
				maxScore = score
			}
		}
		for _, t := range sorted {
			width := 4
			if maxScore > 0 {
				width = int(math.Round(-math.Log10(t.PValue) / maxScore * 100))
			}
			if width < 4 {
				width = 4
			}
			view.Bars = append(view.Bars, reportBar{
				Name:   t.Name,
				Alt:    !strings.HasPrefix(t.Source, "GO:"),
				Width:  width,
				PValue: fmt.Sprintf("%.2e", t.PValue),
			})
		}
	}

	var buf bytes.Buffer
	if err := reportTemplate.Execute(&buf, view); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

var reportTemplate = template.Must(template.New("report").Funcs(template.FuncMap{
	"join": strings.Join,
}).Parse(reportHTMLTemplate))

// Rehash repairs stored input hashes whose algorithm predates the current
// normalisation. New runs are hashed at submission time; this pass only exists
// so an older database keeps one consistent identity for "the same input".
//
// Stale artifacts are dropped for any repaired run: their manifest embeds the
// hash, so a rebuilt manifest is the honest one.
func (s *RunService) Rehash(ctx context.Context) (int, error) {
	runs, err := s.repo.All(ctx)
	if err != nil {
		return 0, err
	}
	changed := 0
	for i := range runs {
		want := gene.Hash(gene.Parse(runs[i].Genes), runs[i].Organism)
		if want == runs[i].InputHash {
			continue
		}
		if err := s.repo.UpdateInputHash(ctx, runs[i].ID, want); err != nil {
			return changed, err
		}
		if err := s.repo.DeleteArtifacts(ctx, runs[i].ID); err != nil {
			return changed, err
		}
		if s.artifactDir != "" {
			_ = os.RemoveAll(filepath.Join(s.artifactDir, runs[i].ID))
		}
		changed++
	}
	return changed, nil
}
