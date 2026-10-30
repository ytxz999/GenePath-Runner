package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"genepath/internal/catalog"
	"genepath/internal/database"
	"genepath/internal/gene"
	"genepath/internal/repository"
	"genepath/internal/service"
)

// API holds the dependencies every handler needs.
type API struct {
	svc      *service.RunService
	repo     *repository.RunRepository
	db       *gorm.DB
	version  string
	endpoint string
	started  time.Time
}

// HealthResponse is the /healthz payload.
type HealthResponse struct {
	Status   string `json:"status"`
	Version  string `json:"version"`
	UptimeMS int64  `json:"uptimeMs"`
	Database string `json:"database"`
}

// healthz is a liveness probe: it answers as long as the process can serve
// HTTP. Database state is reported but does not fail the probe.
func (a *API) healthz(c *gin.Context) {
	dbState := "ok"
	ctx, cancel := context.WithTimeout(c.Request.Context(), 1500*time.Millisecond)
	defer cancel()
	if err := database.Ping(ctx, a.db); err != nil {
		dbState = "error: " + err.Error()
	}
	c.JSON(http.StatusOK, HealthResponse{
		Status:   "ok",
		Version:  a.version,
		UptimeMS: time.Since(a.started).Milliseconds(),
		Database: dbState,
	})
}

// readyz is a readiness probe: it fails when the database is unreachable.
func (a *API) readyz(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 1500*time.Millisecond)
	defer cancel()
	if err := database.Ping(ctx, a.db); err != nil {
		respondProblem(c, http.StatusServiceUnavailable, "not_ready", "数据库不可用", nil)
		return
	}
	// A run cannot make progress without the worker pool, so a pod whose
	// executor failed to start must not be sent traffic.
	if !a.svc.Ready() {
		respondProblem(c, http.StatusServiceUnavailable, "not_ready", "分析执行器尚未就绪", gin.H{"executor": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready", "executor": true})
}

// meta returns every enum the frontend may send, so validation lives in one
// place instead of being duplicated in JavaScript.
func (a *API) meta(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"organisms":   catalog.Organisms,
		"sources":     catalog.Sources,
		"corrections": catalog.Corrections,
		"defaults": gin.H{
			"organism":     catalog.DefaultOrganism,
			"significance": catalog.DefaultSignificance,
			"correction":   catalog.DefaultCorrection,
			"background":   catalog.DefaultBackground,
			"sources":      []string{catalog.DefaultSource},
		},
		"maxGenes": gene.MaxGenes,
		"provider": gin.H{
			"name":          "g:Profiler",
			"endpoint":      a.endpoint,
			"executorReady": a.svc.Ready(),
		},
		"appVersion": a.version,
	})
}

// dashboard returns the home page aggregate.
func (a *API) dashboard(c *gin.Context) {
	data, err := a.svc.Dashboard(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, data)
}

// listRuns returns a filtered page of runs.
func (a *API) listRuns(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	page, err := a.svc.List(c.Request.Context(), repository.RunFilter{
		Status:   statusFilter(c.Query("status")),
		Query:    strings.TrimSpace(c.Query("q")),
		Organism: strings.TrimSpace(c.Query("organism")),
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, page)
}

// createRun accepts a gene list and stores a queued run.
func (a *API) createRun(c *gin.Context) {
	var in service.CreateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		respondProblem(c, http.StatusBadRequest, CodeBadRequest, "请求体不是合法的 JSON", nil)
		return
	}
	detail, err := a.svc.Create(c.Request.Context(), in)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, detail)
}

// validateRun gives the form live feedback without writing anything.
func (a *API) validateRun(c *gin.Context) {
	var in service.CreateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		respondProblem(c, http.StatusBadRequest, CodeBadRequest, "请求体不是合法的 JSON", nil)
		return
	}
	res, err := a.svc.Validate(c.Request.Context(), in)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// getRun returns one run with its terms and audit trail.
func (a *API) getRun(c *gin.Context) {
	detail, err := a.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, detail)
}

// getResults returns the enrichment table plus the provenance the UI shows.
func (a *API) getResults(c *gin.Context) {
	res, err := a.svc.Results(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, res)
}

// getRaw returns the exact upstream request and response, for auditing.
func (a *API) getRaw(c *gin.Context) {
	raw, err := a.svc.RawExchange(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, raw)
}

// deleteDemo removes the seeded demo rows.
func (a *API) deleteDemo(c *gin.Context) {
	deleted, err := a.svc.ClearDemo(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}

// reportMarkdown streams a Markdown report rendered from stored rows.
func (a *API) reportMarkdown(c *gin.Context) {
	id := c.Param("id")
	body, err := a.svc.Report(c.Request.Context(), id)
	if err != nil {
		respondError(c, err)
		return
	}
	c.Header("Content-Type", "text/markdown; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="genepath-`+id+`.md"`)
	c.String(http.StatusOK, body)
}

func statusFilter(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case "queued", "running", "succeeded", "failed", "canceled", "interrupted":
		return s
	}
	return ""
}
