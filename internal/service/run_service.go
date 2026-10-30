// Package service holds the business rules of GenePath Runner. Handlers stay
// thin: they translate HTTP into calls on this package and nothing else.
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"genepath/internal/catalog"
	"genepath/internal/dto"
	"genepath/internal/gene"
	"genepath/internal/model"
	"genepath/internal/provider/gprofiler"
	"genepath/internal/repository"
)

// ValidationError carries per-field messages so the form can highlight the
// exact input that needs fixing instead of showing one generic toast.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	if msg, ok := e.Fields["_"]; ok {
		return msg
	}
	parts := make([]string, 0, len(e.Fields))
	for k, v := range e.Fields {
		parts = append(parts, k+": "+v)
	}
	sort.Strings(parts)
	return strings.Join(parts, "; ")
}

func validationErr(field, message string) *ValidationError {
	return &ValidationError{Fields: map[string]string{field: message}}
}

// CreateInput is the validated shape of a create-run request.
type CreateInput struct {
	Label        string   `json:"label"`
	Genes        string   `json:"genes"`
	Organism     string   `json:"organism"`
	Sources      []string `json:"sources"`
	Significance Number   `json:"significance"`
	Correction   string   `json:"correction"`
	Background   string   `json:"background"`
}

// Number is a JSON scalar that may arrive as either a quoted string or a bare
// number. The web form posts strings ("0.05"), while API clients and curl
// examples naturally post numbers (0.05); both mean the same thing, and an
// omitted value stays empty so the service can fall back to its default.
type Number string

// UnmarshalJSON accepts "0.05", 0.05 and null.
func (n *Number) UnmarshalJSON(b []byte) error {
	trimmed := strings.TrimSpace(string(b))
	if trimmed == "null" {
		*n = ""
		return nil
	}
	if len(trimmed) >= 2 && trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*n = Number(strings.TrimSpace(s))
		return nil
	}
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return err
	}
	*n = Number(trimmed)
	return nil
}

// Options configures non-functional aspects of the service.
type Options struct {
	ProviderEndpoint string
	ProviderReady    bool
	Client           *gprofiler.Client
	ExecTimeout      time.Duration
	Now              func() time.Time
	// ArtifactDir is where report and manifest files are written. An empty
	// value disables persistence and keeps unit tests off the filesystem.
	ArtifactDir string
}

// RunService implements run lifecycle, listing, dashboard aggregation, the
// provider call itself and report rendering.
type RunService struct {
	repo     *repository.RunRepository
	endpoint string
	ready    bool
	now      func() time.Time
	client   *gprofiler.Client
	timeout  time.Duration
	// artifactDir is the root directory for per-run files ("" = memory only).
	artifactDir string

	// enqueue hands a freshly stored run id to the worker queue. It is set by
	// main after construction so the service never depends on the executor.
	enqueue func(string) error
}

// NewRunService wires the service to its repository.
func NewRunService(repo *repository.RunRepository, opts Options) *RunService {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	client := opts.Client
	if client == nil {
		client = gprofiler.New(opts.ProviderEndpoint)
	}
	timeout := opts.ExecTimeout
	if timeout <= 0 {
		timeout = 75 * time.Second
	}
	return &RunService{
		repo:        repo,
		endpoint:    opts.ProviderEndpoint,
		ready:       opts.ProviderReady,
		now:         now,
		client:      client,
		timeout:     timeout,
		artifactDir: opts.ArtifactDir,
	}
}

// AttachQueue registers the callback used to schedule a stored run.
func (s *RunService) AttachQueue(fn func(string) error) { s.enqueue = fn }

// Ready reports whether the executor is wired and can run analyses.
func (s *RunService) Ready() bool { return s.ready && s.enqueue != nil }

type prepared struct {
	label        string
	genes        []string
	organism     string
	sources      []string
	significance string
	threshold    float64
	correction   string
	background   string
	hash         string
}

