---
spec: docs/specs/integrations/mcp-tool-argument-validation.md
created: 2026-08-03
status: done
---

# Fix Plan: Recover Invalid Walkthrough MCP Calls

## Overview

Make shared MCP validation errors actionable without weakening their redaction,
then put the load-bearing walkthrough call shape in both task context and the
built-in walkthrough request. Bind those instructions to the registered schema
with focused tests and update the public MCP reference.

## Confirmed root cause

`sanitizedToolArgumentError` reduces a JSON Schema `required` failure to its
instance path and keyword. A walkthrough step missing `text` therefore returns
`validation failed at /steps/0 (keyword: required)` and never reaches the
walkthrough service's more specific `step 1 text is required` validation. The
shared validator landed on 2026-08-01 and now intercepts this malformed call
before the tool-specific handler.

The registered `show_walkthrough_kandev` schema correctly requires `file`,
`line`, and `text`, but the injected task context omits all walkthrough tools
and the built-in `changes-walkthrough` prompt never states the argument shape.
Clients or models that do not preserve nested schema detail therefore receive
weak initial guidance followed by an error that does not identify the missing
field.

Smallest reproduction: call `show_walkthrough_kandev` with
`steps: [{"file":"example.go","line":1,"title":"Explanation"}]`. The
advertised schema contains `required: ["file", "line", "text"]`, but the
returned error does not contain `text` and no backend action runs.

## Backend

### Actionable required-property errors

- `apps/backend/internal/mcp/server/tool_argument_validation.go` — recognize
  the validator library's `kind.Required` error and append its schema-defined
  missing property names to the sanitized result. Preserve the current path and
  keyword for compatibility and retain the existing generic formatting for all
  other constraints.
- Never format a generic validator message or rejected value. Only the missing
  names declared by the registered schema may enter the error.
- Keep fail-closed validation, root closure, open nested maps, mode rebuilds,
  handler suppression, and create-task compatibility normalization unchanged.

### Existing-install prompt refresh

- `apps/backend/internal/prompts/store/sqlite.go` — refresh only the exact
  loader-normalized stored content of the two historical `changes-walkthrough`
  built-in revisions that predate the required step shape and have never been
  saved by a user.
- Preserve unrecognized content, edited built-ins, and user-owned name
  conflicts. Keep the conditional write race-safe so an edit concurrent with
  startup cannot be overwritten.

## Agent prompt contract

- `apps/backend/config/prompts/kandev-context.md` — list
  `show_walkthrough_kandev`, `get_walkthrough_kandev`, and
  `delete_walkthrough_kandev`. State that `show` requires an ordered `steps`
  array and every step requires `file`, `line`, and `text`; keep optional
  top-level and step fields concise.
- `apps/backend/config/prompts/changes-walkthrough.md` — repeat the exact
  required step shape next to the existing generation requirements. Do not add
  task-specific paths, diff contents, or a copied sample that an agent could
  submit literally.

## Public documentation

Update the MCP validation contract in
`docs/public/automation-and-mcp.md` to state that `required` failures name
missing schema properties while never echoing argument values. The existing
walkthrough guide already documents that each step needs text, a file, and a
positive line, so it needs no behavior change.

Primary content type: reference.

## Tests

- **What:** A nested missing-required-property failure names `text`, retains
  `/steps/0` and `required`, does not expose sibling values, and dispatches no
  backend action.
  **File:**
  `apps/backend/internal/mcp/server/tool_argument_validation_test.go`.
  **How:** Call the real registered `show_walkthrough_kandev` tool through
  `wrapHandler` with the minimal malformed step. Write this regression first
  and confirm it fails because the current result omits `text`.
- **What:** Walkthrough instructions stay aligned with the live tool schema.
  **File:** `apps/backend/internal/mcp/server/sysprompt_sync_test.go`.
  **How:** Extract the `steps` item required fields from the registered tool,
  assert the first-turn task context names all three walkthrough tools, and
  assert both task context and embedded `changes-walkthrough` content mention
  `steps` plus every required item field.
