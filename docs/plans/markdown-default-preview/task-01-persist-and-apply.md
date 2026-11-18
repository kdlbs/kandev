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

UI-01: Editors section, saved On state, desktop structure.

```text
Editors
Open Markdown files in Preview       [ On ]
Choose the initial mode for newly opened
Markdown files. You can switch modes at any time.

                 [ Reset ] [ Save ]
```

The row belongs to the scrolling settings content. Existing Save/Reset stays fixed and appears for a dirty draft.
UI-02: Phone SettingsRow composition:

```text
Open Markdown files in Preview
Choose the initial mode for newly opened
Markdown files. You can switch modes at any time.
                                  [ On ]
```

On phones, the shared row places the control below the wrapping label/help, in an activation area of at least 44px.
Both views use the shared SettingsRow label, help association, and discovery registration. Spacing is illustrative. Localized copy, control order, and Save/Reset behavior are required.
Maps to AC-UI-MARKDOWN-DEFAULT-PREVIEW-001.1, .2, and .6. Both targeted browser specs verify this region.

## Verification

```sh
# Run from the isolated repository root. Product checks run during implementation.
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/user/... ./internal/backendapp/... ./internal/settingscatalog/...)
(cd apps/web && pnpm exec vitest run components/settings/editors-settings-state.test.tsx components/settings/editors-settings-admin-gating.test.tsx components/settings/settings-dirty.test.ts hooks/use-ensure-user-settings.test.ts hooks/use-file-editors.open-action.test.tsx hooks/use-file-editors.build-state.test.ts components/task/file-editor-panel.workspace-sync.test.tsx components/task/file-editor-panel.image.test.tsx components/task/mobile/mobile-file-viewer-panel.test.tsx lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts lib/state/slices/settings/settings-slice.test.ts)
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

## PR remediation

Merged the current main boundary while preserving the Markdown default contract. Updated the phone source-view assertions for main's read-only editor and selection-comment support. The opening browser helper now accepts either supported source editor.

Addressed Greptile's shared-row finding with `SettingsRow`: linked label/help, `aria-describedby`, dirty state, discovery registration, and the shared phone switch target. Updated the desktop/phone browser assertions and UI previews.

Post-merge validation passed: the ten targeted frontend suites (191 tests), web typecheck, web lint, localization check, backend user/settings-catalog suites and boot projections, two desktop browser tests, 14 phone browser tests, public docs, catalog, spec lint, and diff checks. These results supersede the original 190-test frontend count. Desktop managed E2E rebuilt the runtime and assets; phone raw E2E reused that fresh build.

Fresh CI and review evidence for the pushed remediation remains externally pending. Screenshots will be refreshed after the remediation commit.

### Claude aggregate suggestions

Corrected the phone mode-reset comment to describe the saved default and caller-supplied Preview mode. The desktop fallback loader now reads the current preference from the application store when the file read completes. Its effect no longer subscribes to preference changes.

A regression failed with the previous captured preference and now passes: changing the saved default while an initial Markdown load is pending makes one read and initializes the completed buffer with the latest value. Updated the existing image-panel provider mock for the store API hook.

Validation: eleven focused frontend suites passed (196 tests), typecheck passed, web lint passed, and both desktop opening/settings browser tests passed after rebuilding Vite assets. The prior 14-phone-test result remains applicable because the subsequent phone production change is a comment only. No rendered UI changed after the refreshed shared-row screenshots.

### Frontend CI mock compatibility

Frontend shard 1/2 exposed three failures in the existing editor admin-gating tests after adopting `SettingsRow`. The test replaced the entire settings-group module and did not provide that export. Reproduced the failure locally, retained the real module through a partial mock, and added the Markdown preference fields consumed by the mocked settings state. Admin and member permissions retain their existing assertions.

The focused admin-gating, shared-row, settings-state, and dirty-state run passed (24 tests). The expanded twelve-suite work-order run passed (199 tests), and typecheck passed. No production or rendered UI changed in this CI correction. Fresh remote CI is pending at the next pushed head.

### Browser CI opening-mode compatibility

CI exposed 17 failures across eight browser shards because existing tests assumed source-first Markdown opening. Reproduced the Preview-toggle failure locally before changing the tests. Preview, math, local-link, touch-comment, and live-repository attachment checks now assert the saved default mode. The source-comment helper switches to source explicitly. The LSP active-editor scenario saves Off for its unsupported Markdown source editor and restores the original preference in `finally`.

The following desktop commands passed with retries disabled (15 tests):

```sh
pnpm --dir apps/web e2e:raw --project=chromium tests/chat/markdown-preview.spec.ts tests/chat/local-markdown-links.spec.ts --retries=0
pnpm --dir apps/web e2e:raw --project=chromium tests/chat/markdown-math.spec.ts tests/task/live-repository-attachment.spec.ts tests/lsp/lsp-file-intelligence.spec.ts tests/session/session-resume.spec.ts -g 'Markdown math|Live repository attachment|persists status-bar placement|keeps distinct model titles' --retries=0
```

Phone verification used these commands with retries disabled (eight cases):

```sh
pnpm --dir apps/web e2e:raw --project=mobile-chrome tests/chat/mobile-local-markdown-links.spec.ts tests/chat/mobile-markdown-math.spec.ts tests/task/mobile-markdown-selection-comments.spec.ts tests/task/mobile-live-repository-attachment.spec.ts tests/task/mobile-file-selection-comments.spec.ts --retries=0
pnpm --dir apps/web e2e:raw --project=mobile-chrome tests/task/mobile-live-repository-attachment.spec.ts --retries=0
```

The first phone run passed seven cases and exposed one remaining source-view assertion after reload in the live-attachment case. Updated that assertion and reran its complete scenario; it passed. These tests retain the content, sanitization, source-line, repository identity, overflow, and mode-switch assertions. The session-title and phone keyboard-dock cases that flaked in CI both passed locally without retries. No production code changed in this browser compatibility correction. Web typecheck and lint passed. Browser subprocesses used the clean environment and Nix libraries described above. Fresh remote CI remains pending at the next pushed head.

### CodeRabbit test review

Addressed all three aggregate review suggestions from review 5949024478. The live-attachment helper now returns all visible Preview matches and requires exactly one before checking content. The source-view helper scopes its mode toggle to the one visible Preview and checks its count before activation. Phone settings controls, file selection, and mode switching use `tap()`; desktop interactions retain `click()`.

Both affected browser runs passed with retries disabled (11 desktop and two phone cases):

```sh
pnpm --dir apps/web e2e:raw --project=chromium tests/chat/markdown-preview.spec.ts tests/task/live-repository-attachment.spec.ts tests/settings/markdown-default-preview.spec.ts --retries=0
pnpm --dir apps/web e2e:raw --project=mobile-chrome tests/settings/mobile-markdown-default-preview.spec.ts --retries=0
```

Web typecheck, web lint, and diff checks passed. These changes affect tests only. The PR description now includes exact browser commands, explicitly identifies the local full-suite exclusion, and links the remaining commands. It retains the template's prose summary, optional-section policy, and unchanged checklist. Fresh remote CI is pending at the next pushed head.
