package app

import (
	"context"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"horse.fit/scoop/internal/auth"
	"horse.fit/scoop/internal/config"

	"github.com/rs/zerolog"
)

func TestValidateDefaultAdminDeps(t *testing.T) {
	t.Parallel()

	if err := validateDefaultAdminDeps(nil, nil); err == nil || !strings.Contains(err.Error(), "missing dependencies") {
		t.Fatalf("validateDefaultAdminDeps(nil, nil) error = %v, want missing dependencies", err)
	}
}

func TestHashDefaultAdminPassword(t *testing.T) {
	t.Parallel()

	hash, err := hashDefaultAdminPassword(" secret ")
	if err != nil {
		t.Fatalf("hashDefaultAdminPassword(nonblank) error = %v", err)
	}
	if !auth.VerifyPassword("secret", hash) {
		t.Fatalf("nonblank default admin password hash does not verify")
	}

	blankHash, err := hashDefaultAdminPassword(" ")
	if err != nil {
		t.Fatalf("hashDefaultAdminPassword(blank) error = %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(blankHash), []byte("")); err != nil {
		t.Fatalf("blank default admin hash does not verify against empty password: %v", err)
	}
}

func TestShouldCreateDefaultAdminIntegration(t *testing.T) {
	pool, _ := newAppIntegrationPool(t)
	ctx := context.Background()

	shouldCreate, err := shouldCreateDefaultAdmin(ctx, pool)
	if err != nil {
		t.Fatalf("shouldCreateDefaultAdmin(empty) error = %v", err)
	}
	if !shouldCreate {
		t.Fatalf("shouldCreateDefaultAdmin(empty) = false, want true")
	}

	if _, err := pool.CreateUser(ctx, "admin", "hash", true); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	shouldCreate, err = shouldCreateDefaultAdmin(ctx, pool)
	if err != nil {
		t.Fatalf("shouldCreateDefaultAdmin(existing user) error = %v", err)
	}
	if shouldCreate {
		t.Fatalf("shouldCreateDefaultAdmin(existing user) = true, want false")
	}
}

func TestCreateDefaultAdminRejectsBlankUsername(t *testing.T) {
	pool, _ := newAppIntegrationPool(t)
	err := createDefaultAdmin(context.Background(), pool, testConfig(" "), testLogger())
	if err == nil || !strings.Contains(err.Error(), "username is empty") {
		t.Fatalf("createDefaultAdmin(blank username) error = %v, want username error", err)
	}
}

func TestCreateDefaultAdminIntegration(t *testing.T) {
	pool, _ := newAppIntegrationPool(t)
	ctx := context.Background()

	cfg := testConfig(" Admin ")
	cfg.DefaultAdminPassword = " secret "
	cfg.DefaultAdminMustChangePassword = true
	if err := createDefaultAdmin(ctx, pool, cfg, testLogger()); err != nil {
		t.Fatalf("createDefaultAdmin() error = %v", err)
	}

	user, err := pool.GetUserByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if !user.MustChangePassword {
		t.Fatalf("MustChangePassword = false, want true")
	}
	if !auth.VerifyPassword("secret", user.PasswordHash) {
		t.Fatalf("created admin password hash does not verify")
	}
	if _, err := pool.GetUserSettings(ctx, user.UserID); err != nil {
		t.Fatalf("GetUserSettings: %v", err)
	}

	if err := createDefaultAdmin(ctx, pool, cfg, testLogger()); err != nil {
		t.Fatalf("createDefaultAdmin(duplicate) error = %v", err)
	}
	count, err := pool.CountUsers(ctx)
	if err != nil {
		t.Fatalf("CountUsers: %v", err)
	}
	if count != 1 {
		t.Fatalf("CountUsers() = %d, want duplicate create to keep one user", count)
	}
}

func testConfig(username string) *config.Config {
	return &config.Config{
		DefaultAdminUser:               username,
		DefaultAdminPassword:           "password",
		DefaultAdminMustChangePassword: false,
	}
}

func testLogger() zerolog.Logger {
	return zerolog.Nop()
}
