---
spec: docs/specs/integrations/requirements/github-authentication.md
created: 2026-07-27
status: complete
---

# Implementation Plan: Task Git Credential Policy And Identity

## Overview

Fix the confirmed standalone managed-helper wiring failure while separating workspace GitHub
automation from task Git credential routing. Persist the policy first, then make initial launch and
resume resolve the same effective contract and record a truthful session snapshot. Finish with
self-documenting settings, responsive Changes-panel identity disclosure, focused E2E coverage, and
public documentation.

Confirmed root causes:

1. The standalone launcher resolves and starts the `agentctl` control binary by absolute path, but
   the per-task Git config stores `!agentctl git-credential`. The control process only
   creates/prepends its `gh` shim directory when broker environment exists at control-process
   startup; standalone broker data arrives later in the per-instance environment. Therefore child
   Git cannot find `agentctl`.
2. Task environments currently merge `GIT_CONFIG_COUNT` and indexed keys as ordinary map entries.
   A Kandev block with count 2 overwrites a host/executor block's count and first two entries, so
   unrelated tools such as hooks and Git notes disappear. The same defect can hide Docker's
   `safe.directory` and URL rewrites. A focused reproduction against `CollectAgentEnv` produced
   count 2 where the correct composed result is count 4.
3. Resume currently does not rebuild the broker contract at all.

---

## Backend

### Ordered indexed Git configuration

- Add a shared backend utility for parsing, validating, composing, and contiguously serializing the
  `GIT_CONFIG_COUNT` / `GIT_CONFIG_KEY_<n>` / `GIT_CONFIG_VALUE_<n>` protocol.
- Composition preserves source order, appends higher-precedence entries, and removes only the
  longest exact suffix/prefix overlap when the same block has already passed through a control
  process. It never deduplicates arbitrary repeated Git keys because repeated keys can be
  meaningful.
- Use structured composition in agent-profile/request merges, route overrides, container
  environment construction, remote forwarding, and agentctl's parent-plus-instance collection.
  Ordinary scalar environment variables retain their documented precedence.
- Keep `internal/repoclone` isolated: managed clone subprocesses intentionally strip ambient Git
  configuration and build a closed credential environment.
- Add a real Git subprocess regression that starts with locstat-shaped `core.hooksPath` and
  `notes.augment.mergeStrategy` entries, adds Kandev's helper entries, makes a commit, and verifies
  the hook ran. Add focused standalone, Docker, SSH/remote, overlap, precedence, malformed-block,
  and bounded-count tests.

### Workspace policy persistence and description

- In `apps/backend/internal/github/models.go`, add
  `TaskGitCredentialsModeManaged`, `TaskGitCredentialsModeExecutor`, and
  `WorkspaceSettings.TaskGitCredentialsMode` /
  `UpdateWorkspaceSettingsRequest.TaskGitCredentialsMode`.
- In `apps/backend/internal/github/store.go`, add
  `github_workspace_settings.task_git_credentials_mode TEXT NOT NULL DEFAULT 'managed'` to fresh
  schema and a fail-loud, idempotent startup migration. Include it in defaults, normalization,
  reads, upserts, partial patches, and workspace-settings copy.
- In `apps/backend/internal/github/workspace_settings_service.go`, reject unknown policy values
  without mutating the row.
- Add a non-secret service description that combines the policy with
  `github_workspace_connections` and, for Apps, registration display identity. It returns the
  selected mode, method, and known actor context without resolving or exposing a credential.
- Extend the existing `/api/v1/github/workspace-settings` response/update contract. Authorization
  remains the existing workspace-settings authorization path.

### Launch/resume resolution and session snapshot

- In `apps/backend/internal/task/models/models.go`, add
  `SessionMetaKeyGitCredentialSnapshot`, a versioned `GitCredentialSnapshot` type, and a
  JSON-rehydration-safe loader.
- In `apps/backend/internal/orchestrator/executor/executor.go` and
  `executor_credentials.go`, add a task Git credential policy resolver wired beside
  `GitHubCredentialLeaseIssuer`. Resolve one of:
  `workspace`, `executor_profile`, or `executor`.
