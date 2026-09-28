---
id: "10-review-remediations"
title: "Close review findings for provider lifecycle and contract validation"
status: done
wave: 10
depends_on:
  - "02-authenticated-transport"
  - "03-profile-admission"
  - "04-provision-bootstrap"
  - "05-recovery-cleanup"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PLUGIN-001
  - REQ-EXECUTORS-PLUGIN-003
  - REQ-EXECUTORS-PLUGIN-004
  - REQ-EXECUTORS-PLUGIN-006
  - REQ-EXECUTORS-PLUGIN-007
acceptance_criteria:
  - AC-EXECUTORS-PLUGIN-001.2
  - AC-EXECUTORS-PLUGIN-003.1
  - AC-EXECUTORS-PLUGIN-004.1
  - AC-EXECUTORS-PLUGIN-004.3
  - AC-EXECUTORS-PLUGIN-004.4
  - AC-EXECUTORS-PLUGIN-004.2
  - AC-EXECUTORS-PLUGIN-001.6
  - AC-EXECUTORS-PLUGIN-001.7
  - AC-EXECUTORS-PLUGIN-006.2
  - AC-EXECUTORS-PLUGIN-007.6
system_design:
  - ../../specs/executors/system-design/remote-executor-plugins.md
---

# Task 10: Close review findings for provider lifecycle and contract validation

## Summary

Fix the five initial implementation findings and eight additional PR findings in the remote executor
plugin lifecycle. Preserve the plugin provider architecture and durable ownership rules.

## Scope

- Use one concurrency-safe bootstrap token source for HTTP, WebSocket and `Client.AuthToken()`.
- Retain replaced provider credentials while inventory can reference them; block clear or profile deletion
  while a retained environment depends on the profile.
- Reopen stopped plugin sessions by attaching the exact recorded resource and retaining its runtime
  identity, auth and conversation state. Never provision implicitly on reuse.
- Recover every durable nonterminal bootstrap phase. Attach with persisted auth; otherwise perform
  idempotent cleanup and retain unknown cleanup outcomes.
- Compare inventory checkpoints with the caller-observed envelope revision, enforce phase transitions,
  and merge only the plugin envelope into the latest metadata.
- Preserve documented legacy `local_pc` routing while unknown executor types fail closed.
- Recover cleanup-pending operations by their original operation ID when the resource handle is missing;
  distinguish recovered resources, confirmed absence, and unknown outcomes.
- Report `absent` for fixture operations that never allocated a resource.
- Submit blank optional non-secret profile values so the UI can clear them.
- Validate provider state against a required, closed schema, including scalar types, enums and numeric
  bounds, before checkpoint or inventory persistence.
- Require bounded resources to provide a parseable absolute expiry within the declared maximum lifetime.
- Reject malformed numeric schema bounds and resource-state schemas that permit extra fields.
- Keep fine-pointer desktop profile controls at 28px and measure touch/coarse-pointer controls at 44px or
  larger.
- Authorize task-write scope before task environment reset lookup or provider cleanup.

## Acceptance

- A real bootstrap handshake is followed by an authenticated WebSocket upgrade.
- Rotation keeps old inventory references recoverable; clear/delete is guarded until retained inventory is
  settled and historical credentials are cleaned only after profile deletion.
- Manager stop/reopen attaches the same resource and does not provision or destroy it; conversation state
  and authenticated runtime identity remain intact.
- Restart recovery covers `artifact_staging`, `bootstrapping`, `provisioned`, and `ready` before and after
  handshake-token persistence. Unconfirmed cleanup remains retryable.
- A stale inspection checkpoint cannot overwrite cleanup's `absent` state or unrelated metadata.
- Cleanup without a handle queries the original operation and only destroys a recovered exact resource;
  `absent` settles inventory and `unknown` retains it.
- Provider checkpoints reject undeclared, missing-required, invalid-type, enum-invalid, out-of-bound, or
  secret resource-state fields. Bounded expiry must parse and fit the capability limit.
- Task reset by a non-owner stops before environment lookup and provider cleanup.
- Optional non-secret profile fields can be cleared; secret fields retain explicit clear semantics.
- Desktop and phone rendered controls meet the 28px and 44px size contracts respectively.

## Verification

Run from the repository root unless a command says otherwise. Set `TMPDIR` and `GOTMPDIR` to a writable
workspace-backed directory when `/tmp` is full.

```bash
(cd apps/backend && go test ./internal/agent/runtime/agentctl ./internal/gateway/websocket ./internal/agent/executor ./internal/task/models ./internal/task/service ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite ./internal/plugins/... -count=1)
(cd apps/backend && go test -race ./internal/agent/runtime/agentctl -run '^TestPluginExecutorEndpointBootstrapHandshake$' -count=1)
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'TestPluginExecutor|TestManagerReopensStoppedPluginExecutorByAttachingRetainedEnvironment' -count=1)
(cd apps/backend && go test -race ./internal/task/service -run '^TestPluginExecutorProfileSecrets$' -count=1)
(cd apps/backend && go test -race ./internal/task/repository/sqlite -run '^TestPluginExecutorInventoryCAS$' -count=1)
(cd apps/backend && go test -race ./internal/task/repository/sqlite -run '^TestPluginExecutorPostgres' -count=1)
make -C apps/backend build
```

