---
created: 2026-10-08
status: implemented
requirements:
  - REQ-INTEGRATIONS-GITLAB-INTEGRATION-001
system_design:
  - ../../specs/integrations/system-design/gitlab-integration-02.md
legacy_specs: []
---

# Implementation plan: Preserve GitLab discussion reply drafts

## Overview

Prevent a successful reply A from erasing later unsent draft B in the same
discussion. One sequential work order authors an independent causal regression,
changes only local successful-settlement logic, and verifies the real rendered
section through the production hooks/API with transport-only mocks.
ROOT reviewed the concrete four-file package and later granted same-primary
implementation, exclusive global local-heavy83 and normal delivery. Local
implementation and all task-defined local checks are complete. Publication,
hosted review, CI and separate ROOT merge authority remain pending.

## Ownership and scope

Integrations owns the provider discussion reply lifecycle and its visible
outcomes. Extend the existing
[requirement](../../specs/integrations/requirements/gitlab-integration.md)
with AC-INTEGRATIONS-GITLAB-INTEGRATION-001.11-.15 and the existing
[part-2 design](../../specs/integrations/system-design/gitlab-integration-02.md#discussion-reply-draft-settlement).
Part 2 has room; part 1 is already 31,871 bytes near its 32 KiB limit.
The completed GitLab parity package's task 06 owns the original review surface;
this focused follow-up leaves its completed scope and historical results intact.
No parallel incident, UI specification, or architectural decision is needed.

In scope: raw submitted snapshot, latest-value success clearing, and causal
rendered coverage for edits/clear/whitespace, failure/retry/refresh and separate
discussion drafts. Sole production ownership is
`apps/web/components/gitlab/mr-discussions-section.tsx`.

Out of scope: global action/draft frameworks, edit histories, changed pending
policy, lifecycle/source/loading behavior, identity switches, persistence after
unmount/reload/removal, backend/public API, other providers/actions, layout,
copy/translations, instrumentation and additional packages. No production or
permanent test changes, installs, Vitest/typecheck/lint/build/browser/Go,
commits or PR operations occurred during the completed design turn. Subsequent
execution followed ROOT's later explicit release.

## Baseline and accepted evidence

Current HEAD and supplied base: `5638725e5d22d8089269fc53a5b79ed61d9b9445`.
Audited source blobs: discussion section `b6ebd9e75da353e9609a8fb39918f30471557b58`,
detail panel `365c82bb59dcdefc310a9e3aa788c8b655ea2756`, action hook
`1dac7d4805fea785b2eda9a141bee3e6c088062b`, feedback hook
`b390b893934a588b4c34369e7ed77d39662d6b99`, API
`8f37febf474e9962423dbb2e881bfbd4ff1e726a`.

ROOT's protected regular 0400 candidate is
`/tmp/kandev-root-gitlab-reply-draft-discovery-20261008/candidate.test.tsx`,
SHA256 `d4f33350f89c8fce40c042e68624945a2e78c28b99ac5257a98401fd1f0ee1e4`.
Its sibling `qualified-proof.json` and `selected-project-output.log` record
one causal continued-draft failure (expected B, actual empty), plus passing
unchanged-success and POST-failure controls. The actual browser-locales
single-file Vitest run was native session 36559, start chunk ca27e0, actual
join f8469f, exit 1, PID/PGID 4055587, started 10:53:31.793039Z and finished
10:53:39.737671Z on 2026-10-08; cutoff 10:59:41.793039Z. Receipt reports
group absent, temporary test removed and ROOT clean. Earlier session 75690
selected the wrong project and produced NO_TESTS; it is not causal evidence.
The protected fixture and receipt were inspected read-only, not replayed,
copied or imported. Preserve them until ROOT independently releases them.

## Technical approach and consumer audit

Capture raw reply text before deriving the trimmed POST body. On true
`onReply` settlement, use a pure functional setter that clears only an exact
latest-value/raw-snapshot match. Return differing current text unchanged.
Use raw equality so added spaces/newlines survive; a value restored exactly
to the snapshot is eligible to clear. Keep false-result behavior unchanged.
The GitLab settings token save uses a functional current-value comparison as
a nearby source precedent; its trimmed comparison is unsuitable for drafts.

| Boundary | Current contract | Planned evidence |
| --- | --- | --- |
| `Discussion` | Local reply state; enabled textarea; Reply blocked by busy/blank | Raw snapshot clearing and independent per-discussion state |
| `MRDiscussionsSection` | `Discussion` keyed by `discussion.id` | Same ID across refresh/reorder preserves correct textarea |
| `MRDetailContent` / `discussionHandlers` | Sole production section consumer; binds identity and forwards discussion/body | Read-only binding audit and faithful harness with identical real hook/API composition |
| `useMRActions` | POST success toast, schedules refresh, true/false result, releases busy | Successful POST remains successful if refresh fails; error permits retry |
| `useMRFeedback` | Cached same-identity feedback during refresh | Held and replaced feedback objects do not erase later draft |
| `createMRDiscussionNote` | Workspace/expected-host query; project/IID/discussion/body POST | Assert actual transport payload, including trimmed body and target discussion |

Only the section calls `Discussion`; only the detail panel consumes the
section. Other hook users (CI popover and merge button) need no edits.
No unsupported-provider fallback or transport-shape change is introduced.

## Tests

Use `components/gitlab/mr-discussions-section.reply-draft.test.tsx` for new
independently authored component integration tests; retain the existing
`mr-discussions-section.test.ts` context helpers as affected controls.

| AC suffix | Planned named regression / control |
| --- | --- |
| .11, .12 | `preserves continued draft after successful post and refresh`; verify pending controls, POST identity/body, success feedback, refreshed posted note and exact B |
| .11, .12 | `clears unchanged successful raw draft`; padded/newline input sends trimmed body and clears |
| .11, .12 | `preserves whitespace-only edits to submitted draft`; `preserves intentional clear while posting`; `does not submit blank draft` |
| .12 | `clears draft restored exactly to submitted snapshot` |
| .13 | `retains unchanged draft after post failure`; `retains edited draft after post failure and retries current text`; `retains intentional clear after post failure` |
| .14 | `retains continued draft across delayed refresh`; `retains continued draft when refresh fails`; `keeps unchanged successful draft cleared when refresh fails` |
| .15 | `keeps other discussion draft through reply settlement and refreshed reorder` |

Mock only global fetch. Use real Tooltip/Toast providers, locale setup,
section/hooks/API; mirror the detail-panel reply callback without duplicating
the setter under test. Assert actual rendered textareas and request payloads,
not source strings or mocked onReply results. Hold POST and refresh separately;
use causal settlement, article-scoped selectors and complete cleanup.

## E2E and mobile assessment

End-to-end evidence for this local boundary is the rendered section through
real providers/hooks/API to a mocked HTTP transport, including refresh. There
is no browser navigation or geometry change. Both existing desktop row and
phone stacked reply surface use this same local state, so the mobile-parity
pure-state exception applies. No new Playwright test, ASCII layout proposal,
browser/build or mobile geometry audit is needed.

## Public documentation assessment

`docs/public/integrations.md` already describes discussion replies, the refresh
action and upstream permissions without requiring users to avoid editing
while posting. The root README and screenshot catalog introduce no draft
clearing guidance. Commands, API, config, terminology, navigation and
screenshots do not change; no public guidance update is needed. This package
updates internal behavior/design only.

## Work orders

- [x] [Task 01: Preserve the latest discussion reply draft](task-01-preserve-reply-draft.md) (local implementation)

One wave, one sequential work order, no dependencies on another work order.
ROOT's explicit implementation and global local-heavy release were received
after the design checkpoint. No agents/tasks/sessions/tabs/model switch are
authorized.

## Verification results

Design checks passed on 2026-10-08: catalog validation (364 decisions, 1,456
specifications), all 36 spec-validator tests, full specification lint and
four-file whitespace/status checks. Repository documentation preflight returned
`ok: true`, `errors: []` for actual four-doc changes (exempt) and separately
labelled planned production-section trigger (covered). Its first planned-trigger
check exposed the draft subsection at the requirement heading level; lowering
that subsection beneath its owning REQ fixed the reference grouping. All
REQ/AC/design/manifest links now pass; no validator or contract was weakened.
The independent permanent RED reproduced exactly one causal continued-draft
loss with two passing unchanged-success/failure controls against the original
production blob. Only `Discussion.submitReply` then changed: raw submitted
snapshot plus functional latest-value comparison on success. Targeted GREEN
passed 15 tests (13 new rendered regressions plus two existing context-helper
controls). An owned fixture lint repair split long test groups and reused the
success notice; affected GREEN and changed-file ESLint passed afterward.
Normal project typecheck including pretypecheck generation and i18n:ratchet
passed. The latter reports one modified production file clean, with no new
copy. A final whitespace-only fixture formatting adjustment changes no test
behavior. Original command receipts live in `/tmp/kandev-child83-exec-20261008`
and the live task plan. Final documentation checks passed: catalog (364
decisions, 1,456 specifications), all 36 validator tests, full spec lint,
actual changed-file reference coverage (`ok: true`, `errors: []`), four-file
reference/whitespace checks and `git diff --check`. Local status does not
claim hosted review, CI or merge complete.
Exact later checks and native process requirements are in
[Task 01](task-01-preserve-reply-draft.md#verification).
Do not promote unrelated migrated design lifecycle or declare implementation
complete from this package. The live task plan records identities, ROOT-only
resource/delivery/merge gates, checkpoints, user edits and next action.

## Risks

- Comparing trimmed text would destroy whitespace edits; compare raw snapshot.
- A mock onReply-only test misses production action/refresh composition.
- Refreshed objects must keep stable IDs; removed discussions and identity
  switches are excluded, rather than introducing lifecycle machinery.
- Dependencies are absent. One conditional frozen install requires ROOT's
  later heavy release; no install or setup repair is authorized during design.
- Resource/timeout/transport/unknown/out-of-scope failures checkpoint ROOT;
  no automatic retries, cache wipes, foreign kills or broadened checks.
