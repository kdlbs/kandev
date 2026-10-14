---
created: 2026-10-10
status: implemented
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-002
  - REQ-AGENTS-AGENT-RICH-OUTPUT-003
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
legacy_specs: []
---
# Implementation Plan: Rich-output authoring recovery

## Overview

Repair misleading MCP validation feedback, replace duplicated shape guidance
with checked catalog-derived metadata, and verify real-agent authoring and
recovery. Execute two sequential work orders in this primary session. The
completed renderer/component packages retain their passing technical receipts;
this package owns the remaining agent-usability correction.

Agents owns this repair because the failed contract is agent-facing authoring
and recovery shared by task and Office consumers. No material user decision is
unresolved: strict validation, json-render, the native catalog, CSV support and
the isolated real-Codex trial environment are already authorized.

## Evidence and root cause

All eight retained demo histories were inspected without pagination loss.
The first evidence Sol run rejected 12 calls before success; the next rejected
seven calls before four separate successes. Two Luna runs selected Markdown.
The last Sol run succeeded once after metadata changes. Those observations
cannot establish general reliable authoring.

`tool_argument_validation.go` recursively collects required and unknown fields
from every `oneOf`/`anyOf` cause. Mutually exclusive component and source
branches are flattened into one contradictory message, and the primary union
error obscures the nested type failure. Registered discovery advertised nine
variants and seven examples; what the model actually saw is not established.

A private Go overlay reproduced the defect with a single Table element:
`columns:["Service"], rows:[["API"]], title:"Services", children:[]`.
The actual error is that `columns[0]` must be an object with `key` and `label`.
The current message also demands `chart_type`, `direction`, `items`, `kind`,
`labels`, `path`, `series`, `summary`, `text` and `csv`, and calls valid
`columns`, `rows` and `title` unknown. The overlay was removed. The regression
must fail against the current registered validator before production changes.

## Scope

### In scope

- Schema-aware, sanitized diagnostic projection for nested alternatives,
  preserving independent failures and existing non-union behavior.
- Checked generated shape metadata and local examples from the sole catalog.
- Deterministic rejected-call/corrected-call integration evidence, without
  source reads on rejection or changes to accepted authoring shapes.
- Repeated Sol/Luna natural-prompt and controlled-recovery trials in the
  disposable instance, with honest outcomes and desktop/phone confirmation.

### Out of scope

New components, legacy writers, permissive aliases, automatic argument repair,
MCP Apps, new transports/tools, provider normalization, database migrations,
main-instance operations, delegation, commit, push and PR publication.

## Technical approach

### Relevant validation feedback

Keep `compileToolArgumentSchema` and its jsonschema/v6 validator authoritative.
Pass the compiled schema and submitted instance to the sanitizer as needed for
a read-only diagnostic projection. Use schema-declared constant discriminators
to select recognized element variants, then uniquely present required source
properties to distinguish inline and CSV. Do not select by smallest error
count or simply drop all union causes. Unresolved alternatives remain explicitly
labelled alternatives; both/neither sources and multiple matching `oneOf`
branches remain errors. Traverse `allOf` and independent object failures without
discarding them. Inspect the installed dependency's exported `Schema` and
`ValidationError` types before adding an index or duplicating schema traversal.

Format the actual nested pointer, expected type and schema-required object
fields, preserving missing/unknown-property diagnostics and session-bound
task guidance. Never format raw validator messages or submitted values. Bound
new explanatory details to eight entries and 4 KiB, retaining an explicit
omission indicator. Do not change the schema, read behavior or success result.

### Generated discovery and observed usability

Extend `apps/web/scripts/generate-rich-output-catalog.ts` to generate a compact
shape-guidance artifact from the catalog/schema and validated examples.
`rich_output.go` embeds that artifact and combines it with concise routing
policy. Remove its manually maintained shape recipe. Attach relevant complete
element examples to schema variants, while preserving root examples and all
schema acceptance semantics. Keep tool description <=650 runes, Office
guidance <=750 runes and ordinary task context <=4404 bytes. Generation/check
must fail on overflow or drift. Do not hide recipes in user requests or fixture
repository instructions.

| Provider and transport | Evidence and intended behavior | Unsupported or failed outcome |
| --- | --- | --- |
| Codex ACP, exact gpt-6-sol | Fresh natural prompt and real recovery calls; retain actual model metadata and every call | Record failure; do not infer model-visible schema from backend advertisement |
| Codex ACP, exact gpt-6-luna | Same prompt, fixture and feedback tests as Sol | Markdown selection or prolonged guessing leaves usability open |
| Task/Office registered MCP surfaces | Deterministic schema/examples, budget and validation tests | Existing visibility and normal failed-tool behavior remain |
| Other providers | Existing normalized identities and fixture coverage only | No live-provider claim or transport change |

