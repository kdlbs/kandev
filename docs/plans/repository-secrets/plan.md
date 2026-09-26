---
spec: docs/specs/workspaces/requirements/repository-secrets.md
decision: docs/decisions/2026-08-03-scope-and-merge-repository-secrets.md
created: 2026-08-03
status: completed
---

# Implementation Plan: Repository and Workspace Secrets

## Overview

Extend the encrypted user secret catalog with Global and Workspace scopes, then let a
workspace-owned repository persist secret-only environment bindings. Build one origin-aware task
environment from the selected executor profile and every attached repository, rejecting ambiguous
keys before provisioning. Carry the validated snapshot through setup, agent, shell, terminal, and
remote transports, while preserving SSH's narrow forwarding boundary. Add scope-aware desktop and
mobile settings, end-to-end coverage, and public security/runtime documentation.

The implementation follows
[ADR-2026-08-03](../../decisions/2026-08-03-scope-and-merge-repository-secrets.md). It does not add
task overrides, precedence winners, namespacing, or live environment mutation.

## Backend

### Scoped secret catalog and consumer policies

- Extend `internal/secrets` models and responses with `scope` (`global` or `workspace`) and nullable
  `workspace_id`. Add an idempotent migration that marks every existing row Global without changing
  its ID, owner, ciphertext, nonce, or timestamps.
- Keep scope immutable after creation. Default omitted scope to Global for API compatibility.
- Wire the secrets service to a narrow workspace-access interface implemented by the task service.
  Workspace create/list/get/reveal/update/delete must authorize the workspace and preserve the
  existing per-user behavior from the authentication ADR. Internal/synthetic callers retain the
  auth-disabled compatibility path.
- Add scope-aware list options: default/global-only, workspace-only, and repository-selectable
  Global-plus-same-workspace. Keep HTTP and WebSocket contracts aligned.
- Add explicit secret-consumption methods for Global-only and Global-or-workspace resolution.
  Settings reveal stays user-authorized but is not used for runtime injection.
- Enforce Global-only secret references in both agent-profile and executor-profile save paths and
  in their runtime resolvers, so stale/manually inserted Workspace IDs also fail closed. Extract the
  shared profile environment key/value validator instead of duplicating its limits and reserved-key
  rules.
- Keep backend-owned integration secret IDs hidden and unchanged. Scope migration and user-visible
  filters must not make internal credentials selectable.
- Cover SQLite and PostgreSQL migration replay, per-user/global/workspace visibility, workspace
  authorization, workspace cascade deletion, immutable scope, list combinations, and global-only
  profile consumption.

### Repository secret bindings

- Add `RepositorySecretBinding` to task models and API DTOs and a normalized
  `repository_secret_bindings` table with a repository cascade, unique `(repository_id, key)`, and
  no secret foreign key.
- Add replay-safe SQLite/PostgreSQL schema coverage and ensure workspace/repository deletion removes
  bindings while secret deletion leaves the reference intact.
- Extend repository create/update request shapes with optional `secret_bindings`. Validate the
  shared environment-key contract, binding count, unique keys, secret visibility, and Global or
  same-workspace scope.
- Persist repository fields and an explicitly supplied binding set atomically. Omission on update
  preserves current bindings; an empty list clears them. Find-or-create and provider/local-path
  backfill paths preserve bindings unless they explicitly submit a replacement.
- Return bindings from repository get/list/create/update and repository events without values or
  secret names. Batch-load bindings for repository lists to avoid one query per repository.
- Add service/repository/handler tests for atomic replacement and rollback, wrong-workspace and
  missing refs, deleted-secret persistence, duplicate/reserved keys, event projection, and
  multi-repository reads.

### Origin-aware environment resolver

- Add a focused resolver under the orchestrator/executor boundary that receives:
  - managed/request environment values with an explicit runtime origin;
  - the selected executor profile's original `ProfileEnvVar` definitions;
  - all attached repository bindings and their repository origins;
  - the task workspace ID and caller context.
- Refactor `executorConfig` to retain source definitions until resolution completes instead of
  flattening/decrypting executor profile env early. Keep the flattened map only as resolver output.
- Compare source identity before revealing repository values. Exact same-key/same-secret bindings
  deduplicate; different secret IDs, literal-versus-secret, different literals, and managed-value
  collisions return a typed conflict error independent of repository position.
- Resolve executor references Global-only and repository references Global-or-same-workspace. Any
  missing, deleted, unreadable, unauthorized, or wrong-workspace repository reference returns a
  typed broken-binding error.
