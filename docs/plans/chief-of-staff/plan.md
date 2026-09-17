# Native chief of staff implementation

User authorized implementation on 2026-09-07. Supersedes the earlier external
assistant recommendation. Status: complete for local testing.

1. Persona configuration: completed. Reused Office roles, permissions, editable
   instructions and provider profiles; added a Chief of staff preset and explicit
   executor-profile selection in `apps/web/app/office/agents/`.
2. Persistent native conversation: completed. Transactional deterministic channel,
   runner and task IDs in `apps/backend/internal/office/repository/sqlite/conversation.go`;
   browser entry point and canonical task-event publication through Office channels.
3. Bounded context and autonomous coordination: completed. Native turns reset provider
   context and include bounded recent excerpts; comment, child-completion and blocker
   wakes use the durable Office queue. Routines retain Office ownership. Worker profile
   placement and authenticated remote Office callbacks propagate through SSH launch,
   subsequent turns and resumed-instance configuration.
4. Local validation and runnable handoff: completed. Isolated real-provider-capable
   preview at http://127.0.0.1:38439/office with separate <isolated-preview-home> state.
   Provider onboarding and real host/account selection are left to the user.

## Verification

- Backend race tests passed for Office packages, task repository, lifecycle and
  orchestrator. Focused tests cover conversation identity, UTF-8 context limits,
  fresh-session reset, dispatch deduplication, permissions, routine ownership,
  executor-profile routing and remote environment refresh.
- Final production build and Chromium E2E: 2 passed. Configured routine executes;
  the same conversation accepts repeated messages and wakes after a child result.
- Final containers E2E: 1 passed against real ephemeral sshd, including two remote
  subprocess callbacks authenticated with the Office run identity.
- Go lint: zero issues. Frontend targeted ESLint, TypeScript, i18n validation,
  public documentation validation (42 pages), and git diff --check passed.

## Boundaries and handoff

See [local setup](local-testing.md) and
[persona guide](../../public/orchestration-personas.md).
E2E providers are mock agents; the test creates the child through the API, so it
verifies the coordination pipeline, not a real model's delegation decisions.
Real Claude subscriptions and remote hosts are not configured or verified.
Instructions and explicit retrieval still use context: excerpt limits are not a
complete token budget. Authority is workspace-scoped. A native chief depends on
Kandev running; independent outage recovery and host service inventory are outside
this implementation. No external chat integration is included.
