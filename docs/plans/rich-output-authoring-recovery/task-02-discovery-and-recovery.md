---
id: "02-discovery-and-recovery"
title: "Generated discovery and real-agent recovery"
status: done
wave: 2
depends_on:
  - 01-validation-feedback
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-002
  - REQ-AGENTS-AGENT-RICH-OUTPUT-003
acceptance_criteria:
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.10
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.11
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.12
  - AC-AGENTS-AGENT-RICH-OUTPUT-003.13
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
---
# Task 02: Generated discovery and real-agent recovery

## Summary

Generate compact schema guidance instead of maintaining a second shape recipe.
Verify natural authoring and correction in real Sol/Luna conversations using
the repaired validator, recording failures as clearly as successes.

## In scope

- Derive element/property guidance from the existing catalog/schema and
  validated examples, including `{type,props,children}`, empty leaf children,
  keyed Table columns, string cells and supported source alternatives.
- Extend the existing generator/check to emit a backend-embedded compact
  guidance artifact. Keep routing prose separate; remove duplicated handwritten
  shapes from `rich_output.go`. Attach appropriate element examples to component
  schema variants and validate them through producer/consumer contracts.
- Test deterministic generation, budget overflow failure, drift detection,
  local-example validity and actual task/Office registration. Keep existing
  650-rune tool, 750-rune Office and 4404-byte task budgets.
- Run the plan's four natural trials plus two controlled-recovery trials in
  the owned isolated test environment. Update only that instance from current
  binaries/assets. Confirm exact models with runtime metadata; preserve fixed
  fictional repository data and the unmodified natural prompt.
- Browser-check each successful natural report with the existing desktop/phone
  helper. Record every trial's task ID, prompt, model, calls/errors, accepted
  components and screenshot receipt; reconcile owning docs and earlier plans
  without deleting their historical failed attempts or passing renderer checks.

## Out of scope

New discovery APIs, larger prompt budgets, model/provider fallback, hidden
fixture recipes, main :9998 changes, broad QA/review, git publication and
blanket claims of provider reliability.

## Acceptance

1. Actual discovery uses checked catalog-derived guidance and valid local
   examples, with unchanged shape acceptance and instruction budgets.
2. Each of the four natural trials produces Table, StatusList and Callout with
   at most one schema rejection. Both controlled trials accept the next
   corrected call without silently dropping values. Record a failing trial and
   keep usability open if this gate fails; do not repeatedly reseed successes.
3. Successful reports display all expected evidence on desktop and phone with
   no page errors or document overflow. Main :9998 is unchanged; the isolated
   instance remains accessible directly on Tailscale with its shutdown command.

## Verification

Run the deterministic commands from repository root:

```bash
(cd apps/web && pnpm exec vitest run lib/rich-output scripts/generate-rich-output-catalog.test.ts)
(cd apps/web && pnpm run rich-output:check)
(cd apps/web && pnpm exec eslint lib/rich-output scripts/generate-rich-output-catalog.ts)
(cd apps/backend && go test ./internal/mcp/server ./internal/sysprompt -count=1)
(cd apps/backend && golangci-lint run ./internal/mcp/server/...)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Before generating artifacts, run new meaningful catalog/generator tests RED.
After generation, run the block GREEN. If scripts or product copy change,
include their specific lint/i18n checks rather than skipping affected checks.

Real-model verification used a privately retained bounded trial driver and
browser helper in the owned isolated environment. The six-trial protocol and
unchanged prompt are recorded in the plan. Supporting command receipts,
trial histories and captures are privately retained, rather than portable
repository scripts.

The driver must wait for each isolated conversation's terminal waiting state,
retain actual prompts and all pages of history, assert exact model routing and
report failed calls. Explicitly distinguish ordinary prompt discovery from
controlled malformed-call recovery. Adapt the browser helper to consume each
successful natural trial; record the final exact invocations and outcomes.
If the owned environment is unavailable, report that limitation; never reuse
the main instance. Do not print copied authentication or environment secrets.

## Files likely touched

- `apps/web/lib/rich-output/schema.ts`, `catalog.test.ts`, existing `examples.ts` as needed and a pure guidance module/test.
- `apps/web/scripts/generate-rich-output-catalog.ts` and its existing `generate-rich-output-catalog.test.ts`.
- `apps/backend/internal/mcp/server/rich_output_catalog_v1.schema.json`, a generated guidance artifact and `rich_output.go`.
- `apps/backend/internal/mcp/server/sysprompt_sync_test.go`; task/Office guidance only if routing wording needs reconciliation.
- Owning rich-output specs, this package and prior evidence-package diagnosis/results links.

## Dependencies

Task 01 done, with relevant error feedback verified. Prior renderer/catalog
delivery and the user-authorized isolated real-Codex environment are inputs.

## Risks

Generated annotations may be omitted by clients. Do not claim they reached the
model without wire evidence. Multiple natural trials still constitute a small
sample; the outcome is a delivery gate, not a reliability estimate.

## Parallelism

`sequential`

## Inputs

- [Owning requirements](../../specs/agents/requirements/agent-rich-output.md), criteria 002.10 and 003.11 through 003.13.
- [System design](../../specs/agents/system-design/agent-rich-output.md), Validation and generated schemas, Evolution and authoring feedback, Structured evidence presentation and discovery.
- [Plan](plan.md), provider matrix and six-trial protocol.
- Existing generator/catalog tests, sysprompt budget tests and private demo scripts.

## Results

Completed 2026-10-12. RED failed for the absent generated guidance artifact
and missing component-local examples; an added StatusList shape assertion also
failed before its array/item summary was included. Recorded in the private
`discovery-red.log` and session command receipts.

Deterministic checks:

- `pnpm exec vitest run lib/rich-output scripts/generate-rich-output-catalog.test.ts`: 8 files, 158 tests passed. The final test-only type assertion correction was followed by the two guidance tests passing again.
- `pnpm run rich-output:check`: passed (also inside final successful `pnpm run typecheck`).
- `pnpm exec eslint lib/rich-output scripts/generate-rich-output-catalog.ts`: passed.
- `go test ./internal/mcp/server ./internal/sysprompt -count=1`: passed (9.362s and 0.049s final run).
- `golangci-lint run ./internal/mcp/server/...`: 0 issues.
- Additional affected checks: web typecheck and `pnpm run i18n:check` passed; public documentation validator passed (47 pages).
- Specification catalog/lint, actual-diff documentation coverage and `git diff --check`: passed.

The generated description is 645 runes and combines fixed routing policy with
schema-derived shapes. No handwritten shape recipe remains in Go. All nine
variants have valid local element examples, and all eight complete discovery
reports pass the existing producer tests. Overflow fails rather than truncates.

Runtime build: `make build-kandev build-agentctl` passed with the task-private Go
cache. The unchanged frontend renderer/assets were retained because this repair
changes producer diagnostics and build-time MCP metadata, with no browser
runtime modules or presentation changes. Rebuilt both native binaries, then
the privately retained restart helper refreshed only the owned lab. Main
:9998 remained unchanged. The lab has since been shut down at the user’s request.

The privately retained trial driver completed all six
planned trials exactly once. Actual routing confirms the requested model in
each conversation; fixture SHA-256 values remained unchanged. Every natural
trial composed Stack, Table, StatusList and Callout with zero schema rejections.
Each controlled trial first reproduced the exact bad input, received the error
at `/spec/elements/services/props/columns/0` with expected object/required key
and label, then succeeded on the next call while preserving titles, label and
cell. Raw histories and actual prompts are retained privately in
`recovery-trials.json` and the per-trial JSON files.

| Actual model | Trial | Rejections | Successes | Task ID |
| --- | --- | --- | --- | --- |
| gpt-6-sol | natural | 0 | 1 | `43545cb3-ae4f-481f-a75c-584988817256` |
| gpt-6-sol | natural | 0 | 1 | `0c901d9e-1111-4f75-8c56-804866bc0732` |
| gpt-6-luna | natural | 0 | 1 | `39df507f-01a3-463c-bb90-c64a3b01b1f6` |
| gpt-6-luna | natural | 0 | 1 | `8e080698-b97e-40ed-a577-b301912de242` |
| gpt-6-sol | controlled | 1 | 1 | `4120e854-361b-4904-8b56-71913df3f25e` |
| gpt-6-luna | controlled | 1 | 1 | `9c3a7fe3-0a95-4595-94cf-e20e72975b88` |

The two controlled rejections are intentional tests, not natural-authoring
failures. These six observed runs pass the package gate; they do not establish
universal provider reliability or prove which schema annotations the model saw.

The privately retained browser helper checked all four natural trials on
desktop and phone.

Each invocation checked report component selection, every literal table cell,
all desktop headers and phone row labels, note semantics, document width and
page errors. Eight viewport checks passed with no page errors/phone overflow.
Screenshots and per-trial `*-browser.json` receipts remain in the private lab;
representative desktop/phone screenshots were inspected.

The disposable environment used direct Tailscale access and was subsequently
shut down at the user's request. Main :9998 remained unchanged. Supporting
evidence is privately retained; no copied credentials remain in the lab.

Follow-up fixture correction (2026-10-12): the status-color verification added
an existing full-consumer suite to the run and exposed eight failures caused by
prepending the extra FilePreview discovery example. Appended it instead, retaining
the original seven example positions; the nine-file catalog/generator/full-consumer
run then passed all 169 tests. Regenerated schema, retained the same guidance,
and rebuilt native binaries. Live trial receipts remain valid for their actual
advertised schema; no model trials were repeated or replaced.

PR review follow-up: schema generation declares only the Kandev-owned `root`
and `elements` fields. A regression injects a future library `state` field and
confirms it cannot widen the closed MCP spec. The test failed before removing
the redundant library-property spread, then passed. Generated artifacts are
unchanged and the drift check passes. Private lab references and final stopped
state were reconciled in response to the aggregate review.
