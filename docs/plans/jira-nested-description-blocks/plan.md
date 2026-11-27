---
created: 2026-10-13
status: done
requirements:
  - REQ-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001
system_design:
  - ../../specs/integrations/system-design/jira-watcher-task-prompts.md
legacy_specs: []
---

# Implementation Plan: Separate Nested Jira Description Paragraphs

## Overview

Correct the shared Jira plain-text description conversion so separate instructions
inside lists and other containers remain separate. One sequential work order
first proves both affected REST client paths fail, then repairs the converter
and verifies the complete Jira package.

The integration system owns this outcome because it normalizes provider document
data before either launch presets or watcher task prompts consume it.

## Contract reconciliation

The active [requirement](../../specs/integrations/requirements/jira-watcher-task-prompts.md)
and `AC-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001.3` already require paragraph
separation. Reuse that criterion without editing it or creating a repair-only
requirement. Criteria `.2` and `.4` supply readable-description and absent-value
compatibility controls. The current owning design is reconciled in its
[Description conversion](../../specs/integrations/system-design/jira-watcher-task-prompts.md#description-conversion)
section.

The completed [earlier description-fetch package](../fix-jira-watcher-description/plan.md)
fixed REST field selection. Its scope, completion status, and historical results
remain accurate; this package repairs conversion of the description it fetches.
No schema, ownership, persistence, or security decision changes, so no ADR is needed.

## Confirmed root cause and evidence

`walkADF` in `apps/backend/internal/jira/cloud_client.go` inserts `\n\n` only
between children whose parent is `doc`. Paragraphs nested under `listItem`
therefore concatenate even though they are separate blocks. `issueToTicket`
uses that converter for both `CloudClient.GetTicket` and
`CloudClient.SearchTicketsForWatch`.

At baseline `77038ba501ecb26a42a6c2cfc001ce7628c58095`, ROOT's temporary real-HTTP
proof returned `Run migration.Verify the schema.Restart worker.` from both
clients. Expected text is `Run migration.\n\nVerify the schema.\n\nRestart worker.`
The native command joined with exit 1: two causal failures and three passing
controls (plain string, inline adjacency, top-level paragraphs). This evidence
establishes the defect; implementation must independently reproduce RED with
the permanent regressions before changing production.

Evidence files are `/tmp/kandev-root-jira-blocks-discovery-20261013-360oxtae/qualified-proof.json`
and `proof.log`. The private `candidate_test.go` belongs to ROOT and remains
outside this package's file ownership. It must not be copied over, edited, or
deleted by implementation.

## Scope

### In scope

- Preserve paragraph boundaries at arbitrary enclosing-container depth.
- Cover list siblings, multiple paragraphs within an item, nested lists,
  blockquotes, and an unknown container with paragraph descendants.
- Preserve inline text order, plain/null values, existing top-level separators,
  explicit breaks, literal whitespace, and tolerant malformed-node handling.
- Prove both public REST client methods produce the corrected description.

### Out of scope

- Markdown rendering, list markers, table fidelity, mention/media/link features.
- Jira credentials, API fields, pagination, watcher flow, task schema or routing.
- Frontend production, interaction, layout, localization, and browser redesign.
- New dependencies, release toggles, telemetry, or broad refactors.

## Technical approach

Keep `extractDescription` and `walkADF` as the shared conversion boundary in
`cloud_client.go`. Separate paragraph completion from inline text emission so
the walker honors a paragraph boundary across container exits and entries.
Emit a structural boundary only when another readable block follows, and let
the existing document separator satisfy that boundary. Preserve explicit and
literal newlines rather than normalizing the completed string beyond the
existing final `TrimSpace`. Keep existing top-level empty-block spacing.

Do not maintain a growing list of list/blockquote container types. Every
container recursively exposes paragraph descendants. Any private helper or
traversal state needed for this correction stays in the same production file.
Keep unsupported leaf output and adjacent inline fragments unchanged. Empty
nested paragraphs do not manufacture extra blank rows.

Put all new regressions in `apps/backend/internal/jira/adf_description_test.go`.
The existing `cloud_client_test.go` is already 743 lines, near the effective-line
limit. Reuse its `newMockServer`, `clientTo`, and `serverClient` helpers without
modifying that file. Every HTTP server registers immediate cleanup.

| Provider and transport | Input | Intended result and evidence | Unsupported input |
| --- | --- | --- | --- |
| Jira Cloud REST v3, ticket lookup | Nested ADF description plus transition response | Exact paragraph-separated `JiraTicket.Description`; independent HTTP regression | Existing tolerant converter fallback |
| Jira Cloud REST v3, watcher search | Nested ADF in search issues | Same exact description; independent HTTP regression, existing field-selection tests | Existing tolerant converter fallback |
| Server/Data Center REST v2 through the same client | Plain string, null; ADF if returned | Plain string unchanged, null empty; shared converter matrix and existing REST tests | Non-string/non-object description empty |
| Atlassian MCP and seeded mock | Their existing parsed descriptions | Their parser and result contracts remain outside this repair | No new format support asserted |

## Tests

| Acceptance criterion | Permanent evidence in `adf_description_test.go` |
| --- | --- |
| `.2`, `.3` | `TestCloudClient_GetTicket_NestedDescriptionParagraphs`: independently decoded HTTP issue and transition fixtures, exact description |
| `.2`, `.3` | `TestCloudClient_SearchTicketsForWatch_NestedDescriptionParagraphs`: independently decoded HTTP search fixture, exact description |
| `.3` | `TestExtractDescription_NestedParagraphs`: sibling list items, multi-paragraph item, nested ordered/bullet lists, blockquote, unknown containers |
| `.3`, `.4` | `TestExtractDescription_Compatibility`: inline/marked adjacency, explicit hard/soft breaks, literal text whitespace/newlines, top-level and nested empty nodes, malformed nodes, plain string, null and unsupported shapes |

The abbreviated criterion numbers above belong to
`AC-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001`.
Expected values are literal externally readable strings, not values obtained
from another invocation of the converter. Both HTTP regressions must fail
independently on the baseline; control cases must pass there.

## E2E evidence

The two real HTTP client regressions exercise JSON decoding through production
`issueToTicket` to the exact description consumed by launch presets and watcher
prompts. Existing watcher/interpolation code already forwards that string.
This is the faithful boundary for this conversion defect. Playwright's seeded
Jira mock bypasses the ADF converter, so no new browser test or browser suite is
required. No rendered UI changes or UI previews are part of this package.

## Documentation impact

Public Jira instructions already describe copied description content and
placeholders in `docs/public/integrations.md`. Paragraph conformance does not
change a setting, workflow instruction, public field, label, or screenshot.
Internal design and delivery records need updates; public docs need no edit.
If implementation exposes a material public-doc discrepancy, reconcile it
before delivery and run `node scripts/validate-public-docs.mjs` then. Do not run
unchanged public-doc validator unit tests for this prose-only package.

## Work orders

- [x] [Task 01: Preserve nested ADF paragraph boundaries](task-01-preserve-nested-adf-paragraphs.md)

One work order, no dependency waves or delegates. Its verification block owns
the independent RED/GREEN tests, complete affected Jira race package,
staged affected-package lint, catalog/spec validators, and the real
PR-documentation coverage preflight. No initial full-backend lint or unrelated
suite is required.

## Verification results

Design checkpoint passed:

- `python3 scripts/list-docs.py validate`: exit 0; 380 decisions and 1,582 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: exit 0; all specification files passed.
- Real `.github/scripts/pr-docs.cjs` `validateCoverage` preflight against the
  proposed production/test paths and current documents: exit 0, `covered`,
  `errors: []`. Used Node 24.18.0 from its existing installation; the initial
  bare `node` invocation could not start because its bin directory was absent
  from the shell PATH. No installation was needed.
- `git diff --check -- docs/specs/integrations docs/plans/jira-nested-description-blocks`:
  exit 0. Worktree status contains only the updated owning design and this new
  plan directory; no staged changes.

After the later explicit implementation release, the shared converter was
repaired with a deferred paragraph separator. The independent permanent HTTP
regressions both failed before the source change with the expected fused text;
all compatibility controls passed on the same unchanged production.

Implementation verification passed using Go 1.26.0, `GOTOOLCHAIN=local`,
`GOMAXPROCS=2`, `GOMEMLIMIT=512MiB`, `-race -trimpath -tags=fts5 -p=1 -count=1`:

- Independent ticket and watcher RED commands: actual joined exit 1 each,
  expected description mismatch, package 0.082s each.
- Baseline compatibility controls: actual joined exit 0, package 1.084s.
- Focused GREEN matrix: actual joined exit 0, package 1.087s.
- Complete `./internal/jira` package: actual joined exit 0, package 1.335s.
- Staged canonical `bash scripts/lint-go-changed` for the two owned Go files:
  actual joined exit 0, `0 issues.`
- Frozen workspace dependency install completed once using pnpm 9.15.9.

Final document validators and coverage preflight use the completed artifacts;
their results are recorded in the work order. Hosted delivery evidence belongs
to the external task plan and does not change these local implementation results.

## Risks

- Adding both paragraph and document delimiters can duplicate blank lines.
- Collapsing all newline runs would alter explicit breaks or literal text.
- A container allowlist would miss deeper or unfamiliar enclosing nodes.
- Skipped/empty nodes can accidentally consume a pending paragraph boundary.
- A converter-only regression can miss client wiring; retain both HTTP tests.
