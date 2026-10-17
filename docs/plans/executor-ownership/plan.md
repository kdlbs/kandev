---
created: 2026-10-15
status: done
requirements:
  - REQ-EXECUTORS-OWNERSHIP-001
  - REQ-EXECUTORS-OWNERSHIP-002
  - REQ-EXECUTORS-OWNERSHIP-003
  - REQ-EXECUTORS-OWNERSHIP-004
  - REQ-EXECUTORS-OWNERSHIP-005
system_design:
  - ../../specs/executors/system-design/per-user-ownership.md
legacy_specs: []
---

# Implementation plan: Per-user executor ownership

## Overview

Deliver [#4512](https://github.com/kdlbs/kandev/issues/4512): private executor
compute and profile credentials on a shared board, preserving shared defaults.
The [requirements](../../specs/executors/requirements/per-user-ownership.md) and
[system design](../../specs/executors/system-design/per-user-ownership.md) are
implemented in the primary session after the explicit implementation request. See the [ownership ADR](../../decisions/2026-10-15-private-executor-admission.md).

The executor system owns this package because executor/profile eligibility is
its durable admission contract; auth provides identity and scopes. Execute
sequentially: persistence/policy, protected catalogue/configuration, interactive
admission, autonomous attribution, then complete UI and public documentation.
Do not advertise ownership as a secure feature until all entry points pass.

## Scope

### In scope

- Optional executor/profile ownership, compatible storage, owner/admin policy.
- Private catalogue/configuration and live updates across all transports.
- Current-user admission for interactive runtime use and existing-session exec.
- Creator attribution for background watches, automations, and workflows.
- Desktop/mobile ownership controls and picker behavior, seven locales, docs.

### Out of scope

Allowed-user lists, workspace grants, ownership of agent profiles or other
shared configuration, a new scope, OS credential sandboxing, and stopping
existing admitted executions on ownership transfer. No subagents are authorized.

## Technical approach

1. Extend `Executor` and `ExecutorProfile` in
   `apps/backend/internal/task/models/models.go`; versioned migrations and every
   writer in `internal/task/repository/sqlite/` must preserve ownership,
   including script-intent and optimistic saves. Add a focused policy module
   using `authn.Identity` and `authz.ScopeOrgConfigManage`. Never translate a
   missing identity or `Subject.Unscoped` into owned-compute authority.
2. Guard services in `internal/task/service/service_resources.go` and
   `executor_provider_profiles.go`, HTTP/WS handlers, configuration MCP, typed
   settings operations, boot state, and recipient-specific gateway events.
   Catalogue invalidation must evict revoked data without carrying private
   profile payloads. Preserve maintenance repository reads separately.
3. Derive actual user identity in task/session launch paths; propagate a trusted
   durable actor through queues, orchestrator, runtime, and session execution.
   Check after defaults/inheritance resolution and before secrets, scripts,
   provisioning, or prompt delivery. Stored actor data is server-controlled.
4. Add/reuse verified creator persistence on workflows, automations, and provider
   watches, then restore current identity at autonomous dispatch. Keep provider
   issue-author filters separate. Updates and interactive triggers targeting
   private compute must also authorize the actual caller.
5. Extend API/WS/store types and existing Settings/profile editor, task-create,
   workflow/automation/watch pickers. Share domain state across desktop/phone;
   add ownership controls and invalid-default recovery. Update the public docs
   and qualify the existing authentication shared-surface contract only when
   behavior is implemented.

Paths abbreviated with `internal/` above are relative to `apps/backend/`.

### Provider and identity compatibility

| Provider/runtime | Transport/identity | Intended behavior | Planned evidence | Unsupported/missing identity |
| --- | --- | --- | --- | --- |
| SSH | SSH plus agentctl; actual launch actor | Parent/profile checks before connection/seeding | Fake connection and credential spies in Task 03 | Owned denied before SSH |
| Remote Docker | Remote daemon; actual actor | Same check before Docker/config/build work | Remote selection and fake daemon tests | Owned denied before daemon call |
| Kubernetes | Kube client; actual actor | Same admission; existing config admin gates retained | Admission/client-call spies | Owned denied before pod creation |
| Sprites | Provider API; actual actor | Same admission before token resolution | Secret-store/provider spies | Owned denied before token read |
| Plugin remote | Host-side profile plus provider dispatch | Policy before provider/secret access | Plugin launch fake in Task 03 | Missing provider stays unavailable; no fallback |
| Local/Worktree | Host process; actual actor | Same selection/use policy; no OS credential isolation promise | Local fake launch and shared-default regressions | Shared legacy behavior retained |
| Autonomous sources | Durable creator plus current account | Creator admission and interactive caller checks | Task 04 source matrix | Unknown creator may use shared only |
| Passive recovery/cleanup | Trusted runtime inventory | Observe/clean existing resources only | Reattach/no-new-prompt regression | Cannot provision or deliver work |

## ASCII UI preview

### UI-01: Settings ownership (desktop)

Entry: Settings > Executors > a visible profile. State: configuration manager.

```text
Profile: Personal SSH
Ownership     [ Ana Silva                 v ]
              Shared | Me | Choose user...
[ Discard ]                         [ Save changes ]
```

The owner sees Shared and Me; another user's assignment is manager-only.
Shared is the explicit unowned value. Ownership-only changes do not unlock
provider configuration controls. Save failure retains the draft and exposes
an accessible error; loading user choices is distinct from no choices.

### UI-02: Settings ownership (phone)

Entry: the same direct profile route. State: choosing an owner.

```text
< Executors
Personal SSH
Ownership
[ Ana Silva                              v ]
... existing profile sections ...
[ Save changes ]

+------------------------------------------+
| Ownership                          Close |
| [ Search users...                      ] |
| Shared                                   |
| Me                                       |
| Ana Silva                            (o) |
+------------------------------------------+
```

Use direct navigation for the full editor and an inset bottom Drawer for the
temporary choice. The form owns page scrolling; an overflowing drawer owns its
option scrolling with a fixed title. Preserve safe-area clearance, keyboard
focus return, at least 44px phone/coarse-pointer targets, and no page overflow.
Desktop ordinary controls retain 28px density. Names and spacing are illustrative;
control order, permitted options, semantic labels, and responsive surfaces are
structural requirements. Reuse the existing Settings save/discard behavior.

### UI-03: Task-create revoked default (both viewports)

```text
Executor       [ Choose an available executor   v ]
This executor is no longer available. Choose another.
[ Cancel ]                            [ Start: disabled ]
```

Eligible shared/owned options are the only returned choices. Do not display the
revoked profile's name/configuration or silently substitute another machine.
Retain the task draft while the user repairs selection. On phone, use the
existing task-create picker surface with the same filtered state. All preview
copy is illustrative and must use translated locale keys in implementation.

These views map to AC-EXECUTORS-OWNERSHIP-002.4 and
AC-EXECUTORS-OWNERSHIP-003.1 through .4. Rendered proof belongs to Task 05.

## Tests

Planned test files and test names are implementation deliverables, not claims
that tests currently exist or have passed. Use TDD for each work order.

| Acceptance criteria | Focused evidence to add |
| --- | --- |
| 001.1, 001.3, 001.4 | `repository/sqlite/executor_ownership_test.go`: `TestExecutorOwnershipMigrationAndPersistence`; shared SQLite/PostgreSQL scenario in `persistence/storeconformance/executor_ownership_test.go` |
| 001.2, 002.1, 002.3 | `task/service/executor_ownership_test.go`: `TestExecutorOwnershipPolicy`, `TestExecutorOwnershipMutationRace` |
| 002.2, 003.1, 003.2 | `task/handlers/executor_ownership_test.go`, `mcp/handlers/config_executor_ownership_test.go`, `backendapp/executor_ownership_test.go`: HTTP/WS/MCP/settings/boot denial cases |
| 002.3, 003.3 | `gateway/websocket/executor_ownership_test.go`: transfer invalidation, no foreign payload, removed cache, stale-response fencing |
| 004.1 through .4 | `orchestrator/executor/executor_ownership_test.go`, `agent/runtime/lifecycle/executor_ownership_test.go`, `gateway/websocket/terminal_executor_ownership_test.go`: side-effect spies and queued-revocation cases |
| 005.1 through .4 | `automation/executor_ownership_test.go`, `workflow/engine/executor_ownership_test.go`, each provider's `executor_ownership_test.go`, `backendapp/executor_ownership_wiring_test.go`: creator/current-role/legacy/import/edit/retry matrix |
| 002.4, 003.1 through .4 | Web domain/WS tests plus desktop and phone E2E below |

All backend test paths above start at `apps/backend/internal/`. Each test must
use real service/handler wiring where relevant; policy-only tests cannot prove
there are no side effects in runtime paths. Check every changed test file in its
work order, and record exact results there after implementation.

## E2E tests

- `apps/web/e2e/tests/auth/executor-ownership.spec.ts`, project `auth`: create
  admin/A/B contexts on a shared workspace; shared and A-owned profiles; verify
  A/B catalogues, direct API denial, manager reassignment, live eviction,
  task-create invalid-default recovery, owner save/discard/reload, and unchanged
  shared defaults. Covers 001.1-.4, 002.1-.4, 003.1-.3, and 004.1.
- `apps/web/e2e/tests/auth/mobile-executor-ownership.spec.ts`, project
  `mobile-chrome`: same owner change and eligible task-create outcome via phone
  navigation/Drawer; assert focus return, scroll ownership, no horizontal
  overflow, and measured touch targets. Covers 002.4 and 003.1-.4.
- Real remote compute is not needed to prove pre-admission denial: use service
  spies and mock runtime. Do not claim successful real SSH/Docker/Kubernetes
  launches from these checks. Existing provider suites remain compatibility
  references; new runtime behavior needs additional task-defined evidence.
- Reuse `e2e/helpers/auth.ts` and auth-suite backend restart/teardown patterns;
  each context must carry its own cookie. Use causal waits, never timed sleeps.

## Work orders

- [x] [Task 01: Persist ownership and define policy](task-01-ownership-policy.md)
- [x] [Task 02: Protect configuration and catalogue](task-02-private-catalogue.md)
- [x] [Task 03: Guard interactive compute admission](task-03-interactive-admission.md)
- [x] [Task 04: Attribute autonomous execution](task-04-autonomous-attribution.md)
- [x] [Task 05: Complete ownership UI and documentation](task-05-ownership-ui.md)

## Verification results

Design validation on 2026-10-15:

- `python3 scripts/list-docs.py validate`: passed (385 decisions, 1618 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- Focused cross-reference preflight: passed; five pending work orders, five
  requirements, all 20 acceptance criteria covered; plan/design references and
  local document links resolve. The check parsed the actual document frontmatter
  and requirement definitions rather than assuming links from file names.
- `git diff --check -- docs/plans/executor-ownership docs/specs docs/decisions`:
  passed; `git status --short` confirms the new package is untracked/unstaged.
- Repository `validateCoverage` preflight from `.github/scripts/pr-docs.cjs`:
  blocked because `node` is not installed or available on PATH (`nodejs`,
  `pnpm`, and `npm` are also unavailable). The focused cross-reference check
  above ran instead; it does not claim the repository helper passed.

The paragraphs above record the design-only checkpoint. Implementation is now
authorized and present. Node 24 and pnpm were provisioned, the frozen workspace
install completed, and the repository `validateCoverage` preflight passed.
PostgreSQL conformance ran against an isolated PostgreSQL 17 container via
`KANDEV_TEST_POSTGRES_DSN` as well as SQLite; neither engine was skipped.

## Implementation verification (2026-10-15)

- Requirements, design, and ADR accepted; all five implementation work orders completed in the primary session.
- Frozen workspace install completed with Node 24 and pnpm 9.
- Focused ownership/compatibility tests pass across task service/handlers/storage, runtime/lifecycle, orchestrator/executor, MCP, notifications, source providers, workflow, and production wiring.
- `KANDEV_TEST_POSTGRES_DSN=<isolated PostgreSQL 17 DSN> go test -trimpath -race ./internal/persistence/storeconformance -count=1`: passed, both SQLite and PostgreSQL, including historical upgrade/replay scenarios (247.638s).
- `go run ./cmd/sqlguard ./internal`: passed.
- The Task 05 Vitest command passed 104 tests in 11 files. Full web lint, i18n catalog checks, new-copy ratchet, docs validators, and `validateCoverage` preflight passed.
- Desktop browser suite: 5 passed. Mobile browser suite: 4 passed. Coverage includes actual mock-agent launch for the owner, denial of another shared-board collaborator, sharing, manager reassignment, live eviction, and draft retention across the phone breakpoint with a fine pointer. The final phone capture was inspected.
- Final web typecheck, full web lint, and production Vite build: passed.
- Full orchestrator/executor, Azure DevOps, and automation reruns with the isolated Go cache: passed.
- `bash scripts/lint-go-changed <changed Go files>`: passed for every affected package, split into bounded groups. The task/persistence/MCP group and all remaining runtime/provider groups report zero issues. An earlier cold-cache invocation reached the helper timeout; all final groups completed successfully.
- Final specification catalogue/lint, public documentation validation, PR documentation coverage, and whitespace checks: passed. The task-owned PostgreSQL container was removed after conformance completed.
- Full task-service, lifecycle, and orchestrator suites passed. The unfiltered task repository suite exceeded its 12-minute package timeout while the then-current test had run for one second; this was an overall suite budget, not an established stuck test. Focused changed-path storage and full cross-engine conformance remain the required persistence evidence. A prior broad run was interrupted while investigating duration and is not counted as passing.
- A later shared Go-cache cleanup removed export artifacts during compilation. Remaining affected suites and lint use task-local `GOCACHE=/tmp/kandev-4512-go-cache`; no shared cache was deleted by this task.

## PR delivery verification (2026-10-16)

- Captured and inspected fresh desktop profile, desktop ownership choices, and phone drawer screenshots using synthetic accounts. The temporary capture spec was removed; compressed assets remain ignored under `apps/web/.pr-assets`.
- The initial commit hook caught a prohibited direct lifecycle import in application auth wiring. The composition helper now depends on its narrow admission-setter interface; production wiring tests and architecture lint were rerun before delivery.

## PR remediation (2026-10-16)

- Reproduced the settings-contract freshness failure and four task-create consumer failures from CI. Regenerated the catalogue snapshot and supplied eligible executor/profile fixtures; the three contract checks and 17 affected consumer tests pass without relaxing submission guards.
- Reproduced stale-owner executor/profile deletes through the real service/repository path. Conditional deletes now bind the authorized parent/profile snapshot under the transfer lock. Positive owner deletes and both losing interleavings pass, along with race-enabled ownership tests and SQLite/PostgreSQL ownership conformance (including parent cutover and orphan cleanup).
- The terminal first-head CI report also exposed legacy Office runtime-kind selections being read as executor IDs. The bounded `local_pc` compatibility alias now checks the built-in row while preserving actual same-ID rows (including retired rows), private profile denials, unknown-ID denials, and storage errors at both lookup boundaries. Its service regression failed before the fix and now passes. SSH session inventory uses the documented missing/inaccessible 404 contract; its stale empty-list E2E expectation was corrected.
- Reproduced an unsaved profile/ownership draft loss during an unrelated catalogue notification. The editor remains mounted, hidden, and inert until its identity-scoped catalogue entry is revalidated. Revoked entries still disappear. Ownership cards reuse the shared domain directory hook, and payload-free global broadcasts carry the required justification marker.
- The 15 affected desktop Office launch/chat/attachment and Git diff scenarios pass. Two CI flakes exposed test-only setup races: a discovery route proxied a response across reload, and the diff setup scrolled before its reading anchor settled. The discovery response is now captured before routing and diff setup awaits its existing stable-anchor condition. The mobile suite passes all nine scenarios; desktop diff continuity passes six repeats (three per renderer), and mobile collapse passes three repeats. The corrected SSH unknown-executor contract passed in the containers project with the real SSH fixture.
- Targeted frontend checks passed 34 tests in seven files (including identity-switch/back cache disposal); typecheck, Vite build, and SQLguard passed. The six desktop and five phone browser scenarios pass, including hidden-during-refetch draft retention and actual revocation. Full web lint and specification validators pass. Changed-package Go lint reports zero issues. Normal commit hooks run before the remediation push. Existing public guides already describe save/reset and access revocation; no public behavior contract changed beyond enforcing those guarantees. Remote CI/review verification remains pending for the next head.

## Risks

- Existing internal unscoped contexts and repository-direct runtime reads can
  bypass service-only checks. Guard a common admission seam and prove wiring.
- Shared workflow/automation edits and existing terminals can lend private
  compute unless actual caller authorization is included.
- Ownership-bearing user deletion, parent transfer, and concurrent script saves
  must not accidentally clear or roll back private ownership.
- Creator fields are not present everywhere. No guessed legacy attribution is
  safe; unknown sources may need an explicit eligible creator repair later.
- Provider-specific configuration admin gates and existing workspace/tenant
  privacy remain independent of ownership bypass.
- These sequential intermediate commits are not a complete isolation feature.
  Publish the behavior only after all work orders pass.