// prepare validates and normalises a create request, collecting every problem
// at once so the form can show them together.
func prepare(in CreateInput) (*prepared, error) {
	fields := map[string]string{}

	parsed := gene.Parse(in.Genes)
	valid, invalid := gene.Split(parsed)

	if len(valid) == 0 && len(invalid) == 0 {
		fields["genes"] = "请至少输入 1 个基因符号"
	} else if len(invalid) > 0 {
		shown := invalid
		if len(shown) > 6 {
			shown = shown[:6]
		}
		fields["genes"] = fmt.Sprintf("检测到 %d 个不支持的符号（%s），基因符号只能包含字母、数字和 - _ .",
			len(invalid), strings.Join(shown, ", "))
	} else if len(valid) > gene.MaxGenes {
		fields["genes"] = fmt.Sprintf("一次最多提交 %d 个基因，当前 %d 个", gene.MaxGenes, len(valid))
	}

	organism := strings.TrimSpace(in.Organism)
	if organism == "" {
		organism = catalog.DefaultOrganism
	}
	if _, ok := catalog.OrganismByCode(organism); !ok {
		fields["organism"] = "该物种暂不支持，请从列表中选择"
	}

	sources := make([]string, 0, len(in.Sources))
	seen := map[string]bool{}
	for _, s := range in.Sources {
		s = strings.ToUpper(strings.TrimSpace(s))
		if s == "" || seen[s] {
			continue
		}
		if !catalog.ValidSource(s) {
			fields["sources"] = "包含未知的注释来源：" + s
			continue
		}
		seen[s] = true
		sources = append(sources, s)
	}
	if len(sources) == 0 {
		if in.Sources == nil {
			// Field omitted entirely: fall back to the documented default source.
			sources = []string{catalog.DefaultSource}
		} else if _, bad := fields["sources"]; !bad {
			// Field present but empty (or only blanks): the caller must pick one.
			fields["sources"] = "至少选择一个注释来源"
		}
	}

	significance := strings.TrimSpace(string(in.Significance))
	if significance == "" {
		significance = catalog.DefaultSignificance
	}
	threshold, err := strconv.ParseFloat(significance, 64)
	if err != nil || threshold <= 0 || threshold >= 1 {
		fields["significance"] = "显著性阈值需要是 0 到 1 之间的小数，例如 0.05"
	}

	correction := strings.TrimSpace(in.Correction)
	if correction == "" {
		correction = catalog.DefaultCorrection
	}
	if !catalog.ValidCorrection(correction) {
		fields["correction"] = "不支持的校正方法"
	}

	background := strings.TrimSpace(in.Background)
	if background == "" {
		background = catalog.DefaultBackground
	}
	if background != "annotated" && background != "known" {
		fields["background"] = "背景基因集只能是 annotated 或 known"
	}

	if len(fields) > 0 {
		return nil, &ValidationError{Fields: fields}
	}

	label := strings.TrimSpace(in.Label)
	if label == "" {
		label = "未命名分析"
	}
	if len([]rune(label)) > 80 {
		label = string([]rune(label)[:80])
	}

	return &prepared{
		label:        label,
		genes:        valid,
		organism:     organism,
		sources:      sources,
		significance: significance,
		threshold:    threshold,
		correction:   correction,
		background:   background,
		hash:         gene.Hash(valid, organism),
	}, nil
}

// Validate reports what the server would accept without creating a run. It also
// looks up whether the same gene set ran before, so the form can warn early.
func (s *RunService) Validate(ctx context.Context, in CreateInput) (*dto.ValidateResponse, error) {
	raw := gene.ParseRaw(in.Genes)
	normalized := gene.Normalize(raw)
	valid, invalid := gene.Split(normalized)

	rejectedCount := len(invalid)
	if len(invalid) > 50 {
		invalid = invalid[:50]
	}

	organism := strings.TrimSpace(in.Organism)
	if organism == "" {
		organism = catalog.DefaultOrganism
	}

	res := &dto.ValidateResponse{
		Accepted:      valid,
		Rejected:      invalid,
		RejectedCount: rejectedCount,
		Duplicates:    len(raw) - len(normalized),
		Count:         len(valid),
		InputHash:     gene.Hash(valid, organism),
	}
	if res.Accepted == nil {
		res.Accepted = []string{}
	}
	if res.Rejected == nil {
		res.Rejected = []string{}
	}

	if len(valid) > 0 {
		previous, err := s.repo.FindByHash(ctx, res.InputHash, organism, "")
		if err == nil && previous != nil {
			res.DuplicateOf = previous.ID
		}
	}
	return res, nil
}

