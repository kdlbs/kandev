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

**Verification:** from `apps/web`, run
`pnpm e2e:run --host --no-build --project chromium -- tests/chat/quick-chat.spec.ts`
(6 passed) and
`pnpm e2e:run --host --no-build --project mobile-chrome -- tests/chat/mobile-quick-chat-repository.spec.ts`
(1 passed). From the repository root, run `make fmt`, `make typecheck`, `make test`, and
`make lint`; all completed successfully.

**Files likely touched:** quick-chat desktop/mobile E2E specs.

**Dependencies:** Task 03.

**Output contract:** report scenarios, commands, results, artifacts, risks, and status.
