---
spec: docs/specs/tasks/system-design/remote-contribution-tasks.md
created: 2026-08-13
status: complete
---

# Implementation Plan: Remote Contribution History Reconciliation

## Overview

Keep the Changes panel useful when the provider commit request fails. Share and retry identical
provider reads. Reconcile successful evidence by SHA and show commit provenance through accessible
colors instead of a provider-history warning.

The plan implements
[ADR-2026-08-13](../../decisions/2026-08-13-provider-history-changes-enrichment.md).

## Root Cause

`usePRCommits` owns request state inside each hook instance. Each mounted Changes or Git-action
consumer sends the same WebSocket request. A single rejection clears the provider list and sets an
error. `ChangesPanelBody` converts that internal error into the yellow warning shown in the report.

The relation classifier previously required an upstream head before it evaluated provider ancestry.
The unified commit merge also appended provider-only commits after the local list, even though the
provider API orders commits from oldest to newest. It gave these commits the same neutral marker as
shared commits.

## Invariants

- Provider history enriches the checkout view. It does not replace the checkout view.
- Missing provider evidence never proves alignment or divergence.
- Remote actions remain closed unless the available evidence proves that the action is safe.
- Commit reconciliation uses SHA identity only.
- Provider-only history never changes the checkout automatically.
- Color is not the only provenance cue. Every colored marker has matching accessible text.
- Confirmed divergence keeps its explicit version-resolution control.

## Design

### Shared provider evidence resource

Add a small module-level resource behind `usePRCommits`. Key entries by workspace, provider
repository, pull request, and provider sync version. Share one in-flight promise across identical
consumers. Keep a successful value for later consumers of the same evidence version.

Retry one failed request after a short bounded delay. A manual refresh starts a fresh read unless an
equivalent refresh already runs. Ignore results from an old key after the selected pull request or
sync version changes. Remove superseded entries so the resource does not grow with every sync.

Keep the final error in the hook result for classification and diagnostics. Do not clear valid local
Git state and do not render the error as Changes-panel copy.

### Evidence classification and action policy

Evaluate provider evidence in this order:

1. Require a selected pull request, a complete non-empty provider list, provider head, and local
   head.
2. Classify equal heads as `aligned`.
3. After equal heads, classify a complete provider list with local HEAD and a different/newer
   provider head as `provider_ahead`.
4. Use upstream identity and counts to prove `local_ahead` or `diverged`.
5. Use `unknown` when the remaining evidence cannot prove a relation.

A provider-ahead relation disables Push. It enables Pull only when `hasUpstream` is true. An unknown
relation disables remote Push and Pull until provider evidence is available. Disabled action tooltips
must describe the actual missing condition instead of reusing provider-history warning copy.

### Commit reconciliation and presentation

Reverse each repository's provider oldest-first list before merging it into that repository's
newest-first local timeline. Place provider-only commits before the shared history and mark them with
the existing `current_pr` presentation. Keep shared commits neutral. In a confirmed divergence, keep
the two history sections.
Use violet for current-PR markers and amber for local-checkout markers. Keep the existing emerald arrow
for ordinary unpushed local commits.

Expose a stable commit-provenance attribute for component and E2E assertions. Keep the current
translated accessible labels. Remove provider-error banner plumbing from `ChangesPanelData` and
`ChangesPanelBody`.

### Desktop and mobile contract

- **Desktop outcome:** The Changes timeline stays visible during a provider failure. Proven
  provider-only commits use violet. Proven checkout-only divergence commits use amber.
- **Mobile entry point:** The existing Changes panel and Git actions in the task top bar.
- **Nearest exemplar:** The existing desktop Git Changes scenarios and
  `mobile-pr-checkout-drift.spec.ts`.
- **Hierarchy and geometry:** No component hierarchy, scroll owner, safe-area rule, or touch target
  changes.
- **Shared logic:** Desktop and mobile use the same relation, merged commit model, and `CommitRow`.
- **Mobile proof:** The phone Changes timeline exposes the same provenance text and marker colors. It
  shows no provider-history warning.

## Tests

- Prove that concurrent hook consumers send one provider request.
- Prove that one failed request retries and a successful retry becomes complete evidence.
- Prove that a final failure keeps an internal error without a visible Changes warning.
- Prove that a complete provider list can establish `provider_ahead` without an upstream.
- Prove that provider-ahead without an upstream disables Push and does not enable Pull.
- Prove that incomplete provider evidence disables remote Push and Pull.
- Prove that provider-only commits appear newest first within each repository with current-PR provenance.
- Prove that shared commits stay neutral.
- Prove that confirmed provider commits use violet and local-checkout commits use amber.
- Prove the same visible and accessible state in desktop and mobile E2E tests.

## Implementation Waves

Wave 1 (complete):

- [x] [Task 01: Recover provider commit evidence](task-01-recover-provider-commit-evidence.md)

Wave 2 (complete):

- [x] [Task 02: Reconcile and style commit provenance](task-02-reconcile-and-style-commit-provenance.md)

Wave 3 (complete):

- [x] [Task 03: Verify desktop and mobile reconciliation](task-03-verify-desktop-mobile-reconciliation.md)

Run the tasks in order in the primary conversation. Task 02 consumes the resource contract from Task
01. Task 03 validates the completed behavior through real browser surfaces.

## Risks

- A cache key that omits the provider sync version can show stale history after the branch moves.
- A stale response can replace evidence for a newly selected pull request without an ownership guard.
- Provider ancestry can prove a relation without proving that Pull has a valid destination.
- Color-only styling can hide provenance from users who do not distinguish the selected colors.
- Removing the warning must not convert unavailable evidence into permission for a remote mutation.

## Verification

Run the focused commands in each task. After all tasks pass, run:

```bash
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm run i18n:ratchet
git diff --check
```

## Results

- Task 01 shares provider reads by workspace, repository, pull request, and sync version; retries one
  failure; retains the final error internally; ignores stale results; and bounds cache entries.
- Task 02 reconciles complete provider evidence by SHA, preserves checkout history, sorts provider-only
  commits newest first within each repository, adds accessible provenance markers, removes the
  provider-history body warning, and keeps remote actions fail-closed.
- Task 03 passed the desktop Git Changes spec with 21 tests and the mobile rewritten-contribution spec
  with 1 test. Both surfaces expose the same provenance labels without horizontal overflow.
- Review remediation adds repository-scoped newest-first assertions for commit 15 and deterministic
  desktop/mobile browser coverage for one provider failure followed by automatic reconciliation. The
  focused remediation scenarios passed 1 desktop test and 2 mobile tests; the helper unit file passed
  10 tests and the mock-provider backend tests passed 2 tests.
- Final focused tests passed 59 tests across 6 files. Typecheck, lint, i18n checks, i18n ratchet, and
  `git diff --check` passed.
- PR fixup hardened unavailable-evidence action gates, repository-scoped ordering, single-repository
  contribution callbacks, empty-SHA matching, and fail-closed E2E assertions.

Install workspace dependencies first in a fresh worktree:

```bash
cd apps && pnpm install --frozen-lockfile
```