// ErrNotReady is returned when the executor is not wired, so accepting a run
// would create a record that can never finish.
var ErrNotReady = errors.New("分析执行器尚未就绪")

// ErrQueueFull is returned when too many runs are already waiting.
var ErrQueueFull = errors.New("执行队列已满，请稍后重试")

// Create validates the input, stores a queued run and schedules it. It returns
// as soon as the row exists: the provider call happens in the executor.
func (s *RunService) Create(ctx context.Context, in CreateInput) (*dto.RunDetail, error) {
	p, err := prepare(in)
	if err != nil {
		return nil, err
	}
	if !s.Ready() {
		return nil, ErrNotReady
	}

	now := s.now()
	run := &model.Run{
		ID:        newRunID(),
		Label:     p.label,
		Status:    model.StatusQueued,
		Organism:  p.organism,
		GeneCount: len(p.genes),
		Genes:     strings.Join(p.genes, "\n"),
		InputHash: p.hash,
		Params:    model.Params{Significance: p.significance, Correction: p.correction, Background: p.background, Sources: p.sources},
		CreatedAt: now,
	}
	events := []model.RunEvent{
		{Kind: "submitted", Message: fmt.Sprintf("接收 %d 个基因，物种 %s", len(p.genes), catalog.OrganismLabel(p.organism)), At: now},
		{Kind: "queued", Message: "已进入执行队列，等待 worker 调用 g:Profiler。", At: now},
	}
	if err := s.repo.Create(ctx, run, events); err != nil {
		return nil, err
	}

	if err := s.enqueue(run.ID); err != nil {
		failed := *run
		failed.Status = model.StatusFailed
		failed.ErrorCode = model.ErrInternal
		failed.ErrorMessage = "无法进入执行队列：" + err.Error()
		finished := s.now()
		failed.FinishedAt = &finished
		_ = s.repo.Finish(ctx, &failed, nil, []model.RunEvent{
			{Kind: "failed", Message: failed.ErrorMessage, At: finished},
		})
		return nil, ErrQueueFull
	}

	previous, err := s.repo.FindByHash(ctx, p.hash, p.organism, run.ID)
	if err != nil {
		return nil, err
	}
	detail := toDetail(run, nil, events)
	if previous != nil {
		detail.DuplicateOf = previous.ID
	}
	return detail, nil
}

// ExecuteRun performs the analysis for one stored run. It is the only function
// that talks to the provider, which keeps the "what did we send and what came
// back" story in a single place.
func (s *RunService) ExecuteRun(ctx context.Context, id string) error {
	run, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if model.IsTerminal(run.Status) {
		return nil
	}

	started := s.now()
	if err := s.repo.MarkRunning(ctx, id, started, model.RunEvent{
		Kind: "started", Message: "worker 已接管，准备调用 g:Profiler", At: started,
	}); err != nil {
		if errors.Is(err, repository.ErrNotQueued) {
			return nil
		}
		return err
	}

	threshold, _ := strconv.ParseFloat(run.Params.Significance, 64)
	req := gprofiler.Request{
		Organism:      run.Organism,
		Query:         run.GeneList(),
		Sources:       run.Params.Sources,
		UserThreshold: threshold,
		Correction:    run.Params.Correction,
		DomainScope:   run.Params.Background,
	}

	callCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	resp, callErr := s.client.Profile(callCtx, req)
	finished := s.now()

	if callErr != nil {
		return s.failRun(ctx, run, callErr, finished)
	}

	return s.completeRun(ctx, run, resp, threshold, finished)
}

