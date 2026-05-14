package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"horse.fit/scoop/internal/config"
)

var ErrNoRows = sql.ErrNoRows

type TxOptions struct{}

type CommandTag struct {
	rowsAffected int64
}

func (c CommandTag) RowsAffected() int64 {
	return c.rowsAffected
}

type Row struct {
	row *sql.Row
}

func (r *Row) Scan(dest ...any) error {
	if r == nil {
		return ErrNoRows
	}
	if r.row == nil {
		return ErrNoRows
	}
	return r.row.Scan(dest...)
}

type Rows struct {
	rows *sql.Rows
}

func (r *Rows) Next() bool {
	if r == nil || r.rows == nil {
		return false
	}
	return r.rows.Next()
}

func (r *Rows) Scan(dest ...any) error {
	if r == nil || r.rows == nil {
		return ErrNoRows
	}
	return r.rows.Scan(dest...)
}

func (r *Rows) Err() error {
	if r == nil {
		return nil
	}
	if r.rows == nil {
		return nil
	}
	return r.rows.Err()
}

func (r *Rows) Close() {
	if r == nil || r.rows == nil {
		return
	}
	_ = r.rows.Close()
}

type Tx interface {
	QueryRow(ctx context.Context, query string, args ...any) *Row
	Query(ctx context.Context, query string, args ...any) (*Rows, error)
	Exec(ctx context.Context, query string, args ...any) (CommandTag, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

type gormTx struct {
	db *gorm.DB
}

func (t *gormTx) QueryRow(ctx context.Context, query string, args ...any) *Row {
	return &Row{row: t.db.WithContext(ctx).Raw(query, args...).Row()}
}

func (t *gormTx) Query(ctx context.Context, query string, args ...any) (*Rows, error) {
	rows, err := t.db.WithContext(ctx).Raw(query, args...).Rows()
	if err != nil {
		return nil, err
	}
	return &Rows{rows: rows}, nil
}

func (t *gormTx) Exec(ctx context.Context, query string, args ...any) (CommandTag, error) {
	res := t.db.WithContext(ctx).Exec(query, args...)
	return CommandTag{rowsAffected: res.RowsAffected}, res.Error
}

func (t *gormTx) Commit(ctx context.Context) error {
	return finishGormTx(ctx, t.db, true)
}

func (t *gormTx) Rollback(ctx context.Context) error {
	return finishGormTx(ctx, t.db, false)
}

func finishGormTx(ctx context.Context, tx *gorm.DB, commit bool) error {
	scoped := tx.WithContext(ctx)
	if commit {
		return scoped.Commit().Error
	}
	return scoped.Rollback().Error
}

type Pool struct {
	gdb   *gorm.DB
	sqlDB *sql.DB
}

func NewPool(ctx context.Context, cfg *config.Config) (*Pool, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	gdb, err := openGormDatabase(cfg)
	if err != nil {
		return nil, err
	}

	sqlDB, err := configureSQLDatabase(ctx, gdb, cfg)
	if err != nil {
		return nil, err
	}

	pool := &Pool{
		gdb:   gdb,
		sqlDB: sqlDB,
	}
	if err := pool.autoMigrate(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("auto-migrate schema: %w", err)
	}

	return pool, nil
}

// NewIntegrationTestPool opens a single-connection pool for database-backed integration tests.
func NewIntegrationTestPool(ctx context.Context, databaseURL string) (*Pool, error) {
	return NewPool(ctx, &config.Config{
		DatabaseURL:       databaseURL,
		DBMinConns:        0,
		DBMaxConns:        1,
		Environment:       "test",
		LogLevel:          "silent",
		DefaultAdminUser:  "admin",
		SessionTTLHours:   1,
		SessionCookieName: "scoop_session",
	})
}

func openGormDatabase(cfg *config.Config) (*gorm.DB, error) {
	logLevel := resolveGormLogLevel(cfg.LogLevel, cfg.Environment)
	gdb, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		return nil, fmt.Errorf("open gorm database: %w", err)
	}
	return gdb, nil
}

func configureSQLDatabase(ctx context.Context, gdb *gorm.DB, cfg *config.Config) (*sql.DB, error) {
	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("get gorm sql db: %w", err)
	}
	configureSQLPool(sqlDB, cfg)
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return sqlDB, nil
}

func configureSQLPool(sqlDB *sql.DB, cfg *config.Config) {
	maxOpen := int(cfg.DBMaxConns)
	if maxOpen <= 0 {
		maxOpen = 8
	}
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(max(1, min(int(cfg.DBMinConns), maxOpen)))
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
}

func (p *Pool) BeginTx(ctx context.Context, _ TxOptions) (Tx, error) {
	if p == nil || p.gdb == nil {
		return nil, fmt.Errorf("database pool is not initialized")
	}
	tx := p.gdb.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	return &gormTx{db: tx}, nil
}

func (p *Pool) QueryRow(ctx context.Context, query string, args ...any) *Row {
	if p == nil || p.gdb == nil {
		return &Row{row: nil}
	}
	return &Row{row: p.gdb.WithContext(ctx).Raw(query, args...).Row()}
}

func (p *Pool) Query(ctx context.Context, query string, args ...any) (*Rows, error) {
	if p == nil || p.gdb == nil {
		return nil, fmt.Errorf("database pool is not initialized")
	}
	rows, err := p.gdb.WithContext(ctx).Raw(query, args...).Rows()
	if err != nil {
		return nil, err
	}
	return &Rows{rows: rows}, nil
}

func (p *Pool) Exec(ctx context.Context, query string, args ...any) (CommandTag, error) {
	if p == nil || p.gdb == nil {
		return CommandTag{}, fmt.Errorf("database pool is not initialized")
	}
	res := p.gdb.WithContext(ctx).Exec(query, args...)
	return CommandTag{rowsAffected: res.RowsAffected}, res.Error
}

func (p *Pool) Close() error {
	if p == nil || p.sqlDB == nil {
		return nil
	}
	return p.sqlDB.Close()
}

func (p *Pool) DB() *sql.DB {
	return poolSQLDB(p)
}

func (p *Pool) GORM() *gorm.DB {
	if p == nil || p.gdb == nil {
		return nil
	}
	return p.gdb
}

func poolSQLDB(p *Pool) *sql.DB {
	if p == nil {
		return nil
	}
	return p.sqlDB
}

func IsNoRows(err error) bool {
	return errors.Is(err, ErrNoRows)
}

func resolveGormLogLevel(appLogLevel, environment string) logger.LogLevel {
	level := strings.ToLower(strings.TrimSpace(appLogLevel))
	switch level {
	case "trace", "debug":
		return logger.Info
	case "warn", "warning", "info", "":
		return logger.Warn
	case "error":
		return logger.Error
	case "silent":
		return logger.Silent
	default:
		if strings.EqualFold(strings.TrimSpace(environment), "local") {
			return logger.Warn
		}
		return logger.Error
	}
}
