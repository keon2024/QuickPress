# CLAUDE.md

This file gives future AI assistants durable context for maintaining QuickPress without re-discovering the project from scratch.

## Project Summary

QuickPress is a local Go-based load testing tool with a Web console. It lets users edit YAML test plans, upload CSV data, configure request chains, run staged concurrency tests, inspect live metrics, and review request/response details.

The repository is intentionally small and mostly standard-library Go plus a single embedded frontend file.

## Tech Stack

- Language: Go 1.21+
- Module: `quickpress`
- Config format: YAML via `gopkg.in/yaml.v3`
- Web server: `net/http`
- Frontend: plain HTML/CSS/JavaScript in one file, no build step
- Static embedding: `web/server.go` embeds `web/static/index.html` with `//go:embed`

## Common Commands

```bash
# Run the console with the default config path config/prod.yml
go run .

# Run the console on a predictable validation port
go run . -config config/prod.yml -listen :18080

# Run all tests
go test ./...

# Build a Linux x86_64 binary at dist/quickpress-linux-amd64
make build-linux-amd64

# Format Go files after Go edits
gofmt -w <changed-go-files>
```

When validating frontend changes, restart `go run` after editing `web/static/index.html`; the HTML is embedded into the Go binary at startup.

## Repository Map

- `main.go`: CLI flags, config loading, server startup.
- `config/config.go`: config structs, defaults, YAML load/save/normalize/validation.
- `config/prod.yml`: default local runtime config used by `go run .`.
- `config/data/`: uploaded or local CSV datasets. Treat large CSV files as user data; do not delete or rewrite them unless explicitly asked.
- `reader/`: dataset loading. CSV headers become variables for request rendering.
- `requests/executor.go`: request-chain execution, variable rendering, assertions, JSON extraction.
- `utils/httputil.go`: HTTP request construction and response helpers.
- `concurrency/concurrency.go`: load-test manager/runner, worker scheduling, runtime adjustment, metrics, request logs.
- `concurrency/concurrency_test.go`: scheduling and stage-splitting regression tests.
- `web/server.go`: API routing, config import/export, CSV upload, run control endpoints.
- `web/static/index.html`: entire Web console UI, state management, charts, stage editing, results view.

## API Surface

The Web console uses these endpoints from `web/server.go`:

- `GET /`: embedded console HTML.
- `GET|POST /api/config`: load/save YAML config.
- `GET /api/config/default`: default config.
- `POST /api/config/import`: parse imported YAML content.
- `POST /api/config/export`: serialize current config to YAML.
- `POST /api/reader/upload`: save uploaded CSV under `config/data/`.
- `POST /api/run/start`: start a load test from the supplied config.
- `POST /api/run/stop`: stop the current run.
- `GET /api/run/status`: current runner state, stages, metrics, elapsed time.
- `GET /api/run/results?limit=120`: recent request logs.
- `GET /api/run/results?failures=true&limit=0`: all retained failed request logs.
- `POST /api/run/adjust`: adjust current target or replace future stages during a run.

## Configuration Semantics

`config.Config` has these top-level sections:

- `app.listen`: Web console listen address.
- `concurrency.loop`: positive fixed loop count, or `-1` for infinite loops. `0` normalizes to `1`.
- `concurrency.unit`: `s`, `m`, or `h`.
- `concurrency.stages`: ordered staged concurrency plan.
- `reader.type`: currently CSV.
- `reader.file`: CSV file path.
- `global`: global string variables.
- `requests`: ordered request chain.

Important stage rule: `stage.duration` is the duration of that individual stage, not a cumulative timestamp. Stages execute in config order. Do not sort stages by duration.

If a stage label is blank in the UI, the frontend normalizes it to `阶段 N`.

## Load-Test Scheduling Rules

Scheduling lives in `concurrency/concurrency.go`:

