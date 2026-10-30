// Package database owns the SQLite connection, schema migration and the
// explicitly-labelled demo dataset used by the UI during phase 1.
package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"genepath/internal/model"
)

// Open connects to SQLite, applies pragmas, migrates the schema and — when
// asked — seeds the demo dataset. The driver is the pure-Go glebarez driver so
// the project builds on Windows without a C toolchain.
func Open(path string, seedDemo bool) (*gorm.DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}

	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	for _, pragma := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA foreign_keys = ON",
	} {
		if err := db.Exec(pragma).Error; err != nil {
			return nil, fmt.Errorf("apply %q: %w", pragma, err)
		}
	}

	if err := Migrate(db); err != nil {
		return nil, err
	}

	if seedDemo {
		if err := SeedDemo(db); err != nil {
			return nil, fmt.Errorf("seed demo dataset: %w", err)
		}
	}
	return db, nil
}

// Migrate creates or updates every table. It is safe to call on every boot.
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(&model.Run{}, &model.Term{}, &model.RunEvent{}, &model.Artifact{}); err != nil {
		return fmt.Errorf("migrate schema: %w", err)
	}
	return nil
}

// Ping verifies the connection is usable; used by /healthz and /readyz.
func Ping(ctx context.Context, db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// Close releases the underlying connection pool.
func Close(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// waitForWrite retries a write while the database is briefly locked, which can
// happen when the seeder runs at the same moment as a request.
func waitForWrite(fn func() error) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if err = fn(); err == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return err
}
