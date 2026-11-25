## Backend package structure

- `cmd/kandev`, `cmd/agentctl`, and `cmd/mock-agent` contain binary entry points.
- `internal/agent/runtime` owns agent execution lifecycle and the agentctl client.
- `internal/agentctl/server` owns agentctl APIs, adapters, processes, and sessions.
- `internal/orchestrator`, `internal/task`, `internal/runs`, and `internal/workflow` coordinate execution.
- `internal/office` owns autonomous agent management.
- `internal/integrations` contains shared integration shapes; Jira and Linear own their service-specific code.
- `internal/gateway` owns WebSocket delivery; `internal/persistence` owns database initialization.

Use the directory tree for the current package inventory.
For canvas changes, preserve the authority contract: the trusted task adapter binds owner, session,
task, and policy version. The first valid static release may consume that authority for exact
workspace-ceiling grants in the activation transaction. Existing drafts, imports, later permission
increases, and revoked grants require human review. Source metadata and manifests never grant authority.
Canvas scope metrics use fixed transition and result labels. Never put task data in metric labels.
