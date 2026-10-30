package httpapi

import (
	"time"

	"gorm.io/gorm"

	"genepath/internal/repository"
	"genepath/internal/service"
)

// NewAPI builds the handler set. Keeping construction here (rather than in
// main) means tests can build the exact same object graph.
func NewAPI(db *gorm.DB, svc *service.RunService, repo *repository.RunRepository, version, endpoint string, started time.Time) *API {
	return &API{
		svc:      svc,
		repo:     repo,
		db:       db,
		version:  version,
		endpoint: endpoint,
		started:  started,
	}
}
