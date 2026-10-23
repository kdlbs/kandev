---
id: "04-markdown-editor-e2e"
title: "Prove Markdown Editing End to End"
status: completed
wave: 4
depends_on: ["03-mobile-markdown-editing"]
plan: "plan.md"
requirements:
  - REQ-UI-MARKDOWN-FILE-EDITING-001
  - REQ-UI-MARKDOWN-FILE-EDITING-002
  - REQ-UI-MARKDOWN-FILE-EDITING-003
acceptance_criteria:
  - AC-UI-MARKDOWN-FILE-EDITING-001.1
  - AC-UI-MARKDOWN-FILE-EDITING-001.4
  - AC-UI-MARKDOWN-FILE-EDITING-001.5
  - AC-UI-MARKDOWN-FILE-EDITING-001.7
  - AC-UI-MARKDOWN-FILE-EDITING-002.1
  - AC-UI-MARKDOWN-FILE-EDITING-002.2
  - AC-UI-MARKDOWN-FILE-EDITING-002.3
  - AC-UI-MARKDOWN-FILE-EDITING-002.6
  - AC-UI-MARKDOWN-FILE-EDITING-002.8
  - AC-UI-MARKDOWN-FILE-EDITING-003.2
  - AC-UI-MARKDOWN-FILE-EDITING-003.3
  - AC-UI-MARKDOWN-FILE-EDITING-003.4
  - AC-UI-MARKDOWN-FILE-EDITING-003.5
  - AC-UI-MARKDOWN-FILE-EDITING-003.6
system_design:
  - ../../specs/ui/system-design/markdown-file-editing.md
---

# Task 04: Prove Markdown Editing End to End

## Summary

Add production-build Playwright coverage for the integrated desktop and mobile
Markdown workflows. Keep the existing preview security and Review regressions
in the final verification set.

## In scope

- Desktop Files flow for modes, edit, save, reload, comments, and restored mode.
- Desktop MDX and hybrid-failure Source fallback cases.
- Mobile Files flow for edit, save, keyboard, controls, and overflow.
- Managed fixture setup and cleanup through public application behavior.
- Final targeted and repository-defined verification commands.

## Out of scope

- Screenshot-only coverage.
- Direct state injection or unmanaged development-server tests.
- Manual browser compatibility certification.

## Acceptance

- Desktop Playwright proves the complete Markdown lifecycle with real file and
  editor integrations.
- `mobile-chrome` proves the same user value plus phone geometry and keyboard
  behavior.
- Existing security, preview, comments, tables, and Review tests remain green
  with the new dependency and modes.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
pnpm --filter @kandev/web test -- --run components/editors/markdown components/task
pnpm --dir web run i18n:check
pnpm --dir web e2e:run tests/task/markdown-file-editing.spec.ts
pnpm --dir web e2e:run -- --project=mobile-chrome tests/task/mobile-markdown-file-editing.spec.ts
cd ..
make fmt
make typecheck test lint
```

## Files likely touched

- `apps/web/e2e/tests/task/markdown-file-editing.spec.ts`
- `apps/web/e2e/tests/task/mobile-markdown-file-editing.spec.ts`
- `apps/web/e2e/fixtures/`
- `apps/web/e2e/helpers/`

## Dependencies

Task 03 completes the responsive implementation under test.

## Risks

- Rich-editor focus and animation can make selectors timing-sensitive.
- File-changing fixtures can leak state unless cleanup restores content.
- Mobile geometry can pass in one browser while remaining unusable on WebKit.

## Parallelism

`sequential`

## Inputs

- All requirement and system-design sections.
- `apps/web/e2e/README.md` and existing Markdown preview E2E tests.
- `.agents/skills/e2e/SKILL.md` and `.agents/skills/mobile-parity/SKILL.md`.

## Results

- Added managed production-build Chromium coverage for the desktop Markdown
  lifecycle and mobile-chrome coverage for mobile editing, saving, reloading,
  controls, and contained content.
- Updated existing desktop and mobile Markdown regression selectors for the
  explicit mode controls and the mobile Back action.
- Verification passed: new desktop E2E 2/2, new mobile E2E 1/1, existing
  desktop Markdown regression 8/8, existing mobile file-viewer regression 9/9.
- The focused web suite passed 3,264 tests with 4 skips. The repository test
  target passed backend, 14,019 web tests with 4 skips, CLI 5/5, and all script
  suites. The repository typecheck, formatter, lint, and i18n checks passed.
