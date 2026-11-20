---
created: 2026-11-16
status: implemented
requirements:
  - REQ-UI-MARKDOWN-DEFAULT-PREVIEW-001
system_design:
  - ../../specs/ui/system-design/markdown-default-preview.md
legacy_specs: []
---

# Markdown default Preview implementation plan

## Overview

One end-to-end work order adds the persisted setting and applies it to ordinary file opening on desktop and mobile.
[Requirements](../../specs/ui/requirements/markdown-default-preview.md) and [design](../../specs/ui/system-design/markdown-default-preview.md) own the contract.

## Workspace and checkpoint

Branch: `feat/markdown-preview-default`.
Workspace: `/home/zeval/.kandev/tasks/open-markdown-in-pre_46t3yia9/markdown-preview-default`.
Base: fetched `origin/main`, `8b46ceba098d`. No parent feature branch, dirty state, or PR3113 dependency was imported.
The user authorized implementation on 2026-11-17. The user authorized a ready PR on 2026-11-17.

## Scope and technical approach

Follow the existing user-settings JSON pipeline and Editors Save/Reset contributor.
Initialize existing Preview state at ordinary opening boundaries. Preserve explicit actions, open buffers, and tab restoration.
See the paired design for exact integration points. No engine, schema migration, new dependency, or broader settings redesign is needed.

## Work orders and dependency order

1. [Persist and apply the Markdown default](task-01-persist-and-apply.md), wave 1, done, no dependencies.

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

## Risks and exclusions

Missing JSON must differ from false. Fallback file loading can race the primary open request.
Restored tabs and active mode choices must not inherit a new default. Existing Markdown tests may require explicit Off setup for source-view cases.
Public documentation changes belong in `docs/public/developer-tools.md` during implementation.
No delegated implementation, parent branch imports, PR3113 dependency, or renderer replacement is authorized by this plan.

## Results

Implemented on 2026-11-17. The work order is done. See its [verification results](task-01-persist-and-apply.md#results) for exact commands, environment setup, and evidence.

Backend suites, 199 focused frontend tests, two desktop browser tests, and 14 phone browser tests passed after PR remediation.
An additional 15 desktop and eight phone browser regression cases passed with retries disabled after CI test compatibility corrections. Exact commands and results are in the work order.
Type checking, Go/web lint, localization, generated settings contracts, public docs, specification validation, documentation coverage, and diff checks passed.
The phone setting screenshot was inspected. The user confirmed the isolated test worked and authorized a ready PR on `feat/markdown-preview-default`.
