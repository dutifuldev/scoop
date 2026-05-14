package app

import (
	"context"
	"os"
	"testing"
	"time"

	"horse.fit/scoop/internal/db"
)

func TestConfirmDangerousAction(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{name: "yes", in: "yes\n", want: true},
		{name: "short yes", in: "y\n", want: true},
		{name: "default no", in: "\n", want: false},
		{name: "explicit no", in: "no\n", want: false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			oldStdin := os.Stdin
			read, write, err := os.Pipe()
			if err != nil {
				t.Fatalf("pipe: %v", err)
			}
			os.Stdin = read
			t.Cleanup(func() {
				os.Stdin = oldStdin
				_ = read.Close()
			})
			if _, err := write.WriteString(tt.in); err != nil {
				t.Fatalf("write stdin: %v", err)
			}
			_ = write.Close()

			got, err := confirmDangerousAction("Proceed?")
			if err != nil {
				t.Fatalf("confirmDangerousAction() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("confirmDangerousAction() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestDeleteCommandParsingAndBeforeArgument(t *testing.T) {
	t.Parallel()

	cfg, exitCode, ok := parseDeleteCommand([]string{"before", "--dry-run", "--force", "2026-05-14"})
	if !ok || exitCode != 0 {
		t.Fatalf("parseDeleteCommand() ok=%t exit=%d", ok, exitCode)
	}
	if cfg.target != "before" || cfg.value != "2026-05-14" || !cfg.dryRun || !cfg.force {
		t.Fatalf("config = %#v", cfg)
	}
	before, err := parseDeleteBeforeArgument("2026-05-14")
	if err != nil {
		t.Fatalf("parseDeleteBeforeArgument(date) error = %v", err)
	}
	if before.Location() != time.UTC {
		t.Fatalf("before location = %v, want UTC", before.Location())
	}
	if _, err := parseDeleteBeforeArgument("not-a-date"); err == nil {
		t.Fatalf("invalid before argument should fail")
	}
	if code := runDeleteTarget(context.Background(), nil, deleteCommandConfig{target: "collection", value: " "}, time.Now()); code != 2 {
		t.Fatalf("blank collection delete = %d, want validation failure", code)
	}
}

func TestSingleRowChangeOptionsAndDryRun(t *testing.T) {
	t.Parallel()

	opts := newSingleRowChangeOptions(
		"rows_affected",
		"preview",
		"apply",
		func(_ context.Context, _ *db.Pool, id string) (int64, error) {
			if id != "id-1" {
				t.Fatalf("preview id = %q", id)
			}
			return 3, nil
		},
		func(context.Context, *db.Pool, string, time.Time) (int64, error) {
			t.Fatalf("apply should not run during dry-run")
			return 0, nil
		},
	)
	if code := runSingleRowChange(context.Background(), nil, "id-1", time.Now(), true, opts); code != 0 {
		t.Fatalf("dry run code = %d, want 0", code)
	}
}

func TestDeleteBeforeResultFormatting(t *testing.T) {
	before := time.Date(2026, 5, 14, 12, 30, 0, 0, time.FixedZone("SGT", 8*60*60))
	result := db.SoftDeleteBeforeResult{RawArrivals: 1, Articles: 2, Stories: 3}

	output := captureStdout(t, func() error {
		if code := printDeleteBeforeResult(before, result); code != 0 {
			t.Fatalf("printDeleteBeforeResult() code = %d, want 0", code)
		}
		return nil
	})
	assertContainsAll(t, output, "before=2026-05-14T04:30:00Z", "raw_arrivals_affected=1", "articles_affected=2", "stories_affected=3")
}
