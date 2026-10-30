// Command server runs the GenePath Runner HTTP service.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"genepath/internal/config"
	"genepath/internal/database"
	"genepath/internal/executor"
	"genepath/internal/httpapi"
	"genepath/internal/provider/gprofiler"
	"genepath/internal/repository"
	"genepath/internal/service"
)

func main() {
	cfg := config.Load()

	var (
		addr     = flag.String("addr", envOr("GENEPATH_ADDR", cfg.Addr), "HTTP 监听地址")
		dbPath   = flag.String("db", envOr("GENEPATH_DB", cfg.DBPath), "SQLite 数据库文件路径")
		seed     = flag.Bool("seed", cfg.SeedDemo, "首次启动时写入演示数据集")
		dev      = flag.Bool("dev", false, "开发模式：更长的日志与更宽松的 CSP")
		version  = flag.String("version", "0.2.0", "版本号，显示在页脚与健康检查中")
		endpoint = flag.String("provider-endpoint", envOr("GPROFILER_ENDPOINT", gprofiler.DefaultEndpoint), "g:Profiler 接口地址")
		workers  = flag.Int("workers", envInt("GENEPATH_WORKERS", 2), "并行执行分析任务的 worker 数量")
		timeout  = flag.Duration("provider-timeout", envDuration("GENEPATH_PROVIDER_TIMEOUT", 90*time.Second), "单个分析任务调用 g:Profiler 的超时时间")
	)
	flag.Parse()

	if err := run(*addr, *dbPath, *seed, *dev, *version, *endpoint, *workers, *timeout); err != nil {
		log.Fatalf("genepath: %v", err)
	}
}

func run(addr, dbPath string, seed, dev bool, version, endpoint string, workers int, timeout time.Duration) error {
	started := time.Now()

	db, err := database.Open(dbPath, seed)
	if err != nil {
		return err
	}
	defer func() { _ = database.Close(db) }()

	repo := repository.NewRunRepository(db)
	svc := service.NewRunService(repo, service.Options{
		ProviderEndpoint: endpoint,
		ProviderReady:    true,
		Client:           gprofiler.New(endpoint),
		ExecTimeout:      timeout,
	})

	// The queue owns the background workers; the service only knows how to
	// submit an id, which keeps the HTTP path free of goroutines.
	queue := executor.New(svc, workers, workers*8, timeout+15*time.Second)
	svc.AttachQueue(queue.Submit)

	recovered, err := repo.ResetInFlight(context.Background(), time.Now())
	if err != nil {
		return err
	}
	for _, id := range recovered {
		_ = queue.Submit(id)
	}
	if len(recovered) > 0 {
		log.Printf("恢复 %d 个重启前未开始的任务", len(recovered))
	}

	api := httpapi.NewAPI(db, svc, repo, version, endpoint, started)
	router := httpapi.NewRouter(api, httpapi.RouterOptions{Version: version, Dev: dev})

	srv := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	url := "http://" + addr
	if addr == ":8080" || addr == "0.0.0.0:8080" {
		url = "http://127.0.0.1:8080"
	}
	printBanner(url, dbPath, version)

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		if errors.Is(err, syscall.EADDRINUSE) || isAddrInUse(err) {
			return fmt.Errorf("端口 %s 已被占用，请用 -addr 指定其他端口", addr)
		}
		return err
	case <-ctx.Done():
	}

	log.Println("收到退出信号，正在关闭服务…")
	queueCtx, queueCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer queueCancel()
	_ = queue.Shutdown(queueCtx)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func isAddrInUse(err error) bool {
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return errors.Is(netErr.Err, syscall.EADDRINUSE)
	}
	return false
}

func printBanner(url, dbPath, version string) {
	fmt.Println()
	fmt.Println("  GenePath Runner  v" + version)
	fmt.Println("  ────────────────────────────────")
	fmt.Println("  工作台   " + url + "/")
	fmt.Println("  新建分析 " + url + "/new")
	fmt.Println("  运行历史 " + url + "/history")
	fmt.Println("  数据库   " + dbPath)
	fmt.Println("  健康检查 " + url + "/healthz")
	fmt.Println()
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
