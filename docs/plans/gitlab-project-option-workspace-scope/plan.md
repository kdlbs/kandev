---
created: 2026-10-06
status: in_progress
requirements:
  - REQ-INTEGRATIONS-GITLAB-INTEGRATION-001
system_design:
  - ../../specs/integrations/system-design/gitlab-integration-01.md
legacy_specs: []
---

# Implementation Plan: GitLab project choices by workspace

## Overview

Keep the project dropdown's accumulated membership within its active workspace
and browse context. One sequential work order adds workspace identity to the
existing reset tuple and proves the outcome through the real page-state and
toolbar. The design checkpoint completed before ROOT reviewed the four full artifacts
and authorized implementation in this same primary session on 2026-10-06.

## Scope

### In scope

- AC-INTEGRATIONS-GITLAB-INTEGRATION-001.5, .8 and .10: workspace browse scope,
  MR/issue choices, same-context page accumulation and explicit-filter inclusion.
- Existing page-state context wiring and independently authored transport-boundary
  regression tests, with minimal updates to existing direct callers in tests.

### Out of scope

- Request-order, cancellation, credential, backend, API, global cache or
  coordinator changes; unrelated GitHub behavior or forced project-filter reset.
- Production changes to the toolbar, rows, accumulator, saved presets or other
  consumers without new causal evidence and ROOT scope release.
- The parked unused profile-toggle hook candidate. No profile artifacts were
  authored; its protected proof remains read-only and makes no reachable-impact
  claim on this checkout.

## Evidence and production path

ROOT's accepted supported-event proof at baseline
`0235c4f833dccdd4cdbaaa03cfc9b2c1c9b5057f` produced two causal failures and two
passing controls. After workspace B's own MR response settled, A's `alpha/first`
remained alongside B's `beta/only` in state and as an actual Radix toolbar option.
The causal case had no selected project filter. Same-workspace page accumulation
and an initial independent B both passed. Native handle `36813` was actually
joined, exit 1, in 6.072s (2026-10-06 16:42:16 to 16:42:22 UTC).

The initial fixture's missing test-only user-event import was a setup failure:
zero tests ran (native `37723`, exit 1). The separately approved existing
`fireEvent` fixture supplies the accepted causal evidence. Neither protected
fixture may be copied, replayed, modified or deleted. Full receipts and hashes
remain in the live task plan and ROOT's `/tmp` archives.

Current design checkout: `1d8362ecbd4fe5a2cf0827ec4a8315a123316a13`.
Relevant source blobs still match the accepted proof:

| Source | Blob |
| --- | --- |
| `app/gitlab/use-gitlab-page-state.ts` | `5de1af6e84809366b9e0f2f526d32493be240023` |
| `components/gitlab/my-gitlab/use-known-projects.ts` | `9db71d5239749510482f737bbf29500ad0442b86` |
| `components/gitlab/my-gitlab/list-toolbar.tsx` | `e938a15cac016d3f70b626c70deef85b6796177e` |

`GitLabPageClient` calls `useGitLabPageState` with `scope.workspaceId` and feeds
its `projectOptions` to `ListToolbar`. The hook passes workspace identity to
search but omits it from project-option derivation. Identical selection, query
and milestone therefore reuse one accumulator across A and B. Search already
masks rows by workspace; the proof concerns project-dropdown contamination.

## Technical approach

In `apps/web/app/gitlab/use-gitlab-page-state.ts`, add an explicit workspace
input to `UseProjectOptionsArgs` and `buildProjectOptionsResetKey`, and forward
the normalized workspace from `useSearchAndProjects`. Include it in the existing
JSON tuple. Preserve committed-key/loading guards, page accumulation, sorting,
deduplication and selected-filter inclusion. Update existing direct test callers
for the input without adding implementation-mirror assertions or a framework.

No persistence, migration, permission, request sequence or API change is needed.
The module accumulator remains one active context; the design promises no
multi-instance or cross-tab cache isolation. The normal page search and existing
workspace page-reset work remain outside this repair's production ownership.

