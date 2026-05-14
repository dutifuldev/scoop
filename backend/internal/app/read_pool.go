package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"horse.fit/scoop/internal/cli"
	"horse.fit/scoop/internal/db"
)

func runWithReadPool(timeout time.Duration, envLoader *cli.EnvLoader, action func(context.Context, *db.Pool) int) int {
	ctx, cancel, pool, err := connectReadPool(timeout, envLoader)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer cancel()
	defer pool.Close()
	return action(ctx, pool)
}

func runReadPoolList[T any](
	timeout time.Duration,
	envLoader *cli.EnvLoader,
	load func(context.Context, *db.Pool) ([]T, error),
	failureMessage string,
	render func([]T, string) int,
	format string,
) int {
	return runWithReadPool(timeout, envLoader, func(ctx context.Context, pool *db.Pool) int {
		items, err := load(ctx, pool)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", failureMessage, err)
			return 1
		}
		return render(items, format)
	})
}

func runReadPoolValue[T any](
	timeout time.Duration,
	envLoader *cli.EnvLoader,
	load func(context.Context, *db.Pool) (T, error),
	render func(T) int,
) int {
	return runWithReadPool(timeout, envLoader, func(ctx context.Context, pool *db.Pool) int {
		value, err := load(ctx, pool)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return render(value)
	})
}

func renderList[T any](items []T, outputFormat string, writeRows func([]T) error) int {
	if outputFormat == outputFormatJSON {
		if err := printJSON(items); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to encode JSON: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeRows(items); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to render table: %v\n", err)
		return 1
	}
	return 0
}
