package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"genepath/internal/model"
)

// ErrNotFound is returned when a run id does not exist.
var ErrNotFound = errors.New("run not found")

// ErrNotQueued is returned when a worker tries to start a run that is no longer
// queued (for example because the server restarted mid-flight).
var ErrNotQueued = errors.New("run is not queued")

// RunFilter narrows a history query. Zero values mean "no filter".
type RunFilter struct {
	Status   string
	Query    string
	Limit    int
	Offset   int
	Organism string
}

// DayCount is one bar in the dashboard activity chart.
type DayCount struct {
	Day   string `json:"day"`
	Count int    `json:"count"`
}

// StatusCount is one slice of the status breakdown.
type StatusCount struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

// SourceCount is one slice of the annotation-source breakdown.
type SourceCount struct {
	Source string `json:"source"`
	Count  int    `json:"count"`
}

// Stats are the dashboard headline numbers.
type Stats struct {
	Total     int64 `json:"total"`
	Succeeded int64 `json:"succeeded"`
	Failed    int64 `json:"failed"`
	Running   int64 `json:"running"`
}

// RunRepository is the only place that talks SQL for runs.
type RunRepository struct {
	db *gorm.DB
}

// NewRunRepository wires a repository to an open database handle.
func NewRunRepository(db *gorm.DB) *RunRepository {
	return &RunRepository{db: db}
}

// Create inserts a run together with its events in one transaction.
func (r *RunRepository) Create(ctx context.Context, run *model.Run, events []model.RunEvent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(run).Error; err != nil {
			return err
		}
		if len(events) == 0 {
			return nil
		}
		for i := range events {
			events[i].RunID = run.ID
		}
		return tx.Create(&events).Error
	})
}

// Get loads a run by id.
func (r *RunRepository) Get(ctx context.Context, id string) (*model.Run, error) {
	var run model.Run
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// List returns a page of runs plus the total matching count.
func (r *RunRepository) List(ctx context.Context, f RunFilter) ([]model.Run, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.Run{})
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Organism != "" {
		q = q.Where("organism = ?", f.Organism)
	}
	if f.Query != "" {
		like := "%" + f.Query + "%"
		q = q.Where("label LIKE ? OR input_hash LIKE ? OR id LIKE ?", like, like, like)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	var runs []model.Run
	err := q.Order("created_at DESC").Limit(limit).Offset(f.Offset).Find(&runs).Error
	return runs, total, err
}

// Terms returns the stored enrichment rows for a run, ordered by rank.
func (r *RunRepository) Terms(ctx context.Context, runID string) ([]model.Term, error) {
	var terms []model.Term
	err := r.db.WithContext(ctx).Where("run_id = ?", runID).Order("rank ASC").Find(&terms).Error
	return terms, err
}

// Events returns the audit trail for a run in chronological order.
func (r *RunRepository) Events(ctx context.Context, runID string) ([]model.RunEvent, error) {
	var events []model.RunEvent
	err := r.db.WithContext(ctx).Where("run_id = ?", runID).Order("at ASC").Find(&events).Error
	return events, err
}

// MarkRunning records that a worker picked the run up.
func (r *RunRepository) MarkRunning(ctx context.Context, id string, at time.Time, event model.RunEvent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.Run{}).
			Where("id = ? AND status = ?", id, model.StatusQueued).
			Updates(map[string]any{"status": model.StatusRunning, "started_at": at})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// Already picked up (or cancelled) by someone else.
			return ErrNotQueued
		}
		event.RunID = id
		return tx.Create(&event).Error
	})
}