// completeRun stores a successful provider exchange and derives the outcome.
func (s *RunService) completeRun(ctx context.Context, run *model.Run, resp *gprofiler.Response, threshold float64, finished time.Time) error {
	query := resp.Meta.QueryGenesFor("query_1")

	terms := make([]model.Term, 0, len(resp.Rows))
	significant := 0
	for _, row := range resp.Rows {
		if row.Significant {
			significant++
		}
		terms = append(terms, model.Term{
			Source: row.Source,
			// In the g:GOSt schema "native" carries the term identifier
			// (GO:0072332 / KEGG:04115) while "name" and "description" carry the
			// human-readable label. Verified against a live response.
			TermID:          row.Native,
			Name:            row.Name,
			Description:     row.Description,
			PValue:          row.PValue,
			SourceSize:      row.TermSize,
			QuerySize:       row.QuerySize,
			Intersection:    row.IntersectionSize,
			EffectiveDomain: row.EffectiveDomainSize,
			Significant:     row.Significant,
			GroupID:         row.GroupID,
			Genes:           strings.Join(gprofiler.HitGenes(row, query), ","),
		})
	}
	// The provider returns rows grouped by term family; sort by p-value so rank
	// 1 is always the strongest signal and the table matches the chart.
	sort.SliceStable(terms, func(i, j int) bool { return terms[i].PValue < terms[j].PValue })
	for i := range terms {
		terms[i].Rank = i + 1
	}

	mapped := len(query.Mapping)
	outcome := model.OutcomeEnriched
	switch {
	case mapped == 0:
		outcome = model.OutcomeNoGenesMapped
	case significant == 0:
		outcome = model.OutcomeNoSignificant
	}

	thresholds := map[string]dto.Threshold{}
	for source, meta := range resp.Meta.ResultMetadata {
		thresholds[source] = dto.Threshold{
			DomainSize:    meta.DomainSize,
			NumberOfTerms: meta.NumberOfTerms,
			Threshold:     meta.Threshold,
		}
	}
	thresholdsJSON, _ := json.Marshal(thresholds)

	run.Status = model.StatusSucceeded
	run.Outcome = outcome
	run.DataVersion = resp.Meta.Version
	run.HTTPStatus = resp.HTTPStatus
	run.RawRequest = string(resp.RequestRaw)
	run.RawResponse = string(resp.ResponseRaw)
	run.RequestBytes = len(resp.RequestRaw)
	run.ResponseBytes = len(resp.ResponseRaw)
	run.ProviderMS = resp.Duration.Milliseconds()
	run.MappedCount = mapped
	run.UnmappedGenes = strings.Join(resp.Meta.GenesMetadata.Failed, "\n")
	run.DuplicateGenes = strings.Join(resp.Meta.GenesMetadata.Duplicates, "\n")
	run.Thresholds = string(thresholdsJSON)
	run.TermCount = len(terms)
	run.ResultCount = significant
	run.DurationMS = finished.Sub(valueOr(run.StartedAt, run.CreatedAt)).Milliseconds()
	run.FinishedAt = &finished

	events := []model.RunEvent{
		{Kind: "provider_call", Message: fmt.Sprintf(
			"POST %s → HTTP %d，%.1f KB，%d ms（阈值 %g，校正 %s）",
			s.endpoint, resp.HTTPStatus, float64(len(resp.ResponseRaw))/1024, resp.Duration.Milliseconds(), threshold, run.Params.Correction,
		), At: finished},
		{Kind: "mapping", Message: mappingMessage(run.GeneCount, mapped, run.UnmappedList()), At: finished},
		{Kind: "completed", Message: fmt.Sprintf(
			"返回 %d 条注释，其中 %d 条达到显著性阈值；数据源版本 %s。GenePath 未重新计算任何统计量。",
			len(terms), significant, resp.Meta.Version,
		), At: finished},
	}

	if err := s.repo.Finish(ctx, run, terms, events); err != nil {
		return err
	}
	// Best effort: a report that fails to render must never fail the analysis,
	// and the download path regenerates anything that is missing.
	if s.artifactDir != "" {
		_, _ = s.EnsureArtifacts(ctx, run.ID)
	}
	return nil
}

