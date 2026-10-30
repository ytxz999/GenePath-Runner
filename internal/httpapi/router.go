package httpapi

import (
	"io/fs"
	"net/http"
	"path"
	"time"

	"github.com/gin-gonic/gin"

	"genepath/internal/repository"
	"genepath/internal/service"
	"genepath/web"
)

// RouterOptions configures the HTTP surface.
type RouterOptions struct {
	Service   *service.RunService
	Repo      *repository.RunRepository
	Version   string
	Dev       bool
	PageFiles fs.FS
}

// NewRouter builds the complete routing table: JSON API under /v1, probes at
// the root, and the static frontend everywhere else.
func NewRouter(api *API, opts RouterOptions) *gin.Engine {
	if opts.Dev {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(requestID())
	r.Use(recoverPanic())
	r.Use(accessLog(opts.Dev))
	r.Use(securityHeaders(opts.Dev))

	// Static assets are served from the embedded filesystem.
	assets, err := fs.Sub(web.Static, "static/assets")
	if err != nil {
		panic("web: missing static/assets directory: " + err.Error())
	}

	pages, err := fs.Sub(web.Static, "static")
	if err != nil {
		panic("web: missing static directory: " + err.Error())
	}

	r.GET("/assets/*filepath", func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=600")
		fileServer := http.StripPrefix("/assets/", http.FileServer(http.FS(assets)))
		fileServer.ServeHTTP(c.Writer, c.Request)
	})

	// The public API namespace is /api/v1. /v1 stays as an alias so older
	// bookmarked links and the phase-1 frontend keep working.
	registerAPI := func(g *gin.RouterGroup) {
		g.GET("/meta", api.meta)
		g.GET("/dashboard", api.dashboard)
		g.GET("/runs", api.listRuns)
		g.POST("/runs", api.createRun)
		g.POST("/runs/validate", api.validateRun)
		g.GET("/runs/:id", api.getRun)
		g.GET("/runs/:id/results", api.getResults)
		g.GET("/runs/:id/raw", api.getRaw)
		g.GET("/runs/:id/report.md", api.reportMarkdown)
		g.DELETE("/demo", api.deleteDemo)
	}
	registerAPI(r.Group("/api/v1"))
	registerAPI(r.Group("/v1"))

	r.GET("/healthz", api.healthz)
	r.GET("/readyz", api.readyz)

	// Pages. Each route names its file explicitly so a typo fails loudly at
	// request time instead of silently returning the wrong screen.
	page := func(file string) gin.HandlerFunc {
		return func(c *gin.Context) {
			c.Header("Cache-Control", "no-store")
			c.Header("Content-Type", "text/html; charset=utf-8")
			body, err := fs.ReadFile(pages, file)
			if err != nil {
				c.String(http.StatusInternalServerError, "missing page: %s", file)
				return
			}
			c.Writer.WriteHeader(http.StatusOK)
			_, _ = c.Writer.Write(body)
		}
	}

	r.GET("/", page("index.html"))
	r.GET("/index.html", page("index.html"))
	r.GET("/new", page("new.html"))
	r.GET("/history", page("history.html"))
	r.GET("/runs/:id", page("results.html"))

	r.NoRoute(func(c *gin.Context) {
		if len(c.Request.URL.Path) >= 4 && c.Request.URL.Path[:4] == "/v1/" {
			respondProblem(c, http.StatusNotFound, CodeNotFound, "接口不存在", nil)
			return
		}
		if len(c.Request.URL.Path) >= 8 && c.Request.URL.Path[:8] == "/api/v1/" {
			respondProblem(c, http.StatusNotFound, CodeNotFound, "接口不存在", nil)
			return
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusNotFound, notFoundPage)
	})

	return r
}

// requestID tags every request so client and server logs can be correlated.
func requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-Id")
		if id == "" {
			id = "req_" + time.Now().Format("150405.000000")
		}
		c.Header("X-Request-Id", id)
		c.Set("request_id", id)
		c.Next()
	}
}

// recoverPanic converts a panic into a 500 instead of killing the process.
func recoverPanic() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				respondProblem(c, http.StatusInternalServerError, CodeInternal, "服务器内部错误", nil)
				c.Abort()
			}
		}()
		c.Next()
	}
}

// accessLog writes one line per request in a format that is easy to grep.
func accessLog(dev bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		line := time.Now().Format("15:04:05") + " " +
			c.Request.Method + " " + path.Clean(c.Request.URL.Path) + " " +
			statusText(c.Writer.Status()) + " " +
			time.Since(start).Round(time.Microsecond).String()
		if dev && c.Writer.Status() >= 400 {
			line += "  request_id=" + c.GetString("request_id")
		}
		println(line)
	}
}

func statusText(code int) string {
	switch {
	case code >= 500:
		return "500"
	case code >= 400:
		return "4xx"
	default:
		return "200"
	}
}

// securityHeaders sets a conservative baseline. The CSP allows only local
// assets, which is why the frontend vendors its fonts and scripts.
func securityHeaders(dev bool) gin.HandlerFunc {
	scriptSrc := "'self'"
	styleSrc := "'self'"
	if dev {
		// A live-reload helper or browser extension may inject inline styles in
		// development; production stays strict.
		styleSrc = "'self' 'unsafe-inline'"
	}
	csp := "default-src 'self'; img-src 'self' data:; style-src " + styleSrc +
		"; script-src " + scriptSrc + "; connect-src 'self'; font-src 'self'; base-uri 'none'; form-action 'self'"

	return func(c *gin.Context) {
		c.Header("Content-Security-Policy", csp)
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "same-origin")
		c.Header("X-Frame-Options", "DENY")
		c.Next()
	}
}

const notFoundPage = `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><title>页面不存在 · GenePath</title>
<style>body{font-family:system-ui,sans-serif;background:#f6f7f9;color:#1b2436;display:grid;place-items:center;height:100vh;margin:0}
.card{background:#fff;border:1px solid #e3e6ec;border-radius:14px;padding:40px 48px;box-shadow:0 8px 30px rgba(20,30,60,.06);text-align:center}
h1{margin:0 0 8px;font-size:20px}p{margin:0 0 20px;color:#5b6577;font-size:14px}
a{color:#2547b8;text-decoration:none;font-weight:600}</style></head>
<body><div class="card"><h1>404 · 页面不存在</h1><p>你访问的地址没有对应的页面。</p><a href="/">返回工作台</a></div></body></html>`
