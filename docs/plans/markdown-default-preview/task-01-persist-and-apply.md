---
id: task-01
title: Persist and apply the Markdown preview default
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-UI-MARKDOWN-DEFAULT-PREVIEW-001
acceptance_criteria:
  - AC-UI-MARKDOWN-DEFAULT-PREVIEW-001.1
  - AC-UI-MARKDOWN-DEFAULT-PREVIEW-001.2
  - AC-UI-MARKDOWN-DEFAULT-PREVIEW-001.3
  - AC-UI-MARKDOWN-DEFAULT-PREVIEW-001.4
  - AC-UI-MARKDOWN-DEFAULT-PREVIEW-001.5
  - AC-UI-MARKDOWN-DEFAULT-PREVIEW-001.6
system_design:
  - ../../specs/ui/system-design/markdown-default-preview.md
---

# Persist and apply the Markdown preview default

## Summary and scope

Deliver the complete account preference, settings UI, localization, and desktop/mobile initial-mode behavior.
Follow the [plan](plan.md) and paired [design](../../specs/ui/system-design/markdown-default-preview.md).
Use TDD for changed logic. First prove missing-value and explicit-false behavior with failing tests.

## Likely files

- Backend user settings model, DTO, service, request adapters, JSON store, and their tests under `apps/backend/internal/user/`.
- Backend boot projection and tests under `internal/backendapp/`, plus `internal/settingscatalog/defaults.go` and generated contracts.
- Frontend settings types/defaults, SSR mapping, WS projection, discovery catalog, Editors draft/save/reset/dirty helpers, and locale catalogs.
- `apps/web/hooks/file-editor-open-actions.ts`, `use-file-editors.ts` restore tests, `components/task/file-editor-panel.tsx`, and mobile viewer state/tests.
- New `apps/web/e2e/tests/settings/markdown-default-preview.spec.ts` and `mobile-markdown-default-preview.spec.ts`. Existing mobile file-viewer tests and `docs/public/developer-tools.md`.

## Implementation acceptance

1. Account persistence distinguishes missing from false and uses existing Save/Reset, request, boot, and broadcast contracts.
2. Ordinary desktop and phone opens honor the saved value while explicit Preview, restored tabs, and active buffers retain their modes.
3. Localized, keyboard-accessible, touch-accessible settings and real browser coverage satisfy all linked criteria.

## Exclusions and dependencies

No dependencies. Do not add an engine, migrate storage, import parent code, or depend on PR3113.
Read scoped backend/web guidance and the TDD, mobile-parity, better-ui, E2E, and docs-maintainer skills before implementation.
Use current shipped switch and settings-row primitives. Do not redesign the settings shell.

## ASCII UI preview

UI-01: Editors section, saved On state, shared desktop and phone structure.

```text
Editors
Open Markdown files in Preview       [ On ]
Choose the initial mode for newly opened
Markdown files. You can switch modes at any time.

                 [ Reset ] [ Save ]
```

The row belongs to the scrolling settings content. Existing Save/Reset stays fixed and appears for a dirty draft.
On phones, copy wraps beside the switch and its activation area measures at least 44px.
The same hierarchy serves both viewports. Spacing is illustrative. Localized copy, control order, and Save/Reset behavior are required.
Maps to AC-UI-MARKDOWN-DEFAULT-PREVIEW-001.1, .2, and .6. Both targeted browser specs verify this region.

## Verification

```sh
# Run from the isolated repository root. Product checks run during implementation.
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/user/... ./internal/backendapp/... ./internal/settingscatalog/...)
(cd apps/web && pnpm exec vitest run components/settings/editors-settings-state.test.tsx components/settings/settings-dirty.test.ts hooks/use-ensure-user-settings.test.ts hooks/use-file-editors.open-action.test.tsx hooks/use-file-editors.build-state.test.ts components/task/file-editor-panel.workspace-sync.test.tsx components/task/mobile/mobile-file-viewer-panel.test.tsx lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts lib/state/slices/settings/settings-slice.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check && pnpm run i18n:ratchet)
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/settings/markdown-default-preview.spec.ts
pnpm --dir apps/web e2e:raw --project=mobile-chrome tests/settings/mobile-markdown-default-preview.spec.ts tests/task/mobile-file-viewer.spec.ts
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run additional changed suites if implementation touches tests outside this command list. Record exact commands and results here.
Use the repository's settings-contract generator and changed-Go lint helper if those files change.

## Risks

Cover fallback-loader races, binary Markdown names, legacy restored tabs, and explicit false across unrelated saves.
Preserve user edits made while Save is in flight. Test mobile file identity changes and same-file preference updates.

## Results

Completed on 2026-11-17 in the isolated workspace.

- Backend: `go test ./internal/user/... ./internal/backendapp/... ./internal/settingscatalog/...` passed. The subsequently added boot projection test passed with `go test ./internal/backendapp -run TestMarkdownPreviewBootProjection -count=1`. The storage test passed after moving it to `internal/user/store/markdown_preview_test.go`.
- Frontend: the ten targeted suites passed (190 tests). The final opening, mobile, dirty-state, and restore subset passed (37 tests). `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet` passed.
- Desktop: the managed Chromium command above passed both tests (9.3 seconds).
- Phone: the raw mobile command above reused the fresh managed build and passed 14 tests (56.7 seconds), including both new settings/opening cases and the existing mobile file viewer suite.
- Visual: inspected the phone screenshot from the settings test. The label/help wrap correctly, the switch remains visible, and the measured activation row is at least 44px. Page overflow assertion passed.
- Settings contracts: `go run ./cmd/settings-catalog -check` passed from `apps/backend`.
- Go lint: installed pinned `golangci-lint` 2.9.0 into `/tmp/markdown-preview-tools`, then ran `scripts/lint-go-changed` over all changed Go files plus the new storage test. Passed with zero issues.
- Documentation: both public-doc validators, catalog validation, specification lint, PR-documentation coverage preflight, and `git diff --check` passed.

Environment notes: Browser subprocesses removed inherited `KANDEV_*` variables to avoid the host service's tailnet configuration. Chromium used `LD_LIBRARY_PATH` assembled from existing Nix store libraries. Initial readiness/library failures occurred before feature assertions. The test helper now tolerates restoration of an active file tab rather than requiring chat on its second navigation.

The existing renderer is unchanged. No parent feature changes, PR3113 changes, or new dependencies were imported. The user confirmed manual testing passed and authorized commit, push, and a ready PR. The isolated user-test instance on port 48731 was shut down. The main instance on port 9998 retained PID 1165. Desktop and phone setting screenshots were captured in a fresh disposable run (two tests passed).
