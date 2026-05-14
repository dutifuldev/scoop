package db

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
)

//go:embed sql/pre_automigrate.sql
var preAutoMigrateSQL string

//go:embed sql/post_automigrate.sql
var postAutoMigrateSQL string

func (p *Pool) autoMigrate(ctx context.Context) error {
	if p == nil || p.gdb == nil {
		return fmt.Errorf("database pool is not initialized")
	}
	return p.runAutoMigrate(ctx)
}

func (p *Pool) runAutoMigrate(ctx context.Context) error {
	scripts := []struct {
		name string
		sql  string
	}{
		{name: "pre-auto-migrate", sql: preAutoMigrateSQL},
	}

	for _, script := range scripts {
		if err := executeMigrationSQL(ctx, p, script.name, script.sql); err != nil {
			return err
		}
	}

	if err := p.gdb.WithContext(ctx).AutoMigrate(autoMigrateModels()...); err != nil {
		return fmt.Errorf("gorm auto-migrate models: %w", err)
	}

	if err := executeMigrationSQL(ctx, p, "post-auto-migrate", postAutoMigrateSQL); err != nil {
		return err
	}

	return nil
}

func executeMigrationSQL(ctx context.Context, p *Pool, label, sqlText string) error {
	trimmed := strings.TrimSpace(sqlText)
	if trimmed == "" {
		return nil
	}
	if err := p.gdb.WithContext(ctx).Exec(trimmed).Error; err != nil {
		return fmt.Errorf("execute %s SQL: %w", label, err)
	}
	return nil
}