// failRun classifies a provider failure, stores it and returns the reason.
func (s *RunService) failRun(ctx context.Context, run *model.Run, callErr error, finished time.Time) error {
	code := model.ErrInternal
	message := callErr.Error()

	var pe *gprofiler.ProviderError
	if errors.As(callErr, &pe) {
		if pe.StatusCode > 0 {
			run.HTTPStatus = pe.StatusCode
		}
		switch pe.Kind {
		case gprofiler.KindTimeout:
			code = model.ErrUpstreamTimeout
			message = "g:Profiler 未在超时时间内返回结果，请稍后重试或减少基因数量。"
		case gprofiler.KindRejected:
			code = model.ErrUpstreamRejected
			message = "g:Profiler 拒绝了这个请求：" + pe.Message
		case gprofiler.KindUnavailable:
			code = model.ErrUpstreamUnavailable
			message = "g:Profiler 暂时不可用（HTTP " + strconv.Itoa(pe.StatusCode) + "）：" + pe.Message
		case gprofiler.KindInvalidJSON:
			code = model.ErrUpstreamInvalidJSON
			message = "g:Profiler 返回了无法解析的响应：" + pe.Message
		case gprofiler.KindNetwork:
			code = model.ErrNetwork
			message = "无法连接 g:Profiler：" + pe.Message
		}
	} else if errors.Is(callErr, context.DeadlineExceeded) {
		code = model.ErrUpstreamTimeout
		message = "g:Profiler 未在超时时间内返回结果。"
	}

	run.Status = model.StatusFailed
	run.ErrorCode = code
	run.ErrorMessage = truncate(message, 500)
	run.DurationMS = finished.Sub(valueOr(run.StartedAt, run.CreatedAt)).Milliseconds()
	run.FinishedAt = &finished

	events := []model.RunEvent{
		{Kind: "failed", Message: run.ErrorMessage, At: finished},
	}
	if err := s.repo.Finish(ctx, run, nil, events); err != nil {
		return err
	}
	return callErr
}

// ClearDemo deletes the seeded demo rows. It exists so a reviewer can get an
// empty workspace and judge the product on real analyses only.
func (s *RunService) ClearDemo(ctx context.Context) (int64, error) {
	return s.repo.DeleteDemo(ctx)
}

// Results returns everything the results screen needs.
func (s *RunService) Results(ctx context.Context, id string) (*dto.RunResults, error) {
	detail, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &dto.RunResults{
		Run:        detail.RunSummary,
		Genes:      detail.Genes,
		Terms:      detail.Terms,
		Unmapped:   detail.Unmapped,
		Duplicates: detail.Duplicates,
		Events:     detail.Events,
		Artifacts:  s.artifactInfos(ctx, id),
		Thresholds: detail.Thresholds,
		Provider: dto.ProviderInfo{
			Name:        "g:Profiler",
			Endpoint:    s.endpoint,
			DataVersion: detail.DataVersion,
			Configured:  s.ready,
			Ready:       s.Ready(),
		},
	}, nil
}

// RawExchange returns the stored upstream request and response for auditing.
func (s *RunService) RawExchange(ctx context.Context, id string) (*dto.RawExchange, error) {
	run, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &dto.RawExchange{RunID: run.ID, Request: run.RawRequest, Response: run.RawResponse}, nil
}

// Get returns a full run detail including terms and events.
func (s *RunService) Get(ctx context.Context, id string) (*dto.RunDetail, error) {
	run, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	terms, err := s.repo.Terms(ctx, id)
	if err != nil {
		return nil, err
	}
	events, err := s.repo.Events(ctx, id)
	if err != nil {
		return nil, err
	}
	detail := toDetail(run, terms, events)
	detail.Artifacts = s.artifactInfos(ctx, id)
	previous, err := s.repo.FindByHash(ctx, run.InputHash, run.Organism, run.ID)
	if err != nil {
		return nil, err
	}
	if previous != nil {
		detail.DuplicateOf = previous.ID
	}
	return detail, nil
}