## Tests

Independently authored permanent test file:
`apps/web/app/gitlab/use-gitlab-page-state.workspace.test.tsx`, suite
`GitLab project options workspace scope`. Use real page-state, search, accumulator,
store/provider, locales, toolbar/Radix and native MR/issue rows. Partial transport
mocks preserve unrelated exports. No mocked hook, store, component, copied
predicate, source-text assertion or new test library.

| Evidence | Acceptance |
| --- | --- |
| `excludes previous workspace projects after the current MR response settles` | .5, .10; causal RED |
| `offers only current workspace projects in the real toolbar` | .8, .10; causal RED |
| `retains earlier-page projects in the same workspace` | .8, .10; positive control |
| Initial B, MR and issue A-B-A/B-A transitions, empty replacement workspace | .5, .10 |
| Equal-input reuse, page turn, query/milestone/kind reset, explicit selected filter | .8, .10 |
| Loading/empty/error/refresh/disabled search, current stale-response behavior | .5, .8, .10 compatibility |

Existing `use-gitlab-page-state.test.ts`, `use-known-projects.test.ts`,
`use-gitlab-search.test.ts` and `list-toolbar.test.tsx` are the nearby controls.
Change only the necessary direct test arguments. Use deferred native promises
and settled response assertions; do not infer outcomes from elapsed sleeps or
promise exact request counts beyond established baseline behavior.

## Mobile and public documentation audit

Both desktop and phone consume the same project-option state. The mobile-parity
pure state/data exception applies: no toolbar/row composition, copy, layout,
touch, scrolling, navigation or breakpoint change. Real-consumer component
coverage is the rendered check. No browser/build/E2E/screenshots or ASCII layout
preview is needed for this unchanged surface.

`docs/public/integrations.md` already describes workspace-scoped GitLab browse
and current-page client-side project narrowing with pagination. The root README
and screenshot catalog introduce no conflicting filter contract. Public guidance
remains accurate; this package changes internal requirements/design only.

## Work orders

- [ ] [Task 01: Scope project choices to the workspace](task-01-scope-project-options.md)

Task 01 has no dependency and runs sequentially in the same primary session.
Its exact bounded RED, GREEN, nearby-control, ESLint, normal project typecheck,
i18n and documentation commands are in the work order. Global local-heavy
authorization is a prerequisite for every package command, install and hook.

## Verification results

Local implementation and verification completed on 2026-10-06. The independent
anchored RED produced two causal failures and one passing accumulation control
before the correction. Focused GREEN passed 32 tests, and the 14 new regressions
passed again after test lint corrections. The three unchanged consumer/search/
accumulator control files passed 31 tests. Changed-file ESLint, normal project
typecheck (including its existing pretypecheck), i18n check and new-code ratchet
passed. The ratchet covered the changed production page-state file; tests are
excluded and no user-facing copy was added.

Design catalog, all-spec lint, 36 linter tests and actual/prospective reference
coverage passed at their historical checkpoints. Publication documentation
coverage and whitespace checks use the actual seven changed paths. Exact original
command handles, terminal receipts, full logs and artifact hashes remain in the
live task plan. The conditional frozen install ran once because dependencies
were absent. No tracked dependency or generated asset changed.

Hosted CI, substantive current-head review, merge authorization and joined
cleanup remain pending. The package stays in progress until those delivery
gates complete. Local success is not hosted or merge evidence.

## Risks

- A reset must clear prior workspace membership without repopulating it from
  previous-context items before search starts; retain the existing guards.
- Selected project inclusion is intentional even across workspaces. Tests must
  distinguish that selected value from unrelated accumulated options.
- The module store supports one active context. A new cache or simultaneous
  consumer ownership policy would require separate scope and design.
- Future base changes require read-only comparison of relevant contracts. Do
  not rebase or replay ROOT's proof merely to refresh evidence.