- Redact errors and logs: report the environment key and repository/executor origin labels, never
  plaintext or secret IDs.
- Invoke the resolver in fresh prepare/launch, full relaunch, cold resume, and Reset Environment
  paths before `agentManager.LaunchAgent` and before any repository setup or remote provisioning.
  Workspace-only preparation resolves the same snapshot because setup scripts and a later agent
  start must agree.
- Preserve current agent-profile behavior in lifecycle: Global-only profile values fill missing
  keys after the task environment and cannot overwrite repository/executor results.
- Add table-driven merge tests plus launch-path tests proving order independence, exact-reference
  dedupe, all conflict classes, fail-before-provisioning behavior, cold-resume refresh, and warm
  snapshot retention.

### Runtime and transport propagation

- Keep the resolver output in `LaunchAgentRequest.Env` and the defensive in-memory
  `AgentExecution.runtimeEnv` snapshot. Do not persist plaintext in task, session, environment,
  executor-running, event, or metadata rows.
- Verify Local/Worktree repository setup, executor preparation, standalone agent, Docker, Remote
  Docker, and Sprites receive the resolved map through existing launch plumbing.
- Keep terminal-panel shells authoritative on `RuntimeEnvironment`; only legacy execution fallback
  may re-resolve a Global executor profile. It must not silently reconstruct current repository
  bindings for an old snapshot.
- Extend SSH request/instance configuration with an explicit approved repository environment map
  or approved key set. `sshRemoteAgentEnv` forwards that map plus the existing managed credential
  allowlist, not arbitrary `req.Env` or host process values. Remote agent and remote terminal
  instances share the approved snapshot.
- Clear the runtime map during execution removal as today and ensure logs expose counts/origins but
  not values.
- Add lifecycle/transport tests for setup, process, child shell, new terminal, snapshot copying and
  clearing, SSH approved-key forwarding, and rejection of unrelated host/profile keys.

## Frontend

### Scope-aware secret management

- Extend secret HTTP types and API clients with scope/list options. Replace the single unscoped
  cache assumption with scope-keyed state or isolated hooks so loading Workspace secrets can never
  populate a Global-only agent/executor selector.
- Keep `/settings/general/secrets` as the Global page and make its title/help explicit.
- Add `/settings/workspace/:id/secrets`, route loading, and a Secrets leaf under each workspace in
  `WorkspacesGroup`. Reuse one configurable `SecretsSettings` surface for Global and Workspace CRUD.
- Create Workspace secrets with the route workspace ID, render scope as text/badges, preserve
  reveal/edit/delete behavior, and show authorization/loading errors without displaying values.
- Filter both agent and executor profile secret selectors to Global. Treat a persisted Workspace or
  missing selection as invalid/missing instead of silently selecting another secret.
- Add i18n keys in English and pseudo locales. Keep `t()` at render time and include accessible
  labels for reveal/edit/delete controls.
- Add focused API, state/hook, component, routing, navigation, and selector tests.

### Repository binding editor

- Add `secret_bindings` to repository types, action payloads, clone/dirty/merge helpers, and the
  existing manual-save coordinator.
- Add an **Environment secrets** section to `RepositoryEditView`. Each row has a POSIX key input, a
  Global-plus-current-workspace secret selector with a visible scope label, and a remove action.
  Add creates an empty draft row; repository save owns persistence.
- Preserve an unavailable `secret_id` as a “Missing secret” option so users can repair or remove a
  broken binding. Never reveal values in repository UI or selectors.
- Reuse the repository card composition at desktop and phone widths. At phone width rows stack,
  controls remain touch-sized, the Settings page remains the single scroll owner, and no horizontal
  overflow or desktop-only modal is introduced.
- Validate duplicate, invalid, and reserved keys before save while keeping the backend authoritative.
  Scope/error messages and all new copy use i18n.
- Add component and helper tests for dirty tracking, create/update/clear payloads, selector scope,
  missing refs, validation, save errors, and narrow viewport composition.

## Mobile design contract

- Desktop: Global secrets remain under General; Workspace secrets and Repository bindings are
  reachable from the expanded workspace navigation and repository editor.
- Mobile entry point: the same workspace Settings group exposes a touch-sized Secrets leaf.
- Nearest shipped exemplars: `WorkspacesGroup`, `SettingsPageTemplate`, `SecretsSettings`, and
  `RepositoryCard`; preserve their single content scroll owner and manual-save behavior.
