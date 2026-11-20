---
id: "01-default-on"
title: "Default changed files to on"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TURN-CHANGES-001
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-001.1
  - AC-TASKS-TURN-CHANGES-001.2
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
---

# Task 01. Default changed files to on

## Summary

Use the on value for new accounts and accounts without a saved preference.
Apply the same default before browser settings hydrate. Retain saved choices.

## In scope

- Update the default and new-account seed in the user settings store.
- Update the frontend settings default and existing desktop and phone coverage.
- Update the preference instructions in `docs/public/tasks-and-workflows.md`.

## Out of scope

- Stored-value rewrites, past change sets, and capture policy changes beyond
  the default.

## Acceptance

1. A fresh single-user instance and new account start with the preference on.
2. A missing or null field reads as on. An explicit value survives partial
   settings updates and backend restart.
3. The switch starts on for a fresh account on desktop and phone. An off choice
   saved from the switch remains off after reload.

## ASCII UI preview

### UI-01 Conversation preference for a fresh account

Excerpt from the [full preview](plan.md#ui-01-conversation-preference-for-a-fresh-account), covering `.1` and `.2`.

```text
Show changed files after each turn                  [ON]
```

The existing Conversation settings row has the same composition on desktop
and phone.

## Verification

Update the existing account seed, decode, hydration, and browser tests first.
The absent-value expectations must fail against the off default. Keep the
explicit false cases passing and check that Reset restores the current saved
value before the phone test saves off.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -trimpath ./internal/user/store ./internal/user/service ./internal/user/dto -count=1)
bash scripts/lint-go-changed apps/backend/internal/user/store/sqlite.go apps/backend/internal/user/store/sidebar_presentation_migration.go apps/backend/internal/user/store/sidebar_presentation_settings_test.go apps/backend/internal/user/store/turn_changed_files_preference_test.go apps/backend/internal/user/store/startup_page_test.go
(cd apps/web && pnpm exec vitest run lib/ssr/turn-changed-files-user-settings.test.ts components/settings/turn-changed-files-settings.test.tsx)
(cd apps/web && pnpm run typecheck)
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/settings/turn-changed-files-setting.spec.ts
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project mobile-chrome tests/settings/mobile-turn-changed-files-setting.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/user/store/sqlite.go`
- `apps/backend/internal/user/store/sidebar_presentation_migration.go`
- `apps/backend/internal/user/store/{sidebar_presentation_settings,turn_changed_files_preference,startup_page}_test.go`
- `apps/web/lib/ssr/user-settings.ts`
- `apps/web/lib/ssr/turn-changed-files-user-settings.test.ts`
- `apps/web/e2e/tests/settings/turn-changed-files-setting.spec.ts`
- `apps/web/e2e/tests/settings/mobile-turn-changed-files-setting.spec.ts`
- `docs/public/tasks-and-workflows.md`

## Dependencies

None.

## Risks

- Both account seeds and missing-field paths must agree. A changed frontend
  fallback alone would show on before hydration and then switch off.

## Parallelism

`sequential`

## Inputs

- [Persistent capture preference](../../specs/tasks/requirements/turn-changed-files.md#req-tasks-turn-changes-001-persistent-capture-preference).
- [Settings and authority](../../specs/tasks/system-design/turn-changed-files.md#settings-and-authority).
- [Prior default change](../turn-change-default-off/task-01-default-off.md).

## Results

- The updated Go, frontend, and desktop and phone browser tests failed on the
  old default. The existing cases for explicit true and false values passed.
- The user store, service, and DTO tests passed. The store suite exercised
  SQLite and a disposable PostgreSQL 16 database. Changed Go lint passed.
- Both focused Vitest suites passed all five tests. Typecheck, formatting, and
  changed-file Oxlint passed.
- The desktop and phone Playwright runs passed one case each. Both verified the
  default and the saved off choice after reload. The phone case also verified
  Reset. The phone run reused the desktop build.
- The review follow-up waits for user settings hydration before rendering turn
  change cards. The regression tests passed as part of the 178-test focused
  frontend run, and the desktop and phone setting specs each passed one case.
- Public docs validation passed. `python3 scripts/list-docs.py validate`,
  `python3 scripts/lint-spec-files.py --all`, the local PR documentation
  coverage check, and `git diff --check` passed.
