---
id: task-01
title: Plugin manifest types + validation
status: done
wave: 1
depends_on: []
plan: docs/plans/plugins/plan.md
---

# Plugin manifest types + validation

## Title
Define the plugin manifest Go types and a validator, parsed from YAML.

## Inputs
- Spec `docs/specs/plugins/spec.md` → "Data model / Plugin registration" (the
  YAML manifest: id, api_version, version, display_name, description, author,
  categories, base_url, endpoints{health,events,tools,webhooks},
  capabilities{events,api_read,api_write,state,secrets}, tools[], webhooks[],
  config_schema, ui.pages[] (NEW: `key`, `title`, `path`, `surface`)).
- id pattern: `^[a-z0-9][a-z0-9._-]*$`.
- categories enum: connector | automation | tools | analytics.
- `ui.pages[].surface` enum: settings | task-panel | main-nav.
- Follow `apps/backend/AGENTS.md` (package layout, small funcs, golangci limits).

## Acceptance
1. `internal/plugins/manifest/manifest.go` defines `Manifest`, `Endpoints`,
   `Capabilities`, `Tool`, `Webhook`, `UIPage` structs with yaml + json tags.
2. `Parse([]byte) (*Manifest, error)` parses YAML; `(*Manifest) Validate() error`
   returns a joined/first error for: bad id pattern, missing base_url, missing
   required endpoints, unknown category, unknown ui surface, duplicate tool/webhook
   keys, `api_version != 1`.
3. Capability helpers: `HasEvent(name string) bool` (supports wildcard match so
   `task.*` matches `task.created`), `CanRead(resource) bool`, `CanWrite(resource) bool`.

## Files
- `apps/backend/internal/plugins/manifest/manifest.go`
- `apps/backend/internal/plugins/manifest/manifest_test.go`
- `apps/backend/internal/plugins/manifest/match.go` (wildcard subject match) + test

## Verification
- `go test ./internal/plugins/manifest/...` from `apps/backend`
- `make -C apps/backend lint`

## Output contract
Report: types added, validation rules covered by tests, wildcard match behavior,
any spec ambiguity encountered. Do not touch files outside `internal/plugins/manifest/`.

## Dependencies
None.
