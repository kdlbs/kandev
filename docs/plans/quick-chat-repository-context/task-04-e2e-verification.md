---
id: "04-e2e-verification"
title: "Quick-chat repository E2E verification"
status: done
wave: 3
depends_on: ["03-quick-chat-setup"]
plan: "plan.md"
spec: "../../specs/tasks/quick-chat-repository-context.md"
---

# Task 04: Quick-chat repository E2E verification

**Acceptance:** desktop and mobile flows start a repo-backed quick chat; source checkout remains
unchanged; controls fit narrow viewports.

**Verification:** run focused managed Playwright specs from `apps/web` and the full repository
format/typecheck/test/lint pipeline.

**Files likely touched:** quick-chat desktop/mobile E2E specs.

**Dependencies:** Task 03.

**Output contract:** report scenarios, commands, results, artifacts, risks, and status.