- **What:** Public documentation remains structurally valid.
  **File:** `docs/public/automation-and-mcp.md`.
  **How:** Run both public-doc validation scripts.
- **What:** Existing installations receive the corrected walkthrough request
  without losing prompt customizations.
  **File:** `apps/backend/internal/prompts/store/sqlite_test.go` plus historical
  fixtures under `apps/backend/internal/prompts/store/testdata/`.
  **How:** Initialize repositories over each shipped legacy revision in the
  exact trimmed form stored by `promptcfg.Get` and require refresh, then prove
  unrecognized content, a saved built-in, and a user-owned name conflict remain
  unchanged.

No browser E2E is needed: this changes agent-facing MCP diagnostics and hidden
prompt instructions, not rendered UI behavior.

## Verification Results

Completed on 2026-08-03 with strict red-green coverage:

- Focused RED tests failed because required-property diagnostics omitted
  `text`, task context omitted the walkthrough tools, and neither prompt surface
  documented the step shape.
- The same focused tests passed after implementation, including coverage for
  multiple missing properties and submitted-value redaction.
- `cd apps/backend && go test ./internal/mcp/server ./internal/sysprompt` —
  passed.
- `node --test scripts/validate-public-docs.test.mjs` — passed, 58 tests.
- `node scripts/validate-public-docs.mjs` — passed, 41 published pages.
- `git diff --check` — passed.

PR review remediation also passed:

- Focused RED tests reproduced stale stored prompts and declaration-ordered
  missing-property output.
- Focused GREEN tests passed for both shipped legacy prompt revisions, edited
  prompt preservation, canonical property sorting, the top-level `steps`
  contract, and immediate test cleanup.
- `cd apps/backend && go test ./internal/prompts/store ./internal/mcp/server ./internal/sysprompt` — passed.
- `cd apps/backend && golangci-lint run ./... --new-from-rev="0ca00c2aca4430a5e4f06874ba960743de1acf9a" --timeout=5m` — passed with 0 issues.

Follow-up review remediation also passed:

- Focused RED coverage seeded both historical revisions in the exact trimmed
  form produced by `promptcfg.Get`; both remained stale with the fixture-file
  hashes.
- Focused GREEN coverage passed after hashing the loader-normalized historical
  contents. Equal-timestamp unrecognized content and edited content remain
  unchanged.
- `cd apps/backend && go test ./internal/prompts/store ./internal/mcp/server ./internal/sysprompt` — passed.
- `cd apps/backend && golangci-lint run ./... --new-from-rev="0ca00c2aca4430a5e4f06874ba960743de1acf9a" --timeout=5m` — passed with 0 issues.
- `node --test scripts/validate-public-docs.test.mjs` — passed, 58 tests.
- `node scripts/validate-public-docs.mjs` — passed, 41 published pages.
- `git diff --check` — passed.

No browser E2E was run because the change affects MCP diagnostics and embedded
agent prompts, not rendered UI behavior.

## Implementation Wave

- [x] [Task 01: Make walkthrough call recovery actionable](task-01-actionable-walkthrough-call-recovery.md) — done.

Execution stays sequential in the primary conversation. No subagents are
authorized.

## Risks and out of scope

- Calling a generic localized validator formatter could expose rejected values;
  inspect only the typed `required` error's missing schema names.
- Keep existing non-`required` error text stable to avoid broadening this fix.
- Do not alter the walkthrough schema, persistence schema, unrelated
  persistence behavior, rendering, MCP transport, authorization, or retry
  behavior.
- Task context receives only the walkthrough contract needed for reliable calls;
  this work does not mirror all registered tool schemas into that prompt.
- Exact offending ACP frames were not available, so this repair hardens both
  documented contributors without claiming whether one client dropped `text`
  or one model omitted it.
