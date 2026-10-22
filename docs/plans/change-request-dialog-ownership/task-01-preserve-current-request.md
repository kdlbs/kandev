---
id: "01-preserve-current-request"
title: "Preserve current change-request form"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-NATIVE-CREATE-001
acceptance_criteria:
  - AC-INTEGRATIONS-NATIVE-CREATE-001.1
  - AC-INTEGRATIONS-NATIVE-CREATE-001.2
  - AC-INTEGRATIONS-NATIVE-CREATE-001.3
  - AC-INTEGRATIONS-NATIVE-CREATE-001.4
  - AC-INTEGRATIONS-NATIVE-CREATE-001.5
system_design:
  - ../../specs/integrations/system-design/native-change-request-creation.md
---

# Task 01: Preserve current change-request form

## Summary and scope

Fence completion setters by native form ownership. Add permanent regression
coverage through the actual provider, API and WS boundary, registered-provider
retry, and shipped desktop/phone controls. Preserve feedback and original scope.

## Exclusions

Commit dialog changes; utility generation; backend/provider/auth/schema changes;
Kanban, Jira and Sentry; publication; native subagents and platform workers.

## Acceptance

- Stale success/failure/rejection/partial completion preserves exact replacement
  draft, focus/selection, scope and next Submit/Cancel, including equal-value
  same-scope replacement and session/task away/back.
- Owned success/failure controls retain current policy; owned provider partial
  retry skips push while new repository creation pushes independently.
- Real desktop and emulated phone touch tests exercise shipped entry, with
  guarded unique offline source/workspace/workflow and finally cleanup.

## UI-01: Shared create dialog

See [combined preview](plan.md#ui-01-shared-create-dialog). Desktop Changes/menu
and phone bottom navigation Changes open the same form:

```text
Create Pull Request (repository)
Title [current raw title]
Description [current raw description]
[x] Create as draft
[Cancel] [Create PR]
```

AC-INTEGRATIONS-NATIVE-CREATE-001.2/.4/.5: old completion leaves this replacement
usable; only an owned partial failure replaces Create with Retry and its notice.

## Verification

Run from repository root, independently rooted commands:

```bash
(cd apps/web && pnpm exec vitest run components/vcs/vcs-dialogs.change-request.test.tsx components/vcs/vcs-dialogs.commit.test.tsx lib/plugins/change-request-creation.test.ts hooks/use-change-request-provider-target.test.tsx components/vcs-split-button.test.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium -- tests/git/change-request-ownership.spec.ts --workers=1 --retries=0)
(cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome -- tests/git/mobile-change-request-ownership.spec.ts --workers=1 --retries=0)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/vcs/vcs-dialogs.tsx components/vcs/use-create-change-request-handler.ts components/vcs/use-pr-dialog-state.ts components/vcs/vcs-dialogs.change-request.test.tsx e2e/tests/git/*change-request-ownership*)
(cd apps/web && pnpm run i18n:check)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The mobile command reuses the exact frontend build from the desktop command. Use
Node 24 and at most the documented 8 GiB heap if needed. Go builds use
GOMAXPROCS=2 and GOFLAGS=-p=2. No overlapping full suites.

## Files likely touched

- `apps/web/components/vcs/vcs-dialogs.tsx`
- `apps/web/components/vcs/use-create-change-request-handler.ts`
- `apps/web/components/vcs/use-pr-dialog-state.ts` if extraction is needed
- `apps/web/components/vcs/vcs-dialogs.change-request.test.tsx`
- `apps/web/e2e/tests/git/*change-request-ownership*`
- `docs/public/git-operations.md` (small creation/reopening behavior note)

## Dependencies and parallelism

None. Sequential in this task/session only.

## Risks

Scope resolution and live task association must use admitted identity. Browser
causality must hold actual completion without real provider traffic. Keep all
owned handles until terminal and remove only owned tasks/worktrees/resources.

## Results

- The package timestamp receipt precedes all permanent tests and production edits.
- Causal transport RED: 2 intended failures and 1 owned-partial control passed.
  Expanded RED also exposes equal-value/scope-round-trip ownership failures.
  A later dedicated RED proves retained retry state crosses session changes.
- Initial focused run: 5 files, 60 tests passed, including 19 creation ownership
  tests, real registered-provider retry/push consumers, unchanged commit tests,
  provider-target resolution and native split-menu callbacks.
- Browser causal RED: real mock GitLab success clears the reopened raw title.
  Final rebuilt Chromium scenario passed (8.0 seconds in the final build run),
  and Pixel 5 mobile-chrome touch scenario passed (6.4 seconds), retries zero.
  These are browser emulation and loopback mock-provider evidence, not physical
  device, native desktop OS, real-provider, incident, or zero-flake evidence.
- Final TypeScript typecheck (including catalog generation check), ESLint with
  zero warnings, and complete i18n checks passed. No product copy was added.
- Public docs: small how-to clarification in `docs/public/git-operations.md`.
  Public-doc validator tests (63), page validation (54 pages), spec catalog,
  spec lint and documentation coverage preflight passed.
- Numeric instrumented coverage was not collected: the workspace does not
  install `@vitest/coverage-v8`. Acceptance coverage is exercised by the tests
  above; no dependency or guard changes were introduced.
- Browser fixtures guard both source HEAD/full raw porcelain and worker seed;
  unique source, bare origin, task, workspace/workflow and secondary worktree
  cleanup runs even after assertion failure. Final runs completed cleanup.
- Earlier working-directory/version/selector/payload/cleanup setup failures are
  preserved separately and do not count as causal RED.
- Publication is pending parent validation and a separate same-session grant.
  The local commit/hook receipt and exact full diff are supplied externally.

## PR review remediation

CodeRabbit identified that the global active task can differ from the caller's
known task while the session row loads. Pass that known task from the task page
and Office dockview into the native provider; resolve hydrated session owner,
then known caller, then the legacy fallback. This extends the same admitted
identity boundary under AC-INTEGRATIONS-NATIVE-CREATE-001.1 and .3.

Regression RED: two intended failures (URL associated with unrelated active task;
old partial completion reopens after known-owner away/back), with 20 controls
passing. The final focused suite has 63 tests, including 22 creation ownership
cases. Final static, rendered desktop/phone checks and hooked remediation commit
are recorded in the external delivery receipt; earlier 60-test evidence and
screenshots are historical for the initial head.