- Move effective GitHub credential configuration out of the middle of
  `buildLaunchAgentRequest` so it runs after profile/request environment is final. Reuse the same
  function from `buildResumeRequest` after `resolveAllRepoInfo`.
- For managed workspace credentials, clear stale broker keys, reset the GitHub HTTPS helper chain
  with an empty helper before Kandev's helper, set `credential.useHttpPath=true`, and disable
  terminal prompts. Executor inheritance removes/omits Kandev broker/helper/shim configuration.
- Put the candidate snapshot on the in-memory session before lifecycle launch, but persist it only
  through the existing successful `persistLaunchState` / `persistResumeState` full-row write. A
  failed operation leaves the prior durable snapshot unchanged.
- In `apps/backend/internal/backendapp/orchestrator.go`, adapt the GitHub service's non-secret
  policy descriptor into the executor interface alongside the existing lease adapter.

### Managed runtime tools and CLI refresh

- Refactor `apps/backend/cmd/agentctl/github_cli_shim.go` so the control process always creates one
  private managed GitHub tool directory containing both `gh` and `agentctl` links/copies, records
  its path, and does not globally prepend it for every instance.
- In `apps/backend/internal/agentctl/server/config/config.go`, prepend that directory only when the
  instance's additional environment contains the complete broker contract. All agent processes,
  shells, background commands, and agentctl Git operations then share the same per-instance PATH.
- Preserve `runGitHubCLIShim` lookup of the real `gh` outside the managed directory. Executor
  inheritance and profile-token override instances must see the normal executor PATH.
- Add a real subprocess regression around the standalone shape: control `agentctl` starts without
  broker env, a broker-enabled instance receives it later, and both `git credential fill` and
  `gh` resolve through the managed tools. Missing broker/helper fails instead of consulting an
  ambient helper.
- In `apps/backend/internal/github/auth_resolver.go`, give named-CLI cached credentials a separate
  five-minute cache deadline without pretending the provider token itself has an expiry.
  Generation invalidation remains immediate.

---

## Frontend

### Unified GitHub access configuration

- Extend `GitHubWorkspaceSettings` and `UpdateGitHubWorkspaceSettingsRequest` in
  `apps/web/lib/types/github.ts`.
- Refactor `apps/web/components/github/github-task-credentials-section.tsx` from a standalone
  settings section into reusable task-access summary and dialog controls with two explicit choices:
  **Managed workspace credentials** and **Inherit executor Git credentials**. The dialog owns the
  draft, failure feedback, and saved baseline.
- Visible copy must explain local/Worktree host behavior, remote executor behavior, profile-token
  precedence, and when each choice applies. Supplementary responsive help explains the injected
  `agentctl` Git credential helper, scoped broker lease, broker-aware `gh` shim, and launch/resume
  timing without exposing credentials.
- Update `github-auth-method-list.tsx`, `github-connection-dialog.tsx`, and `github-settings.tsx`
  so PAT, named CLI, and App cards state where their credential is stored/resolved and how managed
  tasks receive it; the task policy controls live below the method controls in the same bounded
  dialog/drawer. Remove the standalone Task Git credentials section from the settings page.
- Replace the PAT, CLI, and task-policy submission buttons with one dialog-level **Save changes**
  action in a fixed bottom row that persists every changed local draft. Keep one scrolling content
  region above it, add a bottom fade as an overflow cue, place the GitHub CLI card first, and use
  compact spacing between task-access options. App create/import/install remain workflow actions
  because they navigate through GitHub instead of saving a local credential draft.
- Update `github-status.tsx` so the Workspace GitHub access summary shows the current task access
  mode beside the automation identity and refreshes after a successful dialog submission.
- Desktop retains a bounded but wider dialog. Mobile retains the existing full-height Drawer, one
  internal scroll owner, a safe-area-aware fixed footer, 44px controls, and no horizontal overflow.

### Changes branch credential disclosure