// List returns a filtered page of runs for the history table.
func (s *RunService) List(ctx context.Context, f repository.RunFilter) (*dto.Paged, error) {
	if f.Limit <= 0 {
		f.Limit = 20
	}
	if f.Limit > 200 {
		f.Limit = 200
	}
	if f.Offset < 0 {
		f.Offset = 0
	}

	runs, total, err := s.repo.List(ctx, f)
	if err != nil {
		return nil, err
	}
	items := make([]dto.RunSummary, 0, len(runs))
	for i := range runs {
		items = append(items, toSummary(&runs[i]))
	}
	return &dto.Paged{Items: items, Total: total, Limit: f.Limit, Offset: f.Offset}, nil
}

// Dashboard aggregates everything the home page needs in one call.
func (s *RunService) Dashboard(ctx context.Context) (*dto.Dashboard, error) {
	stats, err := s.repo.Stats(ctx)
	if err != nil {
		return nil, err
	}
	statuses, err := s.repo.StatusBreakdown(ctx)
	if err != nil {
		return nil, err
	}
	activity, err := s.repo.Activity(ctx, 14, s.now())
	if err != nil {
		return nil, err
	}
	sources, err := s.repo.SourceBreakdown(ctx)
	if err != nil {
		return nil, err
	}
	recent, _, err := s.repo.List(ctx, repository.RunFilter{Limit: 6})
	if err != nil {
		return nil, err
	}
	avg, err := s.repo.AvgDuration(ctx)
	if err != nil {
		return nil, err
	}
	demo, err := s.repo.DemoCount(ctx)
	if err != nil {
		return nil, err
	}
	latestVersion, err := s.repo.LatestDataVersion(ctx)
	if err != nil {
		return nil, err
	}

	byStatus := map[string]int{"queued": 0, "running": 0, "succeeded": 0, "failed": 0, "canceled": 0, "interrupted": 0}
	for _, sc := range statuses {
		byStatus[sc.Status] = sc.Count
	}
	ordered := []string{"succeeded", "failed", "interrupted", "canceled", "running", "queued"}
	statusOut := make([]dto.StatusCount, 0, len(ordered))
	for _, st := range ordered {
		statusOut = append(statusOut, dto.StatusCount{Status: st, Count: byStatus[st]})
	}

	srcOut := make([]dto.SourceCount, 0, len(sources))
	for _, sc := range sources {
		srcOut = append(srcOut, dto.SourceCount{Source: sc.Source, Count: sc.Count})
	}

	actOut := make([]dto.DayCount, 0, len(activity))
	for _, d := range activity {
		actOut = append(actOut, dto.DayCount{Day: d.Day, Count: d.Count})
	}

	recentOut := make([]dto.RunSummary, 0, len(recent))
	for i := range recent {
		recentOut = append(recentOut, toSummary(&recent[i]))
	}

	var successRate float64
	if stats.Total > 0 {
		successRate = float64(stats.Succeeded) / float64(stats.Total)
	}

	return &dto.Dashboard{
		Stats:           dto.Stats{Total: stats.Total, Succeeded: stats.Succeeded, Failed: stats.Failed, Running: stats.Running},
		Statuses:        statusOut,
		Activity:        actOut,
		Sources:         srcOut,
		RecentRuns:      recentOut,
		AvgDurationMS:   avg,
		SuccessRate:     successRate,
		DemoDataPresent: demo > 0,
		Provider: dto.ProviderInfo{
			Name:        "g:Profiler",
			Endpoint:    s.endpoint,
			DataVersion: latestVersion,
			Configured:  s.ready,
			Ready:       s.Ready(),
		},
	}, nil
}