// ResetInFlight runs at startup. A run left in "running" can only mean the
// previous process died mid-call, so it is marked interrupted instead of being
// silently stuck forever. Runs still "queued" were never started and are handed
// back to the caller so main can re-submit them to the fresh worker pool.
func (r *RunRepository) ResetInFlight(ctx context.Context, at time.Time) ([]string, error) {
	var queued []string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Run{}).
			Where("status = ?", model.StatusRunning).
			Updates(map[string]any{
				"status":        model.StatusInterrupted,
				"error_code":    model.ErrInterrupted,
				"error_message": "服务重启时该任务仍在执行，已标记为中断；请重新提交。",
				"finished_at":   at,
			}).Error; err != nil {
			return err
		}
		return tx.Model(&model.Run{}).
			Where("status = ?", model.StatusQueued).
			Order("created_at ASC").
			Pluck("id", &queued).Error
	})
	return queued, err
}

// Finish stores the terminal state of a run together with its enrichment rows
// and audit events in one transaction, so a partially written result set can
// never be read by the UI.
func (r *RunRepository) Finish(ctx context.Context, run *model.Run, terms []model.Term, events []model.RunEvent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Model(&model.Run{}).Where("id = ?", run.ID).Updates(map[string]any{
			"status":          run.Status,
			"outcome":         run.Outcome,
			"error_code":      run.ErrorCode,
			"error_message":   run.ErrorMessage,
			"raw_request":     run.RawRequest,
			"raw_response":    run.RawResponse,
			"request_bytes":   run.RequestBytes,
			"response_bytes":  run.ResponseBytes,
			"http_status":     run.HTTPStatus,
			"provider_ms":     run.ProviderMS,
			"data_version":    run.DataVersion,
			"mapped_count":    run.MappedCount,
			"unmapped_genes":  run.UnmappedGenes,
			"duplicate_genes": run.DuplicateGenes,
			"thresholds":      run.Thresholds,
			"result_count":    run.ResultCount,
			"term_count":      run.TermCount,
			"duration_ms":     run.DurationMS,
			"finished_at":     run.FinishedAt,
		}).Error
		if err != nil {
			return err
		}

		if err := tx.Where("run_id = ?", run.ID).Delete(&model.Term{}).Error; err != nil {
			return err
		}
		if len(terms) > 0 {
			for i := range terms {
				terms[i].RunID = run.ID
			}
			if err := tx.Create(&terms).Error; err != nil {
				return err
			}
		}
		if len(events) > 0 {
			for i := range events {
				events[i].RunID = run.ID
			}
			return tx.Create(&events).Error
		}
		return nil
	})
}

// Stats aggregates the dashboard headline numbers in one round trip.
func (r *RunRepository) Stats(ctx context.Context) (Stats, error) {
	type row struct {
		Status string
		Count  int64
	}
	var rows []row
	err := r.db.WithContext(ctx).Model(&model.Run{}).
		Select("status, count(*) as count").Group("status").Scan(&rows).Error
	if err != nil {
		return Stats{}, err
	}

	var s Stats
	for _, row := range rows {
		s.Total += row.Count
		switch row.Status {
		case model.StatusSucceeded:
			s.Succeeded = row.Count
		case model.StatusFailed:
			s.Failed = row.Count
		case model.StatusRunning, model.StatusQueued:
			s.Running += row.Count
		}
	}
	return s, nil
}

// StatusBreakdown returns every status with its count, including zero counts
// filled in by the caller if it needs a stable legend.
func (r *RunRepository) StatusBreakdown(ctx context.Context) ([]StatusCount, error) {
	var out []StatusCount
	err := r.db.WithContext(ctx).Model(&model.Run{}).
		Select("status, count(*) as count").Group("status").Scan(&out).Error
	return out, err
}

// Activity returns per-day run counts for the last n days, oldest first. Days
// with no runs are returned with Count 0 so the chart never has holes.
func (r *RunRepository) Activity(ctx context.Context, days int, now time.Time) ([]DayCount, error) {
	if days <= 0 {
		days = 14
	}
	start := now.AddDate(0, 0, -(days - 1)).Truncate(24 * time.Hour)

	type row struct {
		Day   string
		Count int
	}
	var rows []row
	err := r.db.WithContext(ctx).Model(&model.Run{}).
		Select("substr(created_at, 1, 10) as day, count(*) as count").
		Where("created_at >= ?", start).
		Group("day").Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int, len(rows))
	for _, row := range rows {
		counts[row.Day] = row.Count
	}

	out := make([]DayCount, 0, days)
	for i := 0; i < days; i++ {
		d := start.AddDate(0, 0, i).Format("2006-01-02")
		out = append(out, DayCount{Day: d, Count: counts[d]})
	}
	return out, nil
}

