---
id: turn-changed-files-01
title: Capture preference and initiating settings authority
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-TASKS-TURN-CHANGES-001
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-001.1
  - AC-TASKS-TURN-CHANGES-001.2
  - AC-TASKS-TURN-CHANGES-001.3
  - AC-TASKS-TURN-CHANGES-001.4
  - AC-TASKS-TURN-CHANGES-001.5
  - AC-TASKS-TURN-CHANGES-001.6
  - AC-TASKS-TURN-CHANGES-001.7
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
---

# Capture preference and initiating settings authority

## Summary

Add default-on persistence and a validated settings-policy resolver. Place the preference in General chat settings.

## Scope and owned files

- `apps/backend/internal/user/{models/models.go,dto/dto.go,service/service.go,store/sqlite.go}` and user handlers/DTO mappings.
- New focused user settings tests and replayable missing-key migration following the JSON payload convention.
- `apps/web/lib/state/slices/settings/{types.ts,settings-slice.ts}`, `lib/ssr/user-settings.ts`, `lib/types/http.ts`, settings API types.
- New `components/settings/turn-changed-files-settings.tsx`, General composition, settings discovery catalog, and preference translations.
- Define a narrow effective-policy result carrying settings user, resolution kind, revision, and boolean.
- Trace direct, queued, deferred, workflow, and synthetic launch contexts. Record any identity propagation additions needed by order 05.

## Exclusions

No capture writes, change-set tables, transcript card, or runtime release toggle.

## Implementation acceptance

1. Missing fields resolve to true at every serialization/hydration boundary; explicit false survives unrelated save, upgrade, replay, and restart.
2. The settings resolver reuses authoritative user-service rules, distinguishes settings identity from actor identity, and fails capture off on read error.
3. General Save/Discard preserves revisions and in-flight drafts; saved false immediately updates viewer state without deleting retained data.

## Verification

Planned tests use the `TurnChangedFiles` prefix. Include PostgreSQL migration/false-preservation tests, not only SQLite fixtures.

```bash
cd apps/backend
go test -trimpath ./internal/user/models ./internal/user/dto ./internal/user/service ./internal/user/store -run 'TurnChangedFiles' -count=1
```

With a real test DSN set, the same store command must execute its PostgreSQL cases.

```bash
cd apps/web
pnpm exec vitest run lib/ssr/user-settings.test.ts lib/state/slices/settings/settings-slice.test.ts components/settings/turn-changed-files-settings.test.tsx
pnpm e2e:run --project chromium tests/settings/turn-changed-files-setting.spec.ts
pnpm run i18n:zh-hant
pnpm run i18n:pseudo
pnpm run i18n:check
```

## ASCII UI preview

UI-01: General chat preference, saved on. [Combined preview](plan.md#ui-01-general-chat-preference).

```text
Chat preferences
Show changed files after each turn                 [ ON o ]
Show a file summary and diff beneath completed agent replies.
                                           [Discard] [Save]
```

On phones, stack label, 44px switch target, and description in the existing SettingsRow composition.
Save remains in the existing settings shell. Draft changes do not alter backend capture policy.
The test maps to AC-001.1-.3 and preserves false across reload.

## Dependencies and risks

No predecessor. Coordinate the resolver contract with order 05.
Reserved queue labels are not user IDs. Do not infer a human actor from the default settings account.

## Results

Implemented default-on persistence, SQLite/PostgreSQL-compatible JSON backfill, authoritative policy resolution, boot/API mappings, General Conversation settings draft/save behavior, localization, and desktop/mobile preference coverage.

Validation passed: focused backend user/model/DTO/service/store and boot-state tests; focused frontend settings/hydration tests (87 tests); `pnpm run typecheck`; `pnpm run i18n:check`; `pnpm run i18n:ratchet`; desktop Chromium and mobile-chrome preference E2E. PostgreSQL cases were discovered but skipped because `KANDEV_TEST_POSTGRES_DSN` is not configured.

The settings-discovery catalog now also exposes `show_turn_changed_files` as a writable user preference and records it in the independent coverage inventory. Focused catalog parity and inventory tests passed after the PR CI check identified the missing descriptor.