// Report renders a Markdown report from stored rows. It never recomputes
// statistics: every p-value in the file is the value the provider returned.
func (s *RunService) Report(ctx context.Context, id string) (string, error) {
	detail, err := s.Get(ctx, id)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# GenePath 分析报告 · %s\n\n", detail.Label)
	if detail.IsDemo {
		fmt.Fprintf(&b, "> **演示数据**：该记录由 GenePath 内置的样例数据集生成，不代表真实分析结果。\n\n")
	}

	fmt.Fprintf(&b, "## 运行摘要\n\n")
	fmt.Fprintf(&b, "| 字段 | 值 |\n|---|---|\n")
	fmt.Fprintf(&b, "| 运行 ID | `%s` |\n", detail.ID)
	fmt.Fprintf(&b, "| 状态 | %s |\n", statusLabel(detail.Status))
	fmt.Fprintf(&b, "| 物种 | %s (`%s`) |\n", catalog.OrganismLabel(detail.Organism), detail.Organism)
	fmt.Fprintf(&b, "| 输入基因数 | %d |\n", detail.GeneCount)
	fmt.Fprintf(&b, "| 显著条目 | %d |\n", detail.ResultCount)
	fmt.Fprintf(&b, "| 提交时间 | %s |\n", detail.CreatedAt.Format("2006-01-02 15:04:05 MST"))
	if detail.FinishedAt != nil {
		fmt.Fprintf(&b, "| 完成时间 | %s |\n", detail.FinishedAt.Format("2006-01-02 15:04:05 MST"))
		fmt.Fprintf(&b, "| 耗时 | %s |\n", formatDuration(detail.DurationMS))
	}
	fmt.Fprintf(&b, "| 数据源版本 | %s |\n", orDash(detail.DataVersion))
	fmt.Fprintf(&b, "| 输入哈希 (sha256) | `%s` |\n\n", detail.InputHash)

	fmt.Fprintf(&b, "## 运行参数\n\n")
	fmt.Fprintf(&b, "- 显著性阈值：%s\n", detail.Params.Significance)
	fmt.Fprintf(&b, "- 多重检验校正：%s\n", detail.Params.Correction)
	fmt.Fprintf(&b, "- 背景基因集：%s\n", detail.Params.Background)
	fmt.Fprintf(&b, "- 注释来源：%s\n\n", strings.Join(detail.Params.Sources, ", "))

	if len(detail.Terms) > 0 {
		fmt.Fprintf(&b, "## 富集结果（按 p 值升序）\n\n")
		fmt.Fprintf(&b, "| # | 来源 | Term | 名称 | p 值 | 命中/查询 | Term 大小 |\n")
		fmt.Fprintf(&b, "|---|---|---|---|---|---|---|\n")
		for i, t := range detail.Terms {
			fmt.Fprintf(&b, "| %d | %s | `%s` | %s | %.3e | %d/%d | %d |\n",
				i+1, t.Source, t.TermID, t.Name, t.PValue, t.Intersection, t.QuerySize, t.SourceSize)
		}
		fmt.Fprintf(&b, "\n")
	} else {
		fmt.Fprintf(&b, "## 富集结果\n\n该运行没有存储显著富集条目。\n\n")
	}

	fmt.Fprintf(&b, "## 输入基因\n\n")
	if len(detail.Genes) > 0 {
		for _, g := range detail.Genes {
			fmt.Fprintf(&b, "- %s\n", g)
		}
	} else {
		fmt.Fprintf(&b, "_未记录_\n")
	}
	fmt.Fprintf(&b, "\n")

	fmt.Fprintf(&b, "## 方法说明与免责声明\n\n")
	fmt.Fprintf(&b, "- 富集统计由 g:Profiler 计算，GenePath 只负责提交、存储与展示，不重新实现统计方法。\n")
	fmt.Fprintf(&b, "- p 值、Term 大小等字段按数据源返回值原样记录，可用于复现对比。\n")
	fmt.Fprintf(&b, "- 输入哈希用于判定两次提交是否为同一基因集，与顺序和大小写无关。\n")
	fmt.Fprintf(&b, "- 本报告不构成生物学结论，显著条目仍需结合文献与实验验证。\n")

	return b.String(), nil
}

func newRunID() string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("run_%d", time.Now().UnixNano()%1e12)
	}
	return "run_" + hex.EncodeToString(buf)
}

