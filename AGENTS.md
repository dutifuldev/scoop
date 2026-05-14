# AGENTS.md

This repository follows the Slophammer Agent Entrypoint doc from the local
Slophammer checkout when Slophammer standards are being applied. Keep this file
repo-specific and keep the Slophammer gates hard.

## Repository Safety

- You MUST NOT insert coding agent specific branding, like `[codex]`, in code,
  PRs or issues created on GitHub.
- For git commits and PR titles that act as the effective merge commit title,
  use Conventional Commits format: `<type>[optional scope]: <description>`.
- Although `e2b-agents` is inspired by `spritz`, you MUST NOT refer to `spritz`
  or `textcortex` in `e2b-agents` code, docs, PRs, issues, commits, comments,
  or other repository artifacts.
- Do not commit, document, or otherwise publish personal details from the local
  Scoop setup. Private local hostnames, usernames, absolute home-directory
  paths, private network names, machine names, tokens, private local service
  URLs, and other person- or machine-specific values must stay in local
  environment variables, ignored files, or private notes.

## Required Local Checks

Before finishing backend or cross-cutting changes, run:

```sh
cd backend
gofmt -w .
golangci-lint run
go vet ./...
go test ./... -count=1
./scripts/check-go-coverage.sh
./scripts/check-go-dry.sh
./scripts/check-go-crap.sh
./scripts/check-go-mutation.sh
cd ..
go run github.com/dutifuldev/slophammer/go/cmd/slophammer@latest check .
go run github.com/dutifuldev/slophammer/go/cmd/slophammer@latest check . --execute
```

Before finishing frontend changes, run:

```sh
pnpm -C frontend run typecheck
pnpm -C frontend run test
pnpm -C frontend run build
```

Before finishing embedding-service changes, run:

```sh
cd embedding-service
uv run --no-project --with ruff ruff check .
uv run --no-project --with mypy mypy main.py
uv run --no-project --with pytest pytest
```

Always run `git diff --check` before committing.

## Testing Expectations

- Add or update meaningful tests for every behavior change.
- Do not write tests that merely exercise lines for coverage without asserting
  domain behavior.
- Keep Go coverage at or above `85%`, Go CRAP at or below `8`, and production
  DRY candidates at `0`.
- Mutation targets are declared in `slophammer.yml`; update them when the
  highest-risk production code moves.

## Typing Rules

- TypeScript must stay `strict`. Do not add explicit `any`, `@ts-ignore`, broad
  unchecked casts, or unvalidated external input. Use `unknown` at boundaries
  and narrow it immediately.
- Python public functions and helpers must be typed. Avoid `Any` except at JSON,
  HTTP, or third-party dynamic boundaries, and narrow values before domain use.
- Go package APIs should stay small and explicit. Prefer concrete domain types
  over reflection, `map[string]any`, or unchecked dynamic values outside IO
  boundaries.

## Dependency Rules

- Do not add a dependency when the standard library or existing repo tooling is
  enough.
- If a dependency is required, keep it narrowly scoped, document why it is
  needed in the change, and make sure it is covered by CI.
- Do not weaken, skip, or remove existing tests, linters, type checks, or CI
  gates to land a change.

## Architecture Boundaries

- Keep domain behavior separate from IO, framework, database, queue, clock,
  random, network, and process state.
- Keep interfaces close to consumers.
- Backend import boundaries are declared in `slophammer.yml`; update the
  boundary list when packages move instead of bypassing it.
- Frontend UI primitives should stay reusable and consistent. Do not duplicate
  icon sizing, provider rendering, tag rendering, or typography rules when a
  shared component exists.