## Tests

| Criteria | Required evidence |
| --- | --- |
| AC-AGENTS-AGENT-RICH-OUTPUT-003.12 | `tool_argument_validation_test.go`: `TestToolArgumentValidationSelectsRelevantUnion`, covering typed components, inline/CSV, unknown/missing type, ambiguous sources, nested wrong types, sibling/root failures and unresolved generic alternatives |
| AC-AGENTS-AGENT-RICH-OUTPUT-003.13 and AC-AGENTS-AGENT-RICH-OUTPUT-002.2 | `TestRichOutputRejectedCallRecovery`: wrapped task/Office calls reject without backend action, omit sentinel values, accept corrected inline and CSV calls; existing closed-schema and task-scope regressions |
| AC-AGENTS-AGENT-RICH-OUTPUT-003.11 and AC-AGENTS-AGENT-RICH-OUTPUT-002.10 | `catalog.test.ts`, new pure guidance-generator tests, generated artifact check and `TestRichOutputEvidenceDiscoveryGuidance`: valid local examples, deterministic generation, unchanged budgets and registration |

Use the real compiled schemas and wrapped tool calls. Do not test only an error
string fabricated by a helper or merely snapshot the generator's implementation.

## End-to-end evidence

No new presentation UI is introduced. Actual end-to-end evidence is collected
through the existing isolated ACP/MCP/tool/result/renderer pipeline, followed by
the privately retained browser helper on desktop and phone. Preserve
the existing Playwright catalog/replay receipts; no broad browser rerun is
required for schema annotations or backend error text alone.

Run two fresh natural-prompt trials per model with the exact unchanged prompt:

> Review the service costs and verification results in this repository. Compare the services, explain which checks passed or failed, and highlight the main issue worth investigating.

Then run one controlled recovery trial per model using the malformed Table
regression, exposed as a real tool rejection in that conversation. The recovery
prompt may explicitly request correction of that call; report these separately
from natural discovery, and never insert corrected JSON into the prompt.
Keep the fixture data fixed and capture all six trial histories without lost
pagination. Natural trials must each independently select rich output and
produce Table, StatusList and Callout with at most one schema rejection.
Controlled trials must accept the next corrected call while preserving values.
These are bounded delivery checks, not a population-level reliability claim.
If any fail, record the failing trial and leave usability open; do not retry
until only successes remain or grow prompt budgets to make the check pass.

## Work orders

- [x] [Task 01: Relevant schema validation feedback](task-01-validation-feedback.md)
- [x] [Task 02: Generated discovery and real-agent recovery](task-02-discovery-and-recovery.md)

## Verification results

Implemented locally on 2026-10-12. Both sequential work orders are done;
production diagnostics and generated discovery passed their targeted checks.
All four fresh natural Sol/Luna trials composed the full evidence report with
zero rejected calls. Both controlled trials recovered from the deliberately
malformed Table on their next call. All four natural reports passed desktop
and phone browser checks, including every table field and literal cell.

