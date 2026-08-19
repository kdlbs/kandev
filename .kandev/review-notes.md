# Review notes

## Fixed during review

- `apps/backend/internal/plugins/host.go:315` — `pluginHost.AgentConversations()` returned a nil
  interface when the plugin had not declared `agent_conversation`, but `grpcHostServer`'s
  `AgentConversationHost` type assertion still succeeded, so `EnsureAgentConversation` /
  `DispatchAgentConversation` / `DeleteAgentConversation` called a method on a nil interface and
  panicked the host instead of denying the call. The capability gate is now enforced per method and
  returns `PermissionDenied` (`Unavailable` when the service is not wired), with the
  `host_agent_conversations_test.go` coverage the plan called for. (commit 435b22fd5)
- `apps/backend/internal/task/service/agent_conversations.go:513` — the managed-conversation lookup
  read only the first page (100 rows) of a workspace's ephemeral tasks, ordered `updated_at DESC`.
  Once a workspace held more than a page of quick chats, an existing coordinator conversation fell
  off that page: `Ensure` created a duplicate hidden conversation, `Dispatch` returned `NotFound`
  after the scheduler had already claimed the occurrence key, and `Delete` orphaned rows on
  uninstall. It now pages through the whole ephemeral set. (commit d660a46a1)
- `apps/web/components/settings/plugins/plugin-config-form.tsx:316` — the new `agent_profile` field
  read `agentProfiles` from the store, but only `useSettingsData` populates that slice and the
  plugin settings route never calls it, so a direct visit or reload of Settings > Plugins > plugin
  rendered an empty profile picker. (commit de20c9d43)
- `apps/web/lib/plugins/host-api.ts:160` — `host.ui.WorkspaceAgentChat` was imported eagerly,
  pulling the entire task chat graph into every plugin boot. Now lazy plus Suspense, matching the
  change-request detail view in the same file. (commit 260e57a2c)
- `apps/backend/.golangci.yml:67` — the pluginsdk exemption was a `paths:` entry, which excludes
  `host.go` and `data_types.go` from every linter rather than only the 800-line file-length rule.
  Narrowed to revive's `file-length-limit`; both files lint clean under everything else.
  (commit 1d5a57bdf)

## Fixed during QA

- `apps/backend/pkg/pluginsdk/host.go:1017` — `grpcHostServer` only type-asserted
  `AgentConversationHost` and then called straight through `AgentConversations()`. That interface is
  public and returning nil is how an implementation says "this caller may not use agent
  conversations", so the three RPCs segfaulted the gRPC server. A bufconn test reproduces the
  SIGSEGV without the fix. (commit ac48d8174)
- `apps/backend/internal/workflow/service/coordinator_monitoring.go:27` — `workspace_id` arrived in
  the PUT body and was stored verbatim; posting `TOTALLY-BOGUS-WORKSPACE` against a live instance
  persisted exactly that on a workflow belonging to a different workspace. Now resolved from the
  workflow itself. (commit 37b29d9f4)
- Added the coverage both Review-phase UI fixes shipped without: the `agent_profile` field loading
  its own profiles, and `host.ui.WorkspaceAgentChat` staying off plugin boot while still resolving
  to the real chat surface. (commit 504c4d961)

## Follow-up tasks created (out of scope for this PR)

- Make internal/github tests hermetic against GH_TOKEN (task
  `725d47ae-58f9-4903-88b4-2d9aa8dbe733`) — `apps/backend/internal/github/factory_test.go:38` and
  `service_token_test.go:229` fail in any shell exporting `GH_TOKEN` and pass under `env -i`.
  Introduced in `44811da70` by Carlos Florencio <carlosmflorencio@gmail.com>. `internal/github` has
  zero files in this branch's diff.

## Action required by author

- **Sign off on two permanent proto fields.** `828f949b1` adds
  `WorkflowStep.coordinator_monitored = 6` / `coordinator_prompt = 7` and serves them from the
  existing `api_read: ["workflows"]` capability rather than a new one, so **every** plugin holding
  that capability can read operator-authored coordinator instructions for every step. Note the
  deliberate asymmetry this creates: `wfmodels.WorkflowStep.Prompt` (the step's own agent
  instructions) is still not exposed to plugins at all. The choice is documented in the commit and
  is consistent with the spec's "read by the plugin through a capability-scoped API", but proto
  field numbers are effectively permanent and the plan's own risk list asks for ADR/spec review
  before code generation, so this should be an explicit decision rather than an inherited one.
- **Correct the claim in `94e7efc36`'s message before reusing it in the PR description.** It states
  it "closes a same-process race in Ensure's check-then-create sequence", but the commit does not
  touch `Ensure` or `lockEnsureKey`; that serialization already existed on the branch. The rest of
  the message is accurate.
