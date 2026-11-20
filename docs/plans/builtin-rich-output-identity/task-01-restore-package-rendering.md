---
id: "01-restore-package-rendering"
title: "Restore built-in package rendering"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-001
  - REQ-AGENTS-AGENT-RICH-OUTPUT-002
  - REQ-UI-KANDEV-MCP-TOOL-RESULTS-001
acceptance_criteria:
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.1
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.13
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.17
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.18
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.6
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.9
  - AC-UI-KANDEV-MCP-TOOL-RESULTS-001.2
  - AC-UI-KANDEV-MCP-TOOL-RESULTS-001.3
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
  - ../../specs/ui/system-design/kandev-mcp-tool-results.md
---

# Task 01: Restore built-in package rendering

## Summary

Recognize the exact built-in server names in the existing shared tool-identity
parser. Restore native report dispatch and standalone grouping for saved and
live messages without changing their payloads.

## In scope

Use `/tdd`. First add a synthetic version of the observed persisted message.
Its name is `kandev-rich-output/show_rich_output_kandev`, status is `complete`,
input contains a valid version 2 Stack with two FilePreview elements, and output
contains `structuredContent: null` plus `Rich output accepted.` text content.
Assert `kandevToolStemOf` returns `show_rich_output`, native dispatch succeeds,
and grouping retains the report between ordinary tool groups. Run this test
before modifying production code and record the expected failure.

Implement exact built-in server matching in `parse.ts`. Preserve the historical
provider fallback guard in `types.ts`: only `kandev` permits title fallback.
Test all five server names and existing identity formats. Test longer prefixes before `kandev-`.
Preserve bare and monolithic compatibility. Unknown stems must not select a
renderer. Preserve foreign normalized-name precedence and foreign wrapper guards.

Add renderer cases for FilePreview, Table, and StatusList. Add one desktop and
one phone browser scenario with the exact package-qualified normalized name,
actual disposable image files, a table, neighboring activity, and reload.
Reuse existing image-preview helpers and transcript fixtures. Use ordinary
short titles so title validation cannot hide an identity failure.

## Out of scope

No backend, adapter, storage, schema-limit, authorization, or visual redesign.
Do not mutate the reported task, rerun its tools, or copy its live data into tests.
The separate original table report remains outside this confirmed repair.

## Acceptance

1. Accepted package-qualified reports use native rendering and standalone
   grouping live and after reload, with every supplied value preserved.
2. Legacy messages still render. Explicit foreign, lookalike, malformed, and
   unregistered identities remain generic. Invalid payloads remain invalid.
3. Desktop and phone show the same report content and existing file controls.

## UI outcome

