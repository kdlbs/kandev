---
id: "14-verify-status-customization"
title: "Verify status customization"
status: completed
wave: 6
depends_on: ["13-refine-language-disclosure"]
plan: "plan.md"
spec: "../../specs/platform/requirements/lsp-file-intelligence.md"
---

# Task 14: Verify Status Customization

## Acceptance

- Production-build desktop E2E hides and restores a language, verifies reload persistence and
  aggregate filtering, and proves the current editor shortcut still reaches the hidden language.
- Phone/tablet E2E verifies inline collapsibles, shared visibility, 44 px controls, one drawer and
  scroll owner, safe-area/viewport containment, and no horizontal overflow.
- Public developer-tools and feature-status documentation describe collapsibles and the
  presentation-only visibility setting.

## TDD sequence

1. Add or update production E2E expectations before implementation is complete and capture a
   focused failing run.
2. Make desktop, tablet, and phone paths green with lifecycle/process evidence unchanged.
3. Run focused frontend/backend suites, typecheck, lint, i18n, production build, and docs checks.
4. Record commands/results in all three follow-up tasks and return the plan to completed.

## Verification

```bash
cd apps/web && pnpm build
cd apps/web && pnpm e2e:run --no-build --project chromium -- e2e/tests/lsp/task-lsp-lifecycle.spec.ts
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome -- e2e/tests/lsp/mobile-lsp-file-intelligence.spec.ts
cd apps && pnpm run i18n:check && pnpm run i18n:ratchet
```

## Output contract

Record exact production E2E scenarios, geometry evidence, docs checks, and final PR verification.

## Results

Completed 2026-08-06.

- Production RED: the new desktop visibility scenario timed out looking for a language
  collapsible. Its first GREEN attempt then exposed two integration gaps: the preference was absent
  from boot state, and filtering the last relevant language also removed the topbar fallback. Both
  received focused regression tests before fixes.
- Production GREEN: the 10 task-lifecycle scenarios passed, including persisted hide/reload,
  unchanged Kotlin generation/import count, current-editor access, restart/stop, task cleanup,
  active-panel independence, and status-bar-disabled fallback. All 13 existing desktop LSP
  intelligence scenarios also passed.
- Mobile Chrome passed 3/3 phone/tablet scenarios. They verify collapsed rows, 44 px triggers and
  controls, shared persisted visibility, one contained Status drawer, viewport containment, and no
  mobile-viewer auto-start. The container/SSH command rebuilt production artifacts; this host had
  no reachable Docker daemon, so two daemon-backed cases skipped and two fixture-safe cases passed.
- `pnpm build`, web typecheck, full ESLint, Prettier check, i18n check/ratchet, and seven focused
  frontend files passed. Backend user/backendapp tests passed and changed-file golangci-lint
  reported zero issues.
- Updated Developer Tools and Feature Status for independent collapsibles, presentation-only
  visibility, all-hidden fallback, and mobile parity. Public-doc validator tests passed 58/58 and
  validated all 41 published pages.