- Add a pure parser/view-model module beside the Changes panel that reads
  `git_credential_snapshot` from the active `TaskSession.metadata`, tolerates malformed/legacy
  values, and produces display labels without guessing unknown actors.
- Extract the current branch-card body from
  `apps/web/components/task/changes-panel-header.tsx`. Keep the compact fine-pointer trigger, but
  make it keyboard-focus accessible and include policy, effective method/source, actor, and
  transport below the existing branch/base-branch rows.
- Use `useTouchDrawer` for coarse pointers. The mobile trigger is at least 44px and opens a Drawer
  containing the same branch editing/base-branch controls and credential truth; desktop keeps the
  hover/focus card.
- Select the active session snapshot in `useChangesPanelData` and pass the normalized disclosure
  through both desktop `changes-panel.tsx` and `mobile/mobile-changes-panel.tsx`.

---

## Tests

- **What:** fresh/replayed schemas default existing and new workspaces to `managed`; policy
  round-trips, partial updates reject invalid values, and settings copy includes policy but not
  authentication.  
  **File:** `apps/backend/internal/github/workspace_settings_test.go`,
  `copy_test.go`, and focused migration tests.  
  **How:** real SQLite store/service tests, including same-database schema replay.
- **What:** the internal policy description identifies PAT/CLI humans and App registration context
  without resolving tokens, while executor actors remain unknown.  
  **File:** focused tests under `apps/backend/internal/github/`.  
  **How:** table-driven service tests with fake connections/registration store.
- **What:** initial launch and resume produce the same managed/executor/profile-override contract;
  managed helper chains reset, inheritance injects nothing, and only successful lifecycle launches
  persist a new session snapshot.  
  **File:** `apps/backend/internal/orchestrator/executor/executor_credentials_test.go`,
  `executor_resume_test.go`, and launch-path tests.  
  **How:** table-driven unit tests plus mocked lifecycle success/failure integration tests.
- **What:** host/executor/profile/task indexed Git config blocks compose in precedence order,
  already-forwarded suffixes occur once, malformed blocks fail clearly, and unrelated settings
  remain active beside Kandev's managed helper.  
  **File:** new shared utility tests,
  `apps/backend/internal/agentctl/server/config/config_test.go`,
  `apps/backend/internal/agent/runtime/lifecycle/executor_docker_test.go`, and focused remote tests.  
  **How:** table-driven block composition plus a temp-repository real Git commit/hook subprocess
  regression using locstat-shaped entries.
- **What:** standalone per-instance broker activation makes `agentctl` and `gh` discoverable even
  though the control process started without broker env; inherit/override instances do not activate
  the tool directory.  
  **File:** `apps/backend/cmd/agentctl/github_cli_shim_test.go`,
  `apps/backend/internal/agentctl/server/config/config_test.go`.  
  **How:** temp executable/subprocess and fake HTTP broker tests; invoke real Git credential
  plumbing where supported.
- **What:** named CLI cache entries re-resolve after five minutes and still invalidate immediately
  on generation changes.  
  **File:** `apps/backend/internal/github/auth_resolver_test.go`.  
  **How:** deterministic fake clock and call counter.
- **What:** settings drafts load, save, discard, and explain both policies and every automation
  method.
  **File:** `apps/web/components/github/github-connection-dialog.test.tsx` and focused tests beside
  `apps/web/components/github/github-task-credentials-section.tsx` if state is extracted.
  **How:** mocked workspace-settings API promises; verify compact summary, explicit dialog save,
  close/discard behavior, refresh, and failure retention.
- **What:** valid/malformed/missing session snapshots produce truthful disclosure labels, never an
  inferred actor.  
  **File:** new Changes credential view-model test and focused header component test.  
  **How:** table-driven Vitest plus keyboard-focus component test under `TooltipProvider`.

Targeted pre-PR commands are recorded in the task files.

---

## E2E Tests

- **Scenario:** GIVEN workspace settings, WHEN the user sees the compact Task access summary, opens
  Change GitHub connection, switches to executor inheritance, and explicitly saves, THEN the API
  persists `executor`, the summary updates, reopening retains the saved mode, and the automation
  identity remains unchanged.
  **File:** extend
  `apps/web/e2e/tests/integrations/github-workspace-settings.spec.ts`.