Use the existing composition described in [the plan](plan.md#ui-outcome).
The transcript owns vertical scrolling. Existing cards retain their image
visibility, touch actions, and file-opening behavior. No new visual surface is
introduced. Confirm the two preview images and table remain visible on phone.

## Verification

Run from the repository root. Install dependencies from `apps/` first if absent.

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/messages/kandev/parse.test.ts components/task/chat/types.test.ts components/task/chat/messages/kandev-tool-message-rich-output.test.tsx hooks/use-processed-messages-rich-output.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/task/chat/messages/kandev/parse.ts components/task/chat/types.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/builtin-rich-output.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/chat/mobile-builtin-rich-output.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run E2E commands sequentially. The first managed command builds current code.
The second reuses that build only if no product code changed between commands.
Record actual results and any verification blocker. Do not mark the task done
until the required tests pass.

## Files likely touched

- `apps/web/components/task/chat/messages/kandev/parse.ts`
- `apps/web/components/task/chat/messages/kandev/parse.test.ts`
- `apps/web/components/task/chat/types.ts`
- `apps/web/components/task/chat/types.test.ts`
- `apps/web/components/task/chat/messages/kandev-tool-message-rich-output.test.tsx`
- `apps/web/hooks/use-processed-messages-rich-output.test.ts`
- `apps/web/e2e/tests/chat/builtin-rich-output-helpers.ts`
- `apps/web/e2e/tests/chat/builtin-rich-output.spec.ts`
- `apps/web/e2e/tests/chat/mobile-builtin-rich-output.spec.ts`
- This plan and work order, for status and results.

## Dependencies

None.

## Risks

Keep exact server recognition separate from broad reserved-name policy.
Test the actual normalized name, not only title fallback or bare-name fixtures.

## Parallelism

`sequential`

## Inputs

- [Root cause and scope](plan.md).
- [Native rich-output requirements](../../specs/agents/requirements/agent-rich-output.md).
- [Shared MCP result requirements](../../specs/ui/requirements/kandev-mcp-tool-results.md).
- [Package identity design](../../specs/ui/system-design/kandev-mcp-tool-results.md#built-in-package-namespaces).
- Existing Copilot rich-output grouping and renderer tests.
- `apps/backend/internal/mcp/profile/builtin_descriptors.go` for exact server names.

## Results

Completed on 2026-11-05.

- RED: the four focused unit suites reported 8 expected failures and 75 passes
  before production changes. Failures covered package identity, provider guards,
  renderer dispatch, and standalone grouping.
- GREEN: the same four suites passed all 83 tests after the parser and provider
  guard changes. The grouping fixture uses two ordinary messages on each side,
  because one ordinary message correctly remains a standalone message.
- Browser: Chromium passed one scenario (7.0 seconds); mobile Chrome passed one
  scenario (5.9 seconds). Both prove live updates and reload, two successfully
  loaded preview images, retained table values, chronological activity groups,
  and foreign-name fallback. Phone also checks existing touch-target sizes and
  the compact table. Both check that the page has no horizontal overflow.
- Static checks: `pnpm run typecheck` passed, including rich-output schema
  consistency. Focused Oxlint passed for both production files and all three
  new E2E files. Specification inventory, specification lint, whitespace checks,
  and actual changed-file documentation coverage passed.

The first managed E2E command built the backend, fixture plugin, and web assets.
Subsequent runs used `--no-build`; production code did not change between them.
Initial setup attempts failed because the shell inherited the running service's
network configuration and Chromium lacked NixOS library search paths. Test
processes removed `KANDEV_INTERNAL_CONFIG_FILE`,
`KANDEV_INTERNAL_CONFIG_HOME_FILE`, and `KANDEV_INTERNAL_AGENTCTL_STARTUP_CONFIG`.
Their `LD_LIBRARY_PATH` contained the installed `/nix/store` library directories
required by Chromium. These were process-local settings; no service configuration
or system libraries were changed. One test-fixture correction moved `watchWs`
before navigation, as required by that helper. No assertion was removed.

The passing browser commands, with that isolated environment, were:

```bash
(cd apps/web && pnpm e2e:run --no-build --project chromium tests/chat/builtin-rich-output.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/chat/mobile-builtin-rich-output.spec.ts)
```

The user instance and its stored messages were not changed. Deployment of the
patched frontend is still needed there. This work does not resolve the separate
original 0.98.0 table report with its over-limit copied title.


## PR preparation

On 2026-11-06, the parser was checked directly from remote `main` at
`ce9918b094855c38e5b3b4c79c1b565fe5c379cb`. The new parser regression suite
reported four expected package-name failures and 33 passes. Both affected
production files were unchanged between the original base and that revision.
The branch was fast-forwarded to that main revision before restoring the fix.
The four focused unit suites passed again (83 tests), and type checking passed.
The managed runner rebuilt current backend and web assets. Disposable capture
specs reran the same live/reload assertions on Chromium (one pass) and mobile
Chrome (one pass), then captured synthetic report screenshots for the PR. The
capture specs were removed; images remain in the ignored PR-assets directory.
The commit hook found that the expanded renderer test group exceeded its
function-length limit. The new case now has its own test group; the renderer
suite passed again without changing its assertions or production code.

## Review remediation

Cubic identified that legacy imports preserve external definitions with reserved
package names. The historical provider fallback guard now keeps its original
`kandev`-only rule. Normalized qualified package calls retain native rendering.
Four added provider-collision cases failed before the correction; this directly
reproduced the finding. The unused exact-server predicate was removed.

Cubic and Claude also noted the legacy hyphenated syntax. Compatibility remains
unchanged; the design now distinguishes exact qualified namespaces from legacy
hyphenated tool stems. The full renderer test confirms that a hyphenated unknown
stem cannot select a native renderer. No new authorization or provenance claim
is derived from this presentation selector.

Validation after remediation: all four focused unit suites passed (87 tests),
and `pnpm run typecheck` passed. The existing desktop/phone evidence remains
applicable to the unchanged qualified report path; only ambiguous historical
provider fallback is narrowed. No visual component or layout changed.

CodeRabbit identified the equivalent historical Codex `raw_input.server` case.
The fallback guard now checks both explicit server fields. Three new Codex cases
failed before this correction; monolithic `kandev` remains a passing control.
The final four focused suites pass all 91 tests, and type checking passes.
The design and implementation plan now describe both historical server fields.