Final receipts and commands are in [Task 01](task-01-validation-feedback.md#results)
and [Task 02](task-02-discovery-and-recovery.md#results). Main :9998 remained
unchanged; the isolated instance was later stopped (see PR preparation below).
The six-trial usability gate passed; broader provider reliability remains
unmeasured.

Design checkpoint (2026-10-10): specification catalog validation, specification
lint, actual-diff PR documentation coverage and diff whitespace checks passed.
Both work orders remain pending. No production or permanent test changes were
made for this repair; the package remains unstaged and uncommitted.

## Risks

- The shared sanitizer serves every MCP tool; preserve non-union errors,
  mode-change synchronization, intentionally open maps and task-scope guidance.
- Generic alternatives may not have an unambiguous discriminator; truthful
  labelled alternatives are preferable to invented required fields.
- MCP clients may omit schema annotations. Actual model-visible schema remains
  unknown unless protocol evidence proves it; do not redesign a provider on
  inference alone.
- Six model trials cost time and tokens and cannot guarantee future behavior.
  Preserve all outcomes, copied-auth privacy and the main :9998 process.


PR preparation (2026-10-12): user requested a ready PR and shutdown of the demo.
The owned :48479 instance was stopped and copied Codex authentication removed;
the primary instance stayed running. Fresh PR screenshots were captured in
separate managed E2E fixtures (desktop Chromium and mobile Chrome, one passing
case each), with ephemeral specs removed and a two-asset manifest validated.
The 300-line web harness limit was restored without removing guidance; harness
lint/tests and specification lint/tests passed. Screenshot binaries are kept
outside the feature commit and will be published through the orphan media ref.


PR fixup (2026-10-12): reconciled the current main branch without restoring
legacy authoring. Preserved upstream chart-legend derivation, browser-demo
build support and delivery-recovery translations. Adapted the added chart
visibility regression to the native catalog component. CodeRabbit's aggregate
review identified private lab links and a FilePreview design typo; trial records
now retain task IDs and outcomes with privately held evidence, and the typo is
corrected. No behavior or contract change requires new acceptance criteria.

Post-merge validation: 17 focused Vitest files passed all 234 tests; MCP/server
and sysprompt tests passed. Schema drift, full web lint, i18n, harness/spec and
public-doc checks passed. Desktop rich-output suites passed seven tests; the
mobile-chrome suites passed five, using fresh managed E2E builds and isolated
fixtures. The full typecheck passed with an 8 GiB Node heap after the default
heap exhausted memory. Two incoming documentation whitespace defects were
corrected for the normal hooks. The disposable user demo remains stopped.


Review and CI follow-up (2026-10-12): the four-data-component diagnostic now
names every counted catalog component, including StatusList. The generator
owns the closed `root`/`elements` spec boundary; a future library `state` field
cannot enter the MCP contract. Both regressions failed before correction, then
passed. The stopped-demo receipt is consistent throughout the plan.

The full CI frontend suite exposed a stale reviewed-test entry for the retired
rich-output parser; the test inventory is reconciled. The PostgreSQL retention
job exposed a direct-acknowledgement assumption in its due-discovery fixture.
Twenty local PostgreSQL runs passed, while deterministic lost-commit-ack cases
reproduced the fixture failure in both engines. The test now uses existing
bounded retries and durable payload measurements, retaining the row/read
limits and production deadlines. This is a test-only CI repair, with no new
retention behavior or public contract.

Follow-up verification: 235 focused frontend tests passed, followed by the
three-file catalog/generator/inventory run (nine tests) after the type-only test
correction. Typecheck, schema drift and targeted web lint passed. MCP/server
and sysprompt tests passed; affected Go lint reported zero issues. The corrected
retention discovery fixture passed under `-race` for five repeats of all four
SQLite/PostgreSQL acknowledgement cases (20 cases, 43.056s). Specification
catalog/lint, documentation coverage and diff whitespace checks passed.

Current-main integration: the later upstream admission fix merged cleanly.
Its five focus/admission regressions passed, and fresh managed rich-output E2E
runs passed all seven desktop and five phone cases. Temporary PostgreSQL test
resources were removed. The disposable user demo remains stopped, and the
primary instance is unchanged.


Final review follow-up: public reserved-ID guidance includes `__proto__`, and
machine-local process identifiers were removed from the trial records. The
suggested persistent standalone fallback store is intentionally not adopted:
it retains an open file disclosure across authentication changes. A regression
verifies same-identity rerender preservation, auth-change collapse and no
unrequested preview read. The current renderer passes; a temporary application
of the suggested change fails the collapse assertion. The original renderer
was restored unchanged. Keyed stores retain the existing bounded persistence.

The final review regression run passed all 20 renderer/state/file-target tests
across three files, and targeted lint passed. Public documentation validation,
specification catalog/lint and diff whitespace checks passed. No production
renderer change was made; the authentication reset remains intentional.

Browser CI follow-up: the workspace menu no longer acquires a second modal
pointer lock inside the task picker. Changing workspace can unmount the menu;
the parent drawer retains modal ownership and releases its lock on dismissal.
The original mobile reconnect case failed six of ten local repeats. The
strengthened fixture verifies the open drawer still blocks background input
and that dismissal restores it. A separate trace proved the fixture began
switching before WebSocket reconnection invalidated its read journal; it now
awaits the connected state while keeping old workspace responses held.

With retries disabled, the corrected recovery case passed ten phone repeats
and five desktop repeats. Seven neighboring phone tests passed, including
shared-state pagination, workspace isolation, saved views and Quick Chat
cancellation. Targeted lint and formatting passed. Temporary store probes
were removed; no production read-reconciliation boundary was changed.

Later main integration: preserved upstream automation-start ownership,
terminal-font draft handling and mobile task controls. Main independently added
the same nonmodal workspace-menu setting; the sole textual conflict was
resolved by retaining that setting and its invariant comment. The combined
backend tree is byte-identical to an isolated merge already verified by 911
root Go tests under `-race` across eight packages. Frontend verification passed
55 tests across six files, full typecheck, schema drift and documentation/spec
validation. Fresh managed builds passed seven desktop and six phone browser
cases, including historical CSV replay and reconnect recovery. New-head CI and
late review monitoring remain pending until publication completes.