- **Scenario:** GIVEN a coarse-pointer settings viewport, WHEN Change GitHub connection opens, THEN
  its full-height Drawer contains the task access controls in the same single scroll body, the
  controls meet 44px targets, saving updates the page summary, and neither surface has horizontal
  overflow.
  **File:** extend
  `apps/web/e2e/tests/integrations/mobile-github-workspace-settings.spec.ts`.
- **Scenario:** GIVEN a session seeded with a managed CLI or App snapshot, WHEN a desktop user
  hovers/focuses the Changes branch trigger, THEN branch/base information and launch-time actor,
  method, and managed transport are visible.  
  **File:** `apps/web/e2e/tests/git/git-credential-identity.spec.ts`.
- **Scenario:** GIVEN an inherited or profile-token snapshot on a coarse-pointer task view, WHEN the
  branch trigger is tapped, THEN a Drawer labels the actor runtime-selected, has a 44px trigger, and
  does not overflow.  
  **File:** `apps/web/e2e/tests/git/mobile-git-credential-identity.spec.ts`.

Production E2E runs only after rebuilding backend and Vite artifacts.

---

## Public Documentation

- Update `docs/public/integrations.md`, `docs/public/executors.md`, and
  `docs/public/use-kandev.md` to separate workspace automation method from task credential policy,
  describe named CLI brokerage, explain executor inheritance, and document the Changes snapshot.
- Update `docs/public/integrations.md` so users find task access inside **Change GitHub connection**
  and understand the compact Workspace GitHub access summary; do not describe a standalone Task Git
  credentials section.
- Update troubleshooting so `agentctl: command not found` is a managed-runtime defect rather than a
  prompt to fall back to personal SSH, and distinguish managed-helper failure from missing executor
  credentials.

---

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-00-indexed-git-config-composition](task-00-indexed-git-config-composition.md)
- [x] [task-01-policy-persistence](task-01-policy-persistence.md)

Wave 2 (parallel candidates — user authorization required):

- [x] [task-02-launch-resume-snapshot](task-02-launch-resume-snapshot.md)
- [x] [task-04-settings-explanation](task-04-settings-explanation.md)

Wave 3 (parallel candidates — user authorization required):

- [x] [task-03-managed-runtime-tools](task-03-managed-runtime-tools.md)
- [x] [task-05-changes-identity-disclosure](task-05-changes-identity-disclosure.md)

Wave 4:

- [x] [task-06-e2e-and-documentation](task-06-e2e-and-documentation.md)

Wave 5:

- [x] [task-07-unified-github-access-settings](task-07-unified-github-access-settings.md)

The default execution is sequential in this primary conversation. Waves identify dependency-safe
opportunities only and do not authorize subagents.

## Risks

- Local and Worktree agents retain host process authority. Managed routing can fail closed for
  Kandev-injected GitHub HTTPS/`gh`, but cannot prevent a capable agent from selecting SSH or
  another host tool.
- The helper-reset entry must be ordered before Kandev's helper without deleting unrelated
  caller-supplied Git config.
- Indexed Git config keys form one structured protocol. Plain map overlay at any launch boundary
  can regress host tooling; arbitrary de-duplication can also change the meaning of multi-valued
  Git settings.
- Docker and remote control processes can already contain the task suffix before it is forwarded
  to an instance, so composition must remove only an exact boundary overlap.
- Resume must resolve every attached repository, not only the legacy primary fields.
- The snapshot is launch-time truth. Rendering current workspace status in its place would
  reintroduce misleading identity.
- Agentctl tool-directory activation must be per instance so executor inheritance and explicit
  profile-token overrides do not accidentally invoke the broker shim.
- Workspace connection and task policy remain separate persistence operations inside one dialog.
  The single submission must report complete success only when every changed operation succeeds,
  refresh any partial success, and preserve failed or unsaved drafts for retry.