// RecentSucceeded returns the most recent successful runs for the dashboard.
func (r *RunRepository) RecentSucceeded(ctx context.Context, limit int) ([]model.Run, error) {
	var runs []model.Run
	err := r.db.WithContext(ctx).
		Where("status = ?", model.StatusSucceeded).
		Order("created_at DESC").Limit(limit).Find(&runs).Error
	return runs, err
}

// HasAny reports whether the runs table already contains data. The seeder uses
// it so restarting the server never duplicates demo rows.
func (r *RunRepository) HasAny(ctx context.Context) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Run{}).Limit(1).Count(&count).Error
	return count > 0, err
}

// DemoCount returns how many demo rows are currently stored, which lets the UI
// show a persistent "demo dataset" banner instead of a one-off toast.
func (r *RunRepository) DemoCount(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Run{}).Where("is_demo = ?", true).Count(&count).Error
	return count, err
}

// DeleteDemo removes every row flagged as demo data (terms and events cascade
// through the foreign keys) and returns how many runs were removed.
func (r *RunRepository) DeleteDemo(ctx context.Context) (int64, error) {
	var deleted int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ids []string
		if err := tx.Model(&model.Run{}).Where("is_demo = ?", true).Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		if err := tx.Where("run_id IN ?", ids).Delete(&model.Term{}).Error; err != nil {
			return err
		}
		if err := tx.Where("run_id IN ?", ids).Delete(&model.RunEvent{}).Error; err != nil {
			return err
		}
		res := tx.Where("id IN ?", ids).Delete(&model.Run{})
		deleted = res.RowsAffected
		return res.Error
	})
	return deleted, err
}

// LatestDataVersion returns the annotation release reported by the most recent
// successful run, so the dashboard can show what the local data was measured on.
func (r *RunRepository) LatestDataVersion(ctx context.Context) (string, error) {
	var version string
	err := r.db.WithContext(ctx).Model(&model.Run{}).
		Select("data_version").
		Where("status = ? AND data_version <> ''", model.StatusSucceeded).
		Order("finished_at DESC").Limit(1).
		Row().Scan(&version)
	if err != nil {
		return "", nil // no successful run yet is not an error
	}
	return version, nil
}

// SourceBreakdown counts stored enrichment rows per annotation source.
func (r *RunRepository) SourceBreakdown(ctx context.Context) ([]SourceCount, error) {
	var out []SourceCount
	err := r.db.WithContext(ctx).Model(&model.Term{}).
		Select("source, count(*) as count").Group("source").
		Order("count DESC").Scan(&out).Error
	return out, err
}

// AvgDuration returns the mean duration of successful runs in milliseconds.
func (r *RunRepository) AvgDuration(ctx context.Context) (int64, error) {
	var avg float64
	row := r.db.WithContext(ctx).Model(&model.Run{}).
		Select("COALESCE(AVG(duration_ms), 0)").
		Where("status = ? AND duration_ms > 0", model.StatusSucceeded).
		Row()
	if err := row.Scan(&avg); err != nil {
		return 0, err
	}
	return int64(avg), nil
}

// FindByHash returns the most recent run with the same content hash, excluding
// the run identified by excludeID. This is what powers the "same input already
// ran" notice on the new-analysis page.
func (r *RunRepository) FindByHash(ctx context.Context, hash, organism, excludeID string) (*model.Run, error) {
	var run model.Run
	err := r.db.WithContext(ctx).
		Where("input_hash = ? AND organism = ? AND id <> ?", hash, organism, excludeID).
		Order("created_at DESC").First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}
