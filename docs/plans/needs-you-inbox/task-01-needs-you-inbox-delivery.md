---
id: "01-needs-you-inbox-delivery"
title: "Needs-you Inbox delivery"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-NEEDS-YOU-INBOX-001
acceptance_criteria:
  - AC-UI-NEEDS-YOU-INBOX-001.1
  - AC-UI-NEEDS-YOU-INBOX-001.2
  - AC-UI-NEEDS-YOU-INBOX-001.3
  - AC-UI-NEEDS-YOU-INBOX-001.4
  - AC-UI-NEEDS-YOU-INBOX-001.5
  - AC-UI-NEEDS-YOU-INBOX-001.6
  - AC-UI-NEEDS-YOU-INBOX-001.7
  - AC-UI-NEEDS-YOU-INBOX-001.8
  - AC-UI-NEEDS-YOU-INBOX-001.9
  - AC-UI-NEEDS-YOU-INBOX-001.10
  - AC-UI-NEEDS-YOU-INBOX-001.11
  - AC-UI-NEEDS-YOU-INBOX-001.12
  - AC-UI-NEEDS-YOU-INBOX-001.13
  - AC-UI-NEEDS-YOU-INBOX-001.14
  - AC-UI-NEEDS-YOU-INBOX-001.15
  - AC-UI-NEEDS-YOU-INBOX-001.16
  - AC-UI-NEEDS-YOU-INBOX-001.17
  - AC-UI-NEEDS-YOU-INBOX-001.18
  - AC-UI-NEEDS-YOU-INBOX-001.19
  - AC-UI-NEEDS-YOU-INBOX-001.20
  - AC-UI-NEEDS-YOU-INBOX-001.21
  - AC-UI-NEEDS-YOU-INBOX-001.22
  - AC-UI-NEEDS-YOU-INBOX-001.23
  - AC-UI-NEEDS-YOU-INBOX-001.24
  - AC-UI-NEEDS-YOU-INBOX-001.25
  - AC-UI-NEEDS-YOU-INBOX-001.26
  - AC-UI-NEEDS-YOU-INBOX-001.27
  - AC-UI-NEEDS-YOU-INBOX-001.28
  - AC-UI-NEEDS-YOU-INBOX-001.29
  - AC-UI-NEEDS-YOU-INBOX-001.30
  - AC-UI-NEEDS-YOU-INBOX-001.31
  - AC-UI-NEEDS-YOU-INBOX-001.32
  - AC-UI-NEEDS-YOU-INBOX-001.33
  - AC-UI-NEEDS-YOU-INBOX-001.34
  - AC-UI-NEEDS-YOU-INBOX-001.35
  - AC-UI-NEEDS-YOU-INBOX-001.36
  - AC-UI-NEEDS-YOU-INBOX-001.37
  - AC-UI-NEEDS-YOU-INBOX-001.38
  - AC-UI-NEEDS-YOU-INBOX-001.39
  - AC-UI-NEEDS-YOU-INBOX-001.40
  - AC-UI-NEEDS-YOU-INBOX-001.41
system_design:
  - ../../specs/ui/system-design/needs-you-inbox-01.md
  - ../../specs/ui/system-design/needs-you-inbox-02.md
---

# Task 01: Needs-you Inbox Delivery

## Summary

Add the workspace-scoped Needs-you Inbox: sidebar entry and route, a bounded
list-and-count read path built on `clarificationBundleQuery` /
`currentTurnAuthority`, in-place answering through the shipped
`ClarificationPanelSection`, and a per-user dismiss/snooze/restore sidecar.
Gate the feature (routes, nav entry, and boot-hydration producer) behind
`features.needsYouInbox` / `KANDEV_FEATURES_NEEDS_YOU_INBOX`.

## In scope

- Backend: `internal/clarification/inbox_handlers.go` (4 endpoints under
  `/api/v1/clarification-inbox`, flag-gated route registration), the sidecar
  schema/repository for dismiss/snooze/restore, and a boot-hydration producer
  that runs the identical bounded read as the live endpoint.
- Frontend: `needsYouInbox` store slice, sidebar nav entry, `/needs-you-inbox`
  route, row/empty/error/hidden-panel components, an additive `onOutcome`
  callback on `ClarificationPanelSection`, and i18n in all five locales.
- Desktop and mobile E2E coverage for the list -> expand -> answer -> resume
  flow (AC .30).

## Out of scope

Re-ask, the history bucket, the failed bucket, cross-workspace rows, and any
change to the current-turn predicate's meaning.

## Acceptance

All 41 acceptance criteria under `REQ-UI-NEEDS-YOU-INBOX-001`; see
[requirements](../../specs/ui/requirements/needs-you-inbox.md) for the full
text. Notable ones with dedicated regression coverage:

- AC .10: row ordering (`question_index` ascending, ties broken by
  `question_id` ascending) — `TestHttpListInbox_OrderInboxMessages_NegativeAndAbsentIndexTieBreakByQuestionID`.
  covers negative and absent index normalization.
- AC .20/.21: `200` + zero rows + `next_cursor` present renders the error
  state, never "caught up" — `resolveViewMode`.
- AC .30: list an answerable bundle, expand, answer, row removed, task resumed
  without navigation — `apps/web/e2e/tests/chat/needs-you-inbox-answer.spec.ts`
  and its mobile counterpart.
- AC .34/.40/.41: boot-hydration producer shares the live endpoint's bound
  (limit 50, same sidecar exclusion) — `TestBootNeedsYouInbox_*` /
  `TestInboxBootSummary_*`.
- AC .37: hidden-panel disclosed total reconciles with the enumeration read,
  not a stale prop.

## Verification

- `go build ./...`, `go vet ./...` clean (apps/backend).
- `go test ./internal/clarification/... ./internal/backendapp/...
  ./internal/mcp/handlers/...` — all packages `ok`.
- `make -C apps/backend lint` — 0 issues.
- `pnpm run typecheck`, `pnpm exec eslint` (touched files) — clean.
- `pnpm exec vitest run` (targeted `needs-you-inbox`, `clarification-inbox`) —
  all passing, including boundary and workspace-switch regression tests.
- `pnpm run i18n:check` / `pnpm run i18n:ratchet` — five locales complete, no
  em-dash violations, guard allowlist intact.
- `pnpm e2e:run` — desktop `needs-you-inbox-answer.spec.ts` and mobile
  `mobile-needs-you-inbox-answer.spec.ts` both pass with a full rebuild.
- `python3 scripts/lint-spec-files.py --all` — all specification files
  passed.
- Full-suite pre-existing failures (worktree/launcher/config/task-service,
  macOS TMPDIR-symlink signature) proven pre-existing against
  `git merge-base HEAD origin/main` in a scratch worktree; unrelated to this
  branch's diff.

PR: https://github.com/kdlbs/kandev/pull/3641