Required regressions:

- `TestPluginExecutorEndpointBootstrapHandshake`
- `TestPluginExecutorProfileSecrets`
- `TestManagerReopensStoppedPluginExecutorByAttachingRetainedEnvironment`
- `TestPluginExecutorPreHandshakeRecoveryCleansKnownResources`
- `TestPluginExecutorPreHandshakeUnknownDestroyRetainsCleanupInventory`
- `TestPluginExecutorPostHandshakeRecoveryAttachesEveryCheckpointPhase`
- `TestPluginExecutorStaleInspectionCannotResurrectCleanedInventory`
- `TestPluginExecutorInventoryCAS`
- `TestRunOwnerLaunchCreatesWorkspaceWithoutTaskMarker` and executor mapping tests for `local_pc` and unknown types

PostgreSQL verification requires the disposable test database selected by `KANDEV_TEST_POSTGRES_DSN`.

## Results

Completed all five review remediations and preserved the legacy `local_pc` executor alias while unknown
executor values fail closed.

Validation passed:

- Full affected Go package suites passed after the lifecycle sanitizer assertion was aligned with the
  established `[path-redacted]` output: agentctl, gateway WebSockets, task service, task models, executor
  mapping, lifecycle, SQLite repository, and all plugin packages.
- Race tests for the authenticated post-bootstrap WebSocket handshake, retained-session manager reopen
  and plugin recovery, profile secret rotation/clear/delete, SQLite inventory CAS, gateway transports,
  fixture provider, and plugin packages.
- PostgreSQL inventory CAS and cleanup-claim tests passed under the race detector against the disposable
  local PostgreSQL 16 instance.