- Layout: secret cards and binding rows stack to one column below the existing responsive breakpoint;
  no table with mandatory horizontal scrolling, hover-only action, nested vertical scroller, or
  fixed footer is added.
- Parity: phone users can perform the complete Workspace secret and repository-binding flow.
- Coverage: add a dedicated `mobile-repository-secrets.spec.ts` so the `mobile-chrome` project owns
  the phone contract, plus focused component assertions for semantics not practical in E2E.

## E2E and documentation

- Add a desktop Chromium flow that creates a Global secret and Workspace secret, verifies scope
  filtering, binds both to a repository, reloads, and observes the persisted rows without revealing
  plaintext.
- Add `mobile-repository-secrets.spec.ts` covering workspace navigation, Workspace secret creation,
  repository binding, save, reload, touch accessibility, and no horizontal overflow.
- Add a launch E2E for a local/worktree task proving a repository binding reaches a setup command,
  mock agent child command, and newly opened terminal; add conflict and deleted-secret launch
  failures through backend/API fixtures.
- Extend the heavyweight `containers` project with focused Docker and SSH checks. The SSH assertion
  proves an arbitrary repository-approved key arrives remotely while an unrelated control-plane
  variable does not. Unit tests remain the matrix authority for Sprites/Remote Docker paths.
- Update `docs/public/agents-and-profiles.md`, `docs/public/executors.md`,
  `docs/public/authentication.md`, and `docs/public/security.md` with scopes, repository inheritance,
  conflict behavior, snapshot/reset semantics, and SSH forwarding.

## Implementation waves

Wave 1:

- [x] [Task 01: Add scoped secret storage](task-01-scoped-secret-storage.md) (`done`)

Wave 2:

- [x] [Task 02: Persist repository secret bindings](task-02-repository-secret-bindings.md)
  (`done`, depends on Task 01)
- [x] [Task 05: Build scope-aware secret settings](task-05-scope-aware-secret-settings.md)
  (`done`, depends on Task 01; parallel-safe with Task 02 only if the user explicitly authorizes
  subagents)

Wave 3:

- [x] [Task 03: Resolve multi-source task environments](task-03-task-environment-resolver.md)
  (`done`, depends on Tasks 01 and 02)
- [x] [Task 06: Add repository binding settings](task-06-repository-binding-settings.md)
  (`done`, depends on Tasks 02 and 05; parallel-safe with Task 03 only if the user explicitly
  authorizes subagents)

Wave 4:

- [x] [Task 04: Propagate approved runtime environments](task-04-runtime-environment-propagation.md)
  (`done`, depends on Task 03)

Wave 5:

- [x] [Task 07: Prove and document repository secrets](task-07-e2e-docs-verification.md)
  (`done`, depends on Tasks 04 and 06)

The primary conversation executes these tasks sequentially unless the user explicitly authorizes
native implementation subagents.

## Risks

- The secret store is shared by user-managed and backend-internal credentials. Scope changes must
  preserve internal adapter behavior and user-visible filtering.
- Shared agent/executor profiles currently store only secret IDs. Save-time filtering alone is not
  sufficient; every runtime resolution path must enforce Global scope.
- Prepared workspaces and warm resumes can outlive configuration edits. Tests and docs must make the
  provisioning snapshot boundary explicit rather than implying live rotation.
- `executorConfig.ProfileEnv` currently loses source identity and skips resolution failures. The
  resolver must retain original definitions long enough to distinguish dedupe, conflict, and broken
  references.
- Managed Git/GitLab/Office values are added at several launch stages. A final origin-aware collision
  check is required so repository values cannot overwrite or be overwritten silently.
- Repository list events and route bootstrap can create N+1 binding reads or stale scoped caches.
  Batch persistence/read APIs and scope-keyed frontend state avoid both.
- SSH must gain repository-approved keys without turning `req.Env` into a blanket forwarding
  channel. The transport contract and negative tests are security-critical.
- Workspace deletion spans task/repository and secret schemas. Cascade behavior must be verified in
  the same database transaction model on SQLite and PostgreSQL.

## Completion verification

- Backend: `make -C apps/backend test` and `make -C apps/backend lint` passed.
- Frontend: 1,089 Vitest files passed (8,309 tests passed, 4 skipped), plus typecheck, lint, Vite
  build, and focused repository-secret tests.
- Browser: Chromium desktop, mobile-chrome, Docker, and SSH repository-secret scenarios passed;
  the desktop and mobile capture assets were inspected and compressed for the PR.
- Documentation: public-doc validation, i18n checks, pseudo-locale generation, and ratchet passed.