- `resolveSchedule` sums per-stage durations to choose the target concurrency for the current elapsed time.
- `scheduleCycleDuration` is the sum of stage durations times the configured unit.
- A positive `loop` ends the test after `loop * cycleDuration`.
- `AdjustTarget` can change the current target immediately and may split the current stage internally so the new target takes effect from the current time.
- `ReplaceStages` is used by “同步到运行计划”. It should preserve completed stages and replace only future stages. The cutoff is snapped to completed stage boundaries with `completedStageCutoffDuration` so the currently running stage is not split into an extra visible row.

Regression tests should cover stage-duration semantics and cutoff behavior whenever scheduling changes.

## Runtime Stage Editing UX

The frontend stage table has several important behavior guarantees in `web/static/index.html`:

- While the test is running, only fully executed historical stages are locked.
- The currently executing stage remains a single editable row until it is fully completed.
- After the test stops or completes, all rows return to editable state.
- Automatic status polling must not steal focus from stage inputs.
- Polling must not collapse manually opened request-result details.
- More than five stages makes the table scrollable and defaults the view to the newest rows.
- “新增未来阶段” and “同步到运行计划” live at the bottom of the stage table.

Avoid reintroducing logic that splits the current in-progress stage into “elapsed part” and “remaining part” in the visible table; that causes unstable row counts such as 3 -> 4 -> 3 during a run.

## Results And Charts

Request logs are retained in memory by `requestLogBuffer`:

- Recent general entries are capped by buffer capacity, currently `newRequestLogBuffer(400)`.
- Failures are also tracked separately for the failure filter.
- `/api/run/results` defaults to `limit=120`.

The runtime charts in `web/static/index.html` currently use recent result logs, not a full-run time-series. Full-run charting should be implemented with backend time-bucket aggregation rather than storing every request result for charting. A good design is one bucket per second containing success count, failure count, total latency, and latency sample count.

Chart color convention:

- Average latency line: neutral/blue existing chart style.
- Successful TPS/QPS: green.
- Failed TPS/QPS: red.

Chart fullscreen controls are small `⛶` buttons placed immediately after the chart titles.

## Frontend Maintenance Notes

`web/static/index.html` is a single-file app. Keep changes focused and avoid introducing a build pipeline unless explicitly requested.

Important frontend state fields include:

- `state.config`: current editable config.
- `state.status`: current runner status from `/api/run/status`.
- `state.results`: currently displayed result list.
- `state.chartResults`: unfiltered result data for charts.
- `state.resultFilter`: result list filter mode.
- `state.openResultIds` and `state.closedResultIds`: preserve result detail expansion.
- `state.expandedChart`: active fullscreen chart.

When editing the UI:

- Preserve user focus and selection during polling-driven rerenders.
- Keep result details open when the user explicitly opened them.
- Keep display-filtered result data separate from chart data.
- Restart the Go server before browser validation because of `go:embed`.

## Request Execution Notes

`requests.Executor` renders variables from global config, CSV rows, and extractor outputs.

Supported placeholder styles:

- `${name}`
- `{{name}}`
- `{name}`

Each virtual user executes the configured requests in order. A chain stops at the first request failure, assertion failure, or extractor failure.

Assertions:

- `expected_status > 0` enforces an HTTP status code.
- `contains` requires rendered text to be present in the response body.
- `extractors` read JSON paths from a response and add variables for later requests. Paths support object dots and array indexes, e.g. `data.token`, `data.items[0].id`, `data.items.[0].id`, and `data.items.0.id` when the current node is an array.

## Validation Checklist

Use this checklist before finishing changes:

1. Run `gofmt -w` on changed Go files.
2. Run `go test ./...`.
3. For frontend changes, run `go run . -listen :18080`, reload the browser, and validate the changed UI behavior.
4. Stop the validation server when finished.
5. Do not commit changes unless explicitly asked.

## Safety And Scope

- Do not remove or rewrite user data under `config/data/` without explicit instruction.
- Do not revert unrelated local changes.
- Keep changes consistent with the existing small-project style.
- Prefer focused fixes over broad refactors.
- Add or update tests when changing scheduling, config normalization, request execution, or other shared behavior.