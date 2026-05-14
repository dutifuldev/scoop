package db

import (
	"context"
	"testing"

	"gorm.io/gorm/logger"
)

func TestResolveGormLogLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		appLogLevel string
		environment string
		want        logger.LogLevel
	}{
		{name: "debug enables info SQL logs", appLogLevel: "debug", want: logger.Info},
		{name: "empty defaults to warnings", appLogLevel: "", want: logger.Warn},
		{name: "warning uses warnings", appLogLevel: "warning", want: logger.Warn},
		{name: "error uses errors", appLogLevel: "error", want: logger.Error},
		{name: "silent disables SQL logs", appLogLevel: "silent", want: logger.Silent},
		{name: "unknown local stays warning", appLogLevel: "custom", environment: "local", want: logger.Warn},
		{name: "unknown nonlocal uses errors", appLogLevel: "custom", environment: "production", want: logger.Error},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := resolveGormLogLevel(tt.appLogLevel, tt.environment); got != tt.want {
				t.Fatalf("resolveGormLogLevel(%q, %q) = %v, want %v", tt.appLogLevel, tt.environment, got, tt.want)
			}
		})
	}
}

func TestPoolAccessorsIntegration(t *testing.T) {
	pool := newIntegrationPool(t)

	sqlDB := pool.DB()
	if sqlDB == nil {
		t.Fatalf("DB() = nil, want sql database")
	}
	if pool.GORM() == nil {
		t.Fatalf("GORM() = nil, want gorm database")
	}
	if got := (*Pool)(nil).DB(); got != nil {
		t.Fatalf("nil pool DB() = %#v, want nil", got)
	}
	if got := (&Pool{}).GORM(); got != nil {
		t.Fatalf("empty pool GORM() = %#v, want nil", got)
	}

	rows, err := pool.Query(context.Background(), "SELECT 1 UNION ALL SELECT 2 ORDER BY 1")
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	defer rows.Close()
	total := 0
	for rows.Next() {
		var value int
		if err := rows.Scan(&value); err != nil {
			t.Fatalf("Rows.Scan() error = %v", err)
		}
		total += value
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("Rows.Err() error = %v", err)
	}
	if total != 3 {
		t.Fatalf("query total = %d, want 3", total)
	}

	tx, err := pool.BeginTx(context.Background(), TxOptions{})
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if _, err := tx.Exec(context.Background(), "SELECT 1"); err != nil {
		t.Fatalf("Tx.Exec() error = %v", err)
	}
	if err := tx.Rollback(context.Background()); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
}