func toSummary(run *model.Run) dto.RunSummary {
	return dto.RunSummary{
		ID:            run.ID,
		Label:         run.Label,
		Status:        run.Status,
		Outcome:       run.Outcome,
		Organism:      run.Organism,
		GeneCount:     run.GeneCount,
		MappedCount:   run.MappedCount,
		ResultCount:   run.ResultCount,
		TermCount:     run.TermCount,
		InputHash:     run.InputHash,
		DurationMS:    run.DurationMS,
		ProviderMS:    run.ProviderMS,
		DataVersion:   run.DataVersion,
		HTTPStatus:    run.HTTPStatus,
		RequestBytes:  run.RequestBytes,
		ResponseBytes: run.ResponseBytes,
		ErrorCode:     run.ErrorCode,
		ErrorMessage:  run.ErrorMessage,
		IsDemo:        run.IsDemo,
		Params: dto.Params{
			Significance: run.Params.Significance,
			Correction:   run.Params.Correction,
			Background:   run.Params.Background,
			Sources:      run.Params.Sources,
		},
		CreatedAt:  run.CreatedAt,
		StartedAt:  run.StartedAt,
		FinishedAt: run.FinishedAt,
	}
}

func toDetail(run *model.Run, terms []model.Term, events []model.RunEvent) *dto.RunDetail {
	detail := &dto.RunDetail{
		RunSummary: toSummary(run),
		Genes:      run.GeneList(),
		Unmapped:   run.UnmappedList(),
		Duplicates: run.DuplicateList(),
		Terms:      make([]dto.Term, 0, len(terms)),
		Events:     make([]dto.Event, 0, len(events)),
		Thresholds: map[string]dto.Threshold{},
	}
	if run.Thresholds != "" {
		_ = json.Unmarshal([]byte(run.Thresholds), &detail.Thresholds)
	}
	for i := range terms {
		t := terms[i]
		detail.Terms = append(detail.Terms, dto.Term{
			Source:          t.Source,
			TermID:          t.TermID,
			Name:            t.Name,
			Description:     t.Description,
			PValue:          t.PValue,
			SourceSize:      t.SourceSize,
			QuerySize:       t.QuerySize,
			Intersection:    t.Intersection,
			EffectiveDomain: t.EffectiveDomain,
			Significant:     t.Significant,
			GroupID:         t.GroupID,
			Genes:           t.GeneList(),
			Rank:            t.Rank,
		})
	}
	for _, e := range events {
		detail.Events = append(detail.Events, dto.Event{Kind: e.Kind, Message: e.Message, At: e.At})
	}
	if detail.Genes == nil {
		detail.Genes = []string{}
	}
	if detail.Unmapped == nil {
		detail.Unmapped = []string{}
	}
	if detail.Duplicates == nil {
		detail.Duplicates = []string{}
	}
	return detail
}

// mappingMessage summarises how many submitted symbols the provider recognised.
func mappingMessage(submitted, mapped int, unmapped []string) string {
	msg := fmt.Sprintf("映射 %d/%d 个基因到注释库", mapped, submitted)
	if len(unmapped) > 0 {
		shown := unmapped
		if len(shown) > 8 {
			shown = shown[:8]
		}
		msg += "；未映射：" + strings.Join(shown, ", ")
		if len(unmapped) > len(shown) {
			msg += fmt.Sprintf(" 等 %d 个", len(unmapped))
		}
	}
	return msg
}

func valueOr(t *time.Time, fallback time.Time) time.Time {
	if t == nil {
		return fallback
	}
	return *t
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

func statusLabel(status string) string {
	switch status {
	case model.StatusQueued:
		return "排队中"
	case model.StatusRunning:
		return "运行中"
	case model.StatusSucceeded:
		return "已完成"
	case model.StatusFailed:
		return "失败"
	case model.StatusCanceled:
		return "已取消"
	case model.StatusInterrupted:
		return "已中断"
	}
	return status
}

func formatDuration(ms int64) string {
	if ms <= 0 {
		return "—"
	}
	if ms < 1000 {
		return fmt.Sprintf("%d ms", ms)
	}
	return fmt.Sprintf("%.1f s", float64(ms)/1000)
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