- Root `make build` passed, including the web production build and all backend/runtime binaries.
- Fresh PR capture runs passed: desktop `remote-executor-pr-capture.spec.ts` and phone
  `mobile-remote-executor-pr-capture.spec.ts` each passed under the managed E2E runner; all four
  synthetic-data screenshots were manifest-checked and compressed.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py` (36 passed),
  `python3 scripts/lint-spec-files.py --all`, and `git diff --check` passed after the documentation edits.
- Changed-package Go lint (`bash scripts/lint-go-changed`) passed with zero findings after simplifying
  the remote executor lifecycle and inventory paths.

The lifecycle test previously asserted for an obsolete `***` sanitizer marker. Its expectation now matches
the existing `[path-redacted]` contract; runtime behavior is unchanged.

## Additional PR review remediation

The follow-up review closed eight findings: missing-handle cleanup recovery, fixture absence reporting,
optional profile-field clearing, provider state validation, bounded expiry validation, manifest bound
validation, desktop/phone control sizing, and reset authorization ordering. Requirements, system design,
the implementation plan, and public manifest/authoring references record these contracts.

Post-fixup checks passed:

- `make -C apps/backend build` passed for agentctl, kandev, and the runtime helper binaries.
- `go test ./cmd/plugin-fixture ./internal/plugins/manifest ./internal/agent/runtime/lifecycle -run
  'TestPluginExecutorFixtureReportsAbsentForUnknownOperation|TestPluginExecutorResetRecoversMissingCleanupHandle|TestPluginExecutorResourceStateMatchesProviderSchema|TestPluginExecutorBoundedResourceRequiresValidExpiry|TestPluginExecutorResourceExpiryRejectsInvalidAbsoluteDate|TestValidateExecutorProviderRejectsInvalidNumericBounds|TestValidateExecutorProviderRequiresClosedResourceStateSchema' -count=1`
  passed. `go test ./internal/task/service -run
  '^TestResetPluginExecutorEnvironmentRejectsForeignTaskBeforeProviderCleanup$' -count=1` also passed.
- `pnpm exec vitest run components/settings/plugin-executor-profile.test.tsx lib/plugins/executor-profile-schema.test.ts`
  passed (5 tests), including optional profile-field clearing. `pnpm run typecheck` passed.
- `TMPDIR=/root/k.D6zpBC GOTMPDIR=/root/k.D6zpBC pnpm e2e:run --project mobile-chrome tests/settings/mobile-plugin-executor-profiles.spec.ts`
  and `TMPDIR=/root/k.D6zpBC GOTMPDIR=/root/k.D6zpBC pnpm e2e:run tests/settings/plugin-executor-profiles.spec.ts`
  each passed (1 test); phone controls meet the measured touch target and desktop controls meet the 28px contract.
- Fresh managed-runner capture specs passed on `mobile-chrome` and `chromium`. The manifest contains the
  two refreshed profile screenshots plus the two unchanged retention screenshots; every listed image
  exists, uses fixture data, and was compressed with the supported pngquant fallback.
- Commit hooks passed, including changed-package Go lint, web lint, i18n ratchet, documentation catalog,
  architecture and specification checks. The current-base merge was validated with the same hooks.
- CI follow-up supplied the missing Japanese `sshIdentityFileHint` translation; `pnpm run i18n:check`
  passed with all real locale catalogs complete. The local PR documentation-coverage evaluator reports
  `covered` after Task 10 links criterion 003.1 to its owning requirement.
- Documentation checks passed after the final edits: catalog validation (312 decisions, 1189
  specifications), all specification files linted, 36 spec-linter tests, 62 public-doc validation tests,
  and all 47 public pages validated. `git diff HEAD --check` passed.

CI follow-up for head `820a6611e00`:

- E2E setup found that the packaged backend prompt-history fixture no longer matched the tracked web
  fixture after the base update. The generated backend bundle now matches the web fixture; both have
  SHA-256 `188c98aa0e78a0396c7d9dd57f07a5086be71d7edd53e142e0c3e08e2e7a56e4`. After Prettier restored
  the web fixture's tracked formatting, `make -C apps/backend e2e-plugin-package` regenerated the
  backend bundle, passed, and wrote the E2E plugin identity.
- Backend Static Checks found repeated manifest schema strings. Named constants now cover the bounded
  retention and object/boolean schema types. The exact CI command,
  `golangci-lint run ./... --new-from-rev=dfce4dac05809c0fcec156166f5479f14b0cdb76 --timeout=10m`,
  passed with zero issues.
- `E2E_DEBUG=1 E2E_PORT_OFFSET=29 TMPDIR=/root/k.D6zpBC GOTMPDIR=/root/k.D6zpBC pnpm e2e:run
  --host --no-build --project chromium tests/settings/plugin-executor-profiles.spec.ts` passed (1 test)
  after bundle regeneration, including managed backend readiness and fixture plugin setup. A previous
  default-offset attempt timed out waiting for backend readiness while other E2E jobs were active; the
  isolated-offset reruns passed.
- The PR documentation coverage evaluator reports `covered`; the Task 10 criterion mapping remains
  valid. Final-head PR checks are tracked separately in the implementation task plan.

A broad multi-package Go test invocation later exceeded its 600-second limit while running
`internal/task/service` alongside another suite. It reported no failing assertion. The review-specific
service regression passed in isolation, and the earlier full-suite and race results above remain recorded
for the initial five findings.

## PR fixup for the final pre-push checks

The final PR check run exposed two test issues and a backend test-helper race. The backend workflow-step
fixture iterated a map and could return a non-nearest higher-position step; it now returns the minimum
higher position. `go test -race ./internal/mcp/handlers -run
'^TestPeerMessageInitialLaunch_QueuesBeforeWorkflowTurnPreparation$' -count=20` and
`go test ./internal/plugins/manifest` passed. The manifest schema type check also reuses the existing
string-type constant to satisfy the exact changed-code lint command.

The repository-secret E2E now pins the worktree profile so it exercises the setup-script path instead of
the default local reuse path, which skips setup. The mobile policy-subtask E2E now selects the seeded
local profile already loaded by the UI and scopes the option to the active profile drawer. This avoids
creating a profile after the page's executor list is cached and avoids retrying a tap against a stale,
offscreen option. Both exact tests passed with retries disabled:

- `E2E_DEBUG=1 E2E_PORT_OFFSET=28 TMPDIR=/root/k.D6zpBC GOTMPDIR=/root/k.D6zpBC pnpm e2e:run --host --no-build --project chromium e2e/tests/settings/repository-secrets.spec.ts -- --grep "passes the resolved binding to setup and a new local terminal" --retries=0`
- `E2E_DEBUG=1 E2E_PORT_OFFSET=27 TMPDIR=/root/k.D6zpBC GOTMPDIR=/root/k.D6zpBC pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/task/mobile-sidebar-task-actions.spec.ts -- --grep "creates a policy branch for a local-executor subtask" --retries=0`

The exact CI Go lint command passed with zero issues. `make -C apps/backend build`, Prettier, ESLint,
`gofmt -l`, and `git diff --check` passed. PR #3985 code-fixup head
`67511e8bcc9be52330e37c9e46dbba3a1d60e3f5` reached a terminal run with 60 passed, 10 skipped, and no
failed or pending checks. The documentation-coverage publisher first hit GitHub code-search HTTP 429;
the failed-job rerun passed after the reported retry delay elapsed. No review threads remain unresolved.

## Touch-target retry audit

The retry audit for the green CI rerun at `986910f1c18` found that the phone plugin-executor status
test passed only after retry: a refresh control rendered at `43.99993896484375px`, just below the
44px target. Touch controls in the executor disclosure now use 48px nominal height and width,
including the drawer close and reset actions. The Playwright helper checks both dimensions and covers
the drawer close action. The component test expects the 48px classes.

Local validation passed after `make build-web`: the executor disclosure component tests (12 passed),
the phone and desktop plugin-executor status specs (one test each, retries disabled), and the phone
Kubernetes task-environment spec (one test, retries disabled). Targeted Prettier and ESLint checks
passed. Fresh PR checks for this follow-up commit are tracked in the implementation task plan.

## Dependencies

[Task 02](task-02-authenticated-transport.md), [Task 03](task-03-profile-admission.md),
[Task 04](task-04-provision-bootstrap.md), and [Task 05](task-05-recovery-cleanup.md).

## Parallelism

`sequential`
