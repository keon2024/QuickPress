# Copilot Instructions for QuickPress

Before making code changes in this repository, read the root `CLAUDE.md` file for durable project context, architecture notes, validation commands, and maintenance constraints.

Use `CLAUDE.md` as the source of project-specific context for future work. When code changes alter any durable project facts, update `CLAUDE.md` in the same task. Durable facts include:

- Project structure or module responsibilities.
- Build, run, test, or validation commands.
- Public API routes or request/response behavior.
- Configuration schema, defaults, or semantics.
- Load-test scheduling, stage editing, locking, charting, result retention, or request execution behavior.
- Frontend state conventions or `go:embed` validation requirements.
- Safety rules around user data such as files under `config/data/`.

Do not update `CLAUDE.md` for tiny implementation-only edits that do not change behavior, commands, architecture, or maintenance guidance.

Keep changes focused and consistent with the existing small Go project style. Run `gofmt` on changed Go files and `go test ./...` after code changes. For frontend changes in `web/static/index.html`, restart `go run . -listen :18080` before browser validation because the HTML is embedded at startup.