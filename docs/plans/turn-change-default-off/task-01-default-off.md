---
id: "01-default-off"
title: "Repair turn-change defaults, inventory, and empty cards"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TURN-CHANGES-001
  - REQ-TASKS-TURN-CHANGES-005
  - REQ-TASKS-TURN-CHANGES-007
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-001.1
  - AC-TASKS-TURN-CHANGES-001.2
  - AC-TASKS-TURN-CHANGES-005.7
  - AC-TASKS-TURN-CHANGES-005.9
  - AC-TASKS-TURN-CHANGES-007.1
  - AC-TASKS-TURN-CHANGES-007.5
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
---

# Task 01: Small turn-change fixes

## Scope and acceptance

New default/member accounts and unstored backend/frontend settings default false.
Existing saved true/false values and omitted partial updates retain their meaning.
No reset, new migration, or settings control is added. Full-index reads above
20,000 entries stay valid within existing byte/time bounds; candidate/changed-file
caps remain. Hide settled zero-entry cards via a transcript-only predicate, keeping
positive-file cards and historical selection unchanged. Preserve bottom-follow
through native restoration during delayed card growth; manual scrolling and
explicit navigation retain their existing ownership.
Update existing default/hydration tests and public help.

## ASCII UI preview

UI-01, [combined preview](plan.md#ascii-ui-preview), desktop and phone:

```text
Show changed files after each turn    [OFF]
```

UI-02, [combined preview](plan.md#ascii-ui-preview), desktop/phone chat:

```text
Agent reply
Next conversation item
```

## Files

`apps/backend/internal/user/store/{sqlite.go,sidebar_presentation_migration.go}`
(default seed constant only in the latter), adjacent default tests, frontend
`lib/ssr/user-settings.ts` and its preference tests, `turn_checkpoint_capture.go`
and the new large-inventory regression, `projection.ts`/tests and all three
`message-list-native.tsx` filters, plus existing public/spec docs. Browser setting and Git specs plus their shared
helpers cover default-off opt-in and empty-card rendering on desktop and phone.
CI remediation also touches `message-list-native-scroll.ts` and its unit test,
and the interrupted-prompt recovery test fixture SQLite connection timeout.

## Verification

From repository root, with Node/pnpm on PATH:

```bash
(cd apps/backend && go test -trimpath ./internal/agentctl/server/process -run 'TurnCheckpoint|TurnChange' -count=1)
(cd apps/backend && go test -trimpath ./internal/user/store ./internal/user/service ./internal/user/dto -count=1)
(cd apps/web && pnpm exec vitest run lib/turn-changes/projection.test.ts lib/turn-changes/history-scope.test.ts components/task/chat/message-list-native.test.tsx components/task/chat/turn-changed-files-card.test.tsx lib/ssr/turn-changed-files-user-settings.test.ts components/settings/turn-changed-files-settings.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
bash scripts/lint-go-changed apps/backend/internal/user/store/sqlite.go apps/backend/internal/user/store/sidebar_presentation_migration.go apps/backend/internal/agentctl/server/process/turn_checkpoint_capture.go apps/backend/internal/agentctl/server/process/turn_checkpoint_large_inventory_test.go
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/settings/turn-changed-files-setting.spec.ts tests/git/turn-changed-files.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/settings/mobile-turn-changed-files-setting.spec.ts tests/git/mobile-turn-changed-files.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/git/turn-changed-files-scroll.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/git/mobile-turn-changed-files-scroll.spec.ts)
(cd apps/web && pnpm e2e:run --docker --shards 1 --project chromium tests/chat/message-pagination.spec.ts tests/chat/auto-scroll-toggle.spec.ts tests/git/turn-changed-files-scroll.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --docker --no-build --shards 1 --project mobile-chrome tests/chat/mobile-message-pagination.spec.ts tests/chat/mobile-auto-scroll-toggle.spec.ts tests/git/mobile-turn-changed-files-scroll.spec.ts -- --retries=0)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Results

Red confirmed for new-account/missing-value defaults, empty-card eligibility,
and real large-index capture (unsafe_git_state/output limit exceeded).
Green: backend user store/service/DTO packages passed; checkpoint/change tests
passed, including real 30,001-file capture/compare/export with zero and one-file
changes and unchanged real index/HEAD. Six frontend suites passed (137 tests).
Typecheck passed with NODE_OPTIONS=--max-old-space-size=8192 after the default
4 GiB heap was insufficient. Changed Go lint: zero issues. Frontend eslint passed;
changed files were formatted with Prettier. Public-doc validators, spec/catalog
checks, documentation coverage preflight and git diff whitespace passed.
PostgreSQL-specific cases were not exercised without a DSN. Browser E2E: six desktop Chromium cases and five mobile-chrome cases passed,
including saved opt-in from off, existing historical diff/file-list retry flows,
and hiding an unavailable zero-entry card after reload. Desktop/mobile PR assets
were captured in the isolated runner, inspected, and compressed. Desktop built
fresh artifacts; mobile reused those unchanged artifacts with --no-build. Live data was not changed. No reset, new migration,
flag, or deployment. Existing saved preferences and upgrade backfill remain unchanged.

## Dependencies and risks

Sequential. Existing saved-on users intentionally remain enabled, including old
upgrade backfills. Browser fixtures now explicitly opt in for capture-dependent
tests and restore the original preference afterward.

Review remediation: full-index flag and staged-entry scans now check the capture
context between entries. The deterministic cancellation regression cancels after
the first entry and verifies both scans stop; normal-context positive controls
remain successful. Focused checkpoint/change tests passed again. No rendered UI
changed, so the verified desktop/mobile captures remain applicable.

Capture-dependent desktop/mobile scroll fixtures now explicitly opt in and restore
the saved preference. Their additional managed E2E runs passed (one case per
viewport), bringing browser coverage to eleven passing cases. No product code
or rendered composition changed in this follow-up.

Windows CI remediation: hashing the full 30,001-file checkout exceeded the
unchanged capture deadline on the hosted Windows runner. Both full-index scans
now have portable 30,002-record regressions that verify processing of the final
record; Windows uses those plus the existing small-repository capture tests.
The full-checkout capture/compare/export integration remains on Linux and is
skipped on Windows. A sparse fixture was investigated and discarded because it
also timed out. The focused checkpoint/change suite passed locally again (7.3s).
No production timeout, capture budget, or UI behavior changed in this follow-up.
Remote CI and merge remain pending.

Further CI remediation: the mobile delayed-card reload case reproduced against
GitHub's exact merge commit. Native restoration moved the viewport upward by
88px while the delayed card grew by 88px; the follow predicate incorrectly
released bottom ownership without reader input. A regression using the captured
geometry failed before the bounded absolute-delta correction and passed after it.
Existing explicit-position and reader-input guards remain covered. Four focused
frontend suites passed (151 tests), and the exact mobile browser regression
passed three repetitions after rebuilding the CI merge checkout. Eleven desktop
browser cases passed without retries, including manual reading, disabled/enabled
auto-scroll, cancellation, Quick Chat recovery, and delayed-card reload.

The interrupted-prompt test fixture now uses the existing 5-second SQLite busy
wait convention. A controlled concurrent writer reproduced the CI lock error
before the change and passed afterward. This changes only test setup. Frontend
lint, formatting, and typechecking passed. No public/spec contract changed:
AC-TASKS-TURN-CHANGES-005.9 already requires delayed cards to follow while preserving
manual reading. Temporary geometry tracing and retry overrides were removed.
Fresh remote checks and merge remain pending after the follow-up push.

Nine further mobile browser cases passed without retries, covering manual reading,
auto-scroll transitions, cancellation, and Quick Chat recovery. These reused the
unchanged rebuilt artifacts from the passing mobile reload repetitions. No new
rendered composition was introduced; existing settings and empty-card PR captures
remain applicable.

Pagination follow-up: the native-restoration correction exposed a stale follow
flag when an older-page request began without a wheel gesture. Existing desktop
pagination anchor cases reproduced the CI failures (two failures, no retries).
Older-page preparation now cancels motion/captures the prepend baseline and then
resynchronizes bottom ownership from the actual viewport before fetching data.
The same two desktop cases passed afterward without changing their assertions.
Full browser verification passed 16 desktop and 16 mobile cases without retries,
covering all pagination cases, deep/short/tool-only history, manual reading,
auto-scroll transitions, and delayed changed-file completion/reload together.
Four frontend suites (151 tests), lint, formatting, and typecheck passed again.
This preserves AC-TASKS-TURN-CHANGES-005.9; no new public behavior or composition is
introduced. Fresh remote checks remain pending after the remediation push.
