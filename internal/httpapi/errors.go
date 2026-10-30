// Package httpapi is the transport layer: it maps HTTP requests onto the
// service layer and renders a single, consistent JSON error envelope.
package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"genepath/internal/dto"
	"genepath/internal/repository"
	"genepath/internal/service"
)

// Error codes used by the frontend to branch on failures without parsing
// human-readable messages.
const (
	CodeValidationFailed = "validation_failed"
	CodeNotFound         = "not_found"
	CodeBadRequest       = "bad_request"
	CodeInternal         = "internal_error"
	CodeNotReady         = "executor_not_ready"
	CodeQueueFull        = "queue_full"
	CodeUnknownArtifact  = "unknown_artifact"
)

// respondProblem writes the standard error envelope.
func respondProblem(c *gin.Context, status int, code, message string, details any) {
	c.JSON(status, dto.Problem{Code: code, Message: message, Details: details})
}

// respondError translates a service/repository error into an HTTP status.
func respondError(c *gin.Context, err error) {
	var verr *service.ValidationError
	if errors.As(err, &verr) {
		respondProblem(c, http.StatusUnprocessableEntity, CodeValidationFailed, verr.Error(), verr.Fields)
		return
	}
	if errors.Is(err, repository.ErrNotFound) {
		respondProblem(c, http.StatusNotFound, CodeNotFound, "任务不存在", nil)
		return
	}
	if errors.Is(err, service.ErrNotReady) {
		respondProblem(c, http.StatusServiceUnavailable, CodeNotReady, err.Error(), nil)
		return
	}
	if errors.Is(err, service.ErrQueueFull) {
		respondProblem(c, http.StatusTooManyRequests, CodeQueueFull, err.Error(), nil)
		return
	}
	if errors.Is(err, service.ErrUnknownArtifact) {
		respondProblem(c, http.StatusBadRequest, CodeUnknownArtifact, "不支持的文件类型", nil)
		return
	}
	respondProblem(c, http.StatusInternalServerError, CodeInternal, err.Error(), nil)
}
