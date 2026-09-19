---
id: "03-e2e-and-pr-assets"
title: "Verify browser behavior and attach screenshots"
status: done
wave: 3
depends_on: ["01-transcript-threshold-and-controls", "02-pinned-prompt-overflow"]
plan: "plan.md"
spec: "../../specs/ui/requirements/last-prompt-pinning-regressions.md"
---

# Task 03: Verify browser behavior and attach screenshots

- **Acceptance:** Desktop E2E proves the bar remains closed while the prompt is partially visible and opens only after it fully leaves the transcript viewport; it can expand only when content is clipped. Mobile E2E proves the upward fallback action remains available with no bar. PR #1999 displays hosted settings and desktop pinned-bar screenshots.
- **Verification:** Run `cd apps/web && pnpm e2e:run --host --no-build -- tests/chat/last-prompt-scroll.spec.ts tests/chat/mobile-last-prompt-scroll.spec.ts --project=chromium --project=mobile-chrome`; visually inspect the tailnet server; inspect PR #1999 body after attachments.
- **Files likely touched:** `apps/web/e2e/tests/chat/last-prompt-scroll.spec.ts`, `apps/web/e2e/tests/chat/mobile-last-prompt-scroll.spec.ts`, `apps/web/.pr-assets/*` (untracked only), PR #1999 body.
- **Dependencies:** Tasks 01–02.
- **Parallelism:** sequential — assertions and screenshots must use the completed implementation.
- **Output contract:** Summary, exact files changed, test result, hosted PR asset URLs, and plan/task status update.
