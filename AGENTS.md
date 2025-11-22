# Repository Guidelines

## Project Structure & Module Organization
- `cmd/myagent`: runnable entry point.
- `pkg/agent`: ReAct loop, background jobs, orchestration.
- `pkg/assistants`: task delegators (serial/parallel) implemented as tools.
- `pkg/builder`: constructs agents from configs.
- `pkg/llmclient`: LLM client and config loader.
- `pkg/tools`: tool interface and runtime context.
- `pkg/types`, `pkg/history`: shared types and history strategies.
- Example config: `config.example.toml` (copy to `config.toml`, never commit secrets).

## Build, Test, and Development Commands
- Run example: `go run ./cmd/myagent`
- Build binary: `go build -o bin/myagent ./cmd/myagent`
- Format: `go fmt ./...`
- Lint (standard tools): `go vet ./...`
- Tests (if present): `go test ./...` or `go test -cover ./...`
- Module tidy: `go mod tidy`

## Coding Style & Naming Conventions
- Go version: 1.23+ (module declares toolchain 1.24.x).
- Use `gofmt` (tabs, standard line width). CI expects formatted code.
- Packages: short, lowercase (e.g., `agent`, `tools`).
- Exports: `CamelCase` for types/functions; unexported use `camelCase`/`lower_snake` files.
- Errors: wrap with `%w`; return early on errors.

## Testing Guidelines
- Use Go `testing` with table-driven tests.
- File naming: `*_test.go`; function names: `TestXxx`, `BenchmarkXxx`.
- Run full suite: `go test ./...`; add `-race` where feasible.
- Keep tests hermetic; avoid real network/LLM calls—mock `llmclient`.

## Commit & Pull Request Guidelines
- Commits: concise, imperative subject (<=72 chars). Examples:
  - `agent: handle background tool timeouts`
  - `tools: add FileTool write path validation`
- PRs: include purpose, scope, and testing notes; link issues; add run commands/output when relevant.
- Small, focused PRs are preferred; update `README.md` and `AGENTS.md` when behavior or workflows change.

## Security & Configuration Tips
- Do not commit secrets. Use `config.example.toml` as a template and keep `config.toml` untracked (see `.gitignore`).
- For local dev, create `config.toml` from the example and supply API keys locally.

## Agent-Specific Notes
- New tools must implement `tools.Tool` and can optionally support background execution via `run_in_background` in parameters. Register tools via `pkg/builder` configs.
