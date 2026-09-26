---
id: "03-present-npm-recovery"
title: "Present npm runtime recovery"
status: done
wave: 3
depends_on: ["02-retry-managed-runtime-launch"]
plan: "plan.md"
spec: "../../specs/agents/requirements/runtime-updates.md"
---

# Task 03: Present npm runtime recovery

Carry the structured terminal failure to Kanban and Office, then render one
clear runtime retry path.

- **Acceptance:** Lifecycle and watcher events carry optional failure code and
  sanitized details without changing generic error compatibility.
- **Acceptance:** `last_agent_error` stores those optional fields in existing
  JSON metadata and reads both API snake case and store camel case.
- **Acceptance:** The orchestrator creates a specialized recovery message with
  `failure_kind = managed_runtime_npm_resolution`, collapsed technical details,
  and only **Retry runtime**. It uses resume mode when possible and the existing
  fresh-run mode otherwise.
- **Acceptance:** Kanban and Office explain that npm could not prepare the
  runtime and that Kandev refreshed package data and retried once. User copy
  does not mention ACP.
- **Acceptance:** All new copy uses i18n. Desktop and mobile use the same
  parsing, state, actions, and translation keys.
- **Verification:** Add failing backend and frontend tests first, then run:

  ```bash
  cd apps/backend
  go test ./internal/orchestrator -run 'Test.*ManagedRuntime.*Recovery|Test.*RecoveryActions|Test.*LastAgentError'

  cd ../../apps/web
  pnpm exec vitest run components/task/chat/messages/action-message.test.tsx components/task/simple/chat-entries.test.ts components/task/simple/components/run-error-entry.test.tsx lib/session-last-agent-error.test.ts
  pnpm run typecheck
  pnpm run i18n:check
  pnpm run i18n:ratchet
  ```

- **Files likely touched:**
  `apps/backend/internal/orchestrator/event_handlers_agent.go`,
  `apps/backend/internal/orchestrator/watcher/watcher.go`,
  `apps/backend/internal/task/models/models.go`,
  `apps/web/lib/session-last-agent-error.ts`,
  `apps/web/components/task/chat/types.ts`,
  `apps/web/components/task/chat/messages/action-message.tsx`,
  `apps/web/components/task/simple/chat-entries.ts`,
  `apps/web/components/task/simple/components/run-error-entry.tsx`, relevant
  tests, and the matching locale catalogs.
- **Dependencies:** Task 02.
- **Parallelism:** sequential because UI metadata depends on the final backend
  event contract.
- **Inputs:** The terminal failure payload from Task 02 and the existing
  provider recovery card pattern.
- **Output contract:** Report files changed, RED and GREEN commands and results,
  persisted metadata shape, action payloads for resumable and non-resumable
  sessions, i18n results, and synchronized task and plan status.
