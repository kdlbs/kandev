---
id: "02-shared-repository-chips"
title: "Shared controlled repository chips"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/quick-chat-repository-context.md"
---

# Task 02: Shared controlled repository chips

**Acceptance:** task creation and quick chat can use one controlled workspace repo/branch chip
surface; task-only source modes remain unchanged; quick chat can exclude duplicate repos.

**Verification:** run the focused repository-chip Vitest files from `apps/`.

**Files likely touched:** `apps/web/components/task-create-dialog-repo-chips.tsx`, new shared
component and tests.

**Dependencies:** None.

**Output contract:** summarize extraction, tests run, files changed, risks, and status.
