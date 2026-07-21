---
id: "03-quick-chat-setup"
title: "Responsive quick-chat setup"
status: done
wave: 2
depends_on: ["01-backend-contract", "02-shared-repository-chips"]
plan: "plan.md"
spec: "../../specs/tasks/quick-chat-repository-context.md"
---

# Task 03: Responsive quick-chat setup

**Acceptance:** users select an agent and optional repository branches before starting; first-use
and field helper copy render correctly; failed/superseded starts preserve current cleanup rules.

**Verification:** run focused quick-chat Vitest tests and web typecheck from `apps/web`.

**Files likely touched:** quick-chat modal/hook/setup files and workspace API types.

**Dependencies:** Tasks 01 and 02.

**Output contract:** summarize UI/state changes, tests run, files changed, risks, and status.
