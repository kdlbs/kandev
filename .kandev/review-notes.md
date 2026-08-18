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

## Action required by author

- `apps/backend/internal/workflow/handlers/handlers.go:296` — `PUT
  /api/v1/workflows/:id/coordinator-monitoring` takes `workspace_id` from the request body and
  stores it unvalidated on every `workflow_coordinator_monitoring` row; nothing checks it against
  the workflow's own workspace. No impact today because the column is never read back, but the
  Coordinator plugin is specified to read this policy workspace-scoped, so the provenance should be
  derived server-side from the workflow before that consumer lands. The obvious repository-level fix
  (`SELECT workspace_id FROM workflows` inside the replace transaction) was tried and reverted: the
  handler tests back workflows with a mock provider rather than a `workflows` row, so it cannot be
  verified at that layer without changing the fixtures, and the right owning layer is an author
  decision.
