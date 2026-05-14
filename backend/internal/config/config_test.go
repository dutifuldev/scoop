package config

import (
	"strings"
	"testing"
)

func TestValidateRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			name: "missing database url",
			cfg:  Config{DefaultAdminUser: "admin", DBMaxConns: 1, SessionTTLHours: 1, SessionCookieName: "sid"},
			want: "DATABASE_URL is required",
		},
		{
			name: "negative min conns",
			cfg:  Config{DatabaseURL: "postgres://example", DefaultAdminUser: "admin", DBMinConns: -1, DBMaxConns: 1, SessionTTLHours: 1, SessionCookieName: "sid"},
			want: "NP_DB_MIN_CONNS",
		},
		{
			name: "max conns too low",
			cfg:  Config{DatabaseURL: "postgres://example", DefaultAdminUser: "admin", DBMaxConns: 0, SessionTTLHours: 1, SessionCookieName: "sid"},
			want: "NP_DB_MAX_CONNS",
		},
		{
			name: "min exceeds max",
			cfg:  Config{DatabaseURL: "postgres://example", DefaultAdminUser: "admin", DBMinConns: 3, DBMaxConns: 2, SessionTTLHours: 1, SessionCookieName: "sid"},
			want: "cannot exceed",
		},
		{
			name: "missing admin user",
			cfg:  Config{DatabaseURL: "postgres://example", DBMaxConns: 1, SessionTTLHours: 1, SessionCookieName: "sid"},
			want: "DEFAULT_ADMIN_USER",
		},
		{
			name: "invalid session ttl",
			cfg:  Config{DatabaseURL: "postgres://example", DefaultAdminUser: "admin", DBMaxConns: 1, SessionTTLHours: 0, SessionCookieName: "sid"},
			want: "SESSION_TTL_HOURS",
		},
		{
			name: "missing cookie name",
			cfg:  Config{DatabaseURL: "postgres://example", DefaultAdminUser: "admin", DBMaxConns: 1, SessionTTLHours: 1},
			want: "SESSION_COOKIE_NAME",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.Validate()
			if err == nil {
				t.Fatalf("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %q", tt.want, err.Error())
			}
		})
	}
}

func TestValidateAcceptsMinimalConfig(t *testing.T) {
	t.Parallel()

	cfg := Config{
		DatabaseURL:       "postgres://example",
		DBMinConns:        1,
		DBMaxConns:        2,
		DefaultAdminUser:  "admin",
		SessionTTLHours:   1,
		SessionCookieName: "sid",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestLoadReadsEnvironmentAndValidates(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("NP_DB_MIN_CONNS", "2")
	t.Setenv("NP_DB_MAX_CONNS", "4")
	t.Setenv("DEFAULT_ADMIN_USER", "root")
	t.Setenv("SESSION_TTL_HOURS", "24")
	t.Setenv("SESSION_COOKIE_NAME", "sid")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://a.example, https://b.example")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.DatabaseURL != "postgres://example" {
		t.Fatalf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.DBMinConns != 2 || cfg.DBMaxConns != 4 {
		t.Fatalf("pool bounds = %d/%d, want 2/4", cfg.DBMinConns, cfg.DBMaxConns)
	}
	if cfg.DefaultAdminUser != "root" {
		t.Fatalf("DefaultAdminUser = %q", cfg.DefaultAdminUser)
	}
}

func TestLoadReturnsValidationError(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DEFAULT_ADMIN_USER", "admin")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL is required") {
		t.Fatalf("Load() error = %q", err.Error())
	}
}

func TestCORSAllowedOriginsList(t *testing.T) {
	t.Parallel()

	var nilConfig *Config
	if got := nilConfig.CORSAllowedOriginsList(); got != nil {
		t.Fatalf("expected nil list for nil config, got %#v", got)
	}

	cfg := Config{CORSAllowedOrigins: " https://a.example,https://b.example, https://a.example ,, "}
	got := cfg.CORSAllowedOriginsList()
	want := []string{"https://a.example", "https://b.example"}
	if len(got) != len(want) {
		t.Fatalf("origin count mismatch: want %d, got %d (%#v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("origin %d mismatch: want %q, got %q", i, want[i], got[i])
		}
	}
}
