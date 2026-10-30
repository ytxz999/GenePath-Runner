package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"genepath/internal/model"
	"genepath/internal/service"
)

// reportCP allows the self-contained report to style itself while blocking
// every network fetch, so an archived report can be opened offline.
const reportCP = "default-src 'none'; style-src 'unsafe-inline'; img-src data:; base-uri 'none'; form-action 'none'"

// reportHTML serves the standalone HTML report for one run.
func (a *API) reportHTML(c *gin.Context) {
	a.serveArtifact(c, model.ArtifactHTML, "inline")
}

// manifestJSON serves manifest.json as a download.
func (a *API) manifestJSON(c *gin.Context) {
	a.serveArtifact(c, model.ArtifactManifest, "attachment")
}

// listArtifacts returns every stored file for a run, with its hash and size.
func (a *API) listArtifacts(c *gin.Context) {
	items, err := a.svc.Artifacts(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// downloadArtifact serves one stored file by kind.
func (a *API) downloadArtifact(c *gin.Context) {
	kind := c.Param("kind")
	if _, known := artifactKinds[kind]; !known {
		respondError(c, service.ErrUnknownArtifact)
		return
	}
	disposition := "attachment"
	if kind == model.ArtifactHTML {
		disposition = "inline"
	}
	a.serveArtifact(c, kind, disposition)
}

// serveArtifact is the shared body of every artifact download.
func (a *API) serveArtifact(c *gin.Context, kind, disposition string) {
	id := c.Param("id")
	body, err := a.svc.ArtifactBytes(c.Request.Context(), id, kind)
	if err != nil {
		respondError(c, err)
		return
	}
	name := service.ArtifactFileName(kind)
	c.Header("Content-Type", service.ArtifactContentType(kind))
	c.Header("Content-Disposition", disposition+`; filename="genepath-`+id+`-`+name+`"`)
	if kind == model.ArtifactHTML {
		c.Header("Content-Security-Policy", reportCP)
	}
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, service.ArtifactContentType(kind), body)
}

// rerun submits a fresh analysis with the same genes and parameters as a
// historical run. The original record is never overwritten.
func (a *API) rerun(c *gin.Context) {
	detail, err := a.svc.Rerun(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.Header("Location", "/api/v1/runs/"+detail.ID)
	c.JSON(http.StatusCreated, detail)
}

// artifactKinds is the allow-list for the generic download route.
var artifactKinds = map[string]struct{}{
	model.ArtifactRequest:  {},
	model.ArtifactResponse: {},
	model.ArtifactManifest: {},
	model.ArtifactHTML:     {},
	model.ArtifactMarkdown: {},
}
