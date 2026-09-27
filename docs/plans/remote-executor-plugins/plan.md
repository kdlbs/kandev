---
created: 2026-09-26
status: implemented
requirements:
  - REQ-EXECUTORS-PLUGIN-001
  - REQ-EXECUTORS-PLUGIN-002
  - REQ-EXECUTORS-PLUGIN-003
  - REQ-EXECUTORS-PLUGIN-004
  - REQ-EXECUTORS-PLUGIN-005
  - REQ-EXECUTORS-PLUGIN-006
  - REQ-EXECUTORS-PLUGIN-007
system_design:
  - ../../specs/executors/system-design/remote-executor-plugins.md
legacy_specs: []
---

# Implementation plan: Remote executor plugins

## Overview

Add a supported plugin extension for remote environments running Kandev agentctl.
Keep provider operations in the plugin and session behavior in core.
Deliver contract and transport foundations before allocation, recovery, administration, and UI.
Prove the integrated contract with a packaged fixture provider.

The ten work orders are implemented sequentially, with results recorded in each packet. The production
Lambda plugin and built-in executor migration remain out of scope.

## Scope

### In scope

- Manifest-owned provider identities, optional SDK interfaces, gRPC contracts and capability enforcement.
- Provider profile schemas, secret references, availability, and normal executor selection.
- Direct authenticated HTTPS/WebSocket transport shared by agentctl and gateway proxies.
- Runtime artifact delivery, bootstrap, isolated session environments, and reverse API reachability checks.
- Provisional inventory, idempotent launch, exact attachment, cleanup fencing and plugin lifecycle guards.
- Explicit retention, expiry, desktop/mobile flows, and provider conformance evidence.
- Restart-required release toggle, default off in every shipped profile.

### Out of scope

- The production Lambda plugin, built-in executor migration, or cloud account provisioning.
- Shared workspace support for plugin providers, custom provider form JavaScript, and local process sandboxes.
- Workspace backup/migration, durable event replay, automatic compute replacement, and a new NAT relay.
- Billing UI, image builders, marketplace publication, or a force-forget resource API.

## Assumption check and source evidence

Confirmed: the user requested implementation of the reviewed remote executor plugin package.
The accepted contract uses session-isolated v1, schema-driven profiles, guarded uninstall, and explicit
bounded-workspace support. The ADR records alternatives and the scope remains generic core support plus
a fixture provider; it does not add a production cloud plugin.

Verified source anchors: `ExecutorBackend`, `ExecutorRegistry`, `ExecutorProfile.Config`,
`ExecutorRunning.Metadata`, SDK optional interfaces, manifest provider declarations, and plugin lifecycle locks.
Both existing executor-to-runtime maps default unknown types to standalone and need explicit safe routing.
Editor/preview gateways currently rebuild their own transports and must join the connection lease path.

The contributor's mentioned design package is not linked in issue 3953 and was not reviewed.
The earlier assessment read all three issue comments and AWS references. Live AWS validation is excluded.
The provider extension does not depend on the proposed generic Host v2 or local sandbox-provider work.

## Technical approach

- Task 01 expands `plugin.proto`, `pkg/pluginsdk`, manifest validation and feature flag contracts.
- Task 02 adds endpoint/lease transport to `internal/agent/runtime/agentctl` and gateway proxies.
- Task 03 adds `plugin_remote`, provider catalog/profile validation, DTOs and authoritative admission.
- Task 04 implements the lifecycle adapter, artifact callbacks and CAS inventory without a new inventory table.
- Task 05 connects exact recovery and ownership-fenced cleanup to current runtime/task services.
- Task 06 guards plugin disable/uninstall/upgrade against retained inventory and active operations.
- Tasks 07 and 08 deliver native profile selection and lifetime/status presentation with mobile parity.
- Task 09 proves managed-plugin interoperability and updates author/operator documentation.

The RPCs and wire fields are defined in the system design. Do not invent provider-specific
branches in task orchestration, a second ACP protocol, or a second plugin-owned runtime inventory.
The first schema supports scalar fields and vault-backed secret references, preserving map[string]string storage.
No schema migration is planned. Any newly required DDL changes must return to design before implementation.

## Dependency order

Executed 01, 02, 03, 04, 05, 06, 07, 08, 09, then 10 sequentially.
The actual dependencies are in each work order. Separate waves do not authorize delegation.
Task 07 owns the profile UI; Task 08 owns status and expiry UI. Both share the same domain projection.

## ASCII UI preview

Control order, state visibility, phone composition, scrolling, and action reachability are structural requirements.
ASCII spacing, provider names, and example expiry values are illustrative. Copy must be localized.

### UI-01: Provider profile settings

Entry: Settings > Executors > provider > profile. State: editable bounded profile.

```text
Desktop
Executors / Provider / Profile
[Name                      ]
[Image                     ]  [Region                ]
[Credential: configured    ]  [Replace] [Clear]
Workspace ends with compute. Maximum lifetime: 8 hours.
[field validation error, when present]
                                           [Save]

Phone: dedicated settings page
< Executors       Profile
Name
[                       ]
Image
[                       ]
Region
[                       ]
Credential: configured
[Replace] [Clear]
Workspace ends with compute.
Maximum lifetime: 8 hours.
-------------------------
[          Save         ]
```

The phone page body scrolls; the Save region clears the safe area and keyboard.
The desktop layout may group short fields and uses standard 28px profile controls; the phone form has one column and controls of at least 44px.
The 8-hour value is fixture data, not a host constant. Save errors preserve edits.
Provider absent: keep fields and selection visible, disable Save, and show a reason.
Loading: show status without replacing saved values. No providers: link to plugin settings.
Host strings use translations; provider strings use the plugin localization contract.
Covers AC-EXECUTORS-PLUGIN-001.1 through 001.3 and 001.7, plus AC-EXECUTORS-PLUGIN-007.1 through 007.6.

### UI-02: Executor selection

Entry: task creation or existing executor profile selector. State: available and unavailable choices.

```text
Desktop: existing picker
Executor [Build profile v]
  Build profile       Ready
    Workspace ends with compute; maximum 8 hours
  Previous profile    Provider unavailable (disabled)

Phone: temporary inset picker drawer
+-----------------------------+
| Choose executor       Close |
| [Search                   ] |
| Build profile               |
| Workspace ends with compute |
| Maximum 8 hours             |
| Previous profile            |
| Provider unavailable        |
+-----------------------------+
```

Use the current selector's shared model. Unavailable choices are identifiable but cannot launch.
An already selected unavailable profile remains selected until the user chooses another.
The drawer body scrolls independently; header and safe-area padding stay fixed.
Covers AC-EXECUTORS-PLUGIN-001.1, 001.3, 002.2 and 007.1 through 007.5.

### UI-03: Environment and plugin lifecycle state

Entry: task environment control; plugin settings for disable/uninstall outcomes.

```text
Desktop: environment disclosure
Provider: Example remote       State: Running
Workspace: ends with compute
Expires: Sep 26, 18:00          [Executor settings]

Phone: environment button opens touch drawer
+-----------------------------+
| Environment           Close |
| Example remote              |
| Running                     |
| Workspace ends with compute |
| Expires Sep 26, 18:00        |
| [Executor settings]         |
+-----------------------------+

Changed states, both presentations
Expired: Workspace is no longer available. [Reset environment]
Unreachable: Could not confirm environment state. [Refresh]
Cleanup pending: Resource removal is unconfirmed. [Retry cleanup]
Plugin disabled: Remote resources may still be running. [Plugin settings]
Uninstall blocked: Clean up retained resources before uninstalling.
```

Reuse existing authorized reset/cleanup actions. Do not create a force-forget control.
Show retry only for an authorized pending cleanup; it repeats that admitted operation.
Status data is shared between desktop popover and touch drawer. The drawer owns scrolling.
Exact expiry, retention consequence, and available actions remain visible without hover.
Covers AC-EXECUTORS-PLUGIN-005.1 through 005.4, 006.1 through 006.4 and 007.2 through 007.5.

## Tests

The following acceptance mapping identifies the evidence recorded in each work order.
Use the exact commands and results in those packets to assess coverage.

| Criteria | Required evidence | Work order |
| --- | --- | --- |
| 001.1-001.3, 001.7 | TestPluginExecutorProfileSecrets, Catalog, AdmissionPaths, NoLocalFallback; profile tests and E2E | 03, 07, 10 |
| 001.6 | Provider state required/type/enum/bounds/closed-schema tests; manifest schema tests | 09, 10 |
| 001.4-001.5 | TestPluginExecutorWireContract, OldPluginCompatibility, Admission, DisabledNoDispatch, FlagOffRetention | 01, 06, 09 |
| 002.1 | TestPluginExecutorPartialLaunch, provider operation recovery, and retained cleanup inventory | 04, 05, 09 |
| 002.2 | TestPluginExecutorAdmissionPaths; incompatible profile UI | 03, 07 |
| 002.3-002.4 | TestPluginExecutorLaunch, BootstrapSecrets, and host agentctl/gateway transport coverage | 04, 02, 09 |
| 003.1-003.4 | TestPluginExecutorTransportMatrix, LeaseRefresh, EndpointRejection, ProxyHTTPAndUpgrade, ProxyGeneration | 02 |
| 004.1-004.5 | TestPluginExecutorRestartRecovery, UnknownOperation, StopMatrix, OwnershipFence, missing-handle cleanup and reset-scope tests; PostgreSQL CAS/claim tests | 04, 05, 10 |
| 005.1-005.4 | TestPluginExecutorDisableRetention, UninstallRace, UpgradeCompatibility; lifecycle status E2E | 06, 08 |
| 006.1-006.4 | TestPluginExecutorExpiry, UnknownRetention, bounded expiry validation; desktop/mobile status E2E | 08, 10 |
| 007.1-007.6 | Profile component tests, status projection tests, desktop/mobile E2E, measured control sizes, and i18n gates | 07, 08, 10 |
| 001.2, 003.1, 004.1, 004.3-004.4 | Review regressions for bootstrap auth, retained secrets, exact reopen, partial bootstrap, and revision-fenced inventory | 10 |

All short criterion numbers above use the prefix `AC-EXECUTORS-PLUGIN-`.
PostgreSQL checks require an isolated KANDEV_TEST_POSTGRES_DSN and may not be recorded as passing when skipped.
Use failpoints with synchronization barriers rather than sleeps for crash windows and race tests.
Check secret absence in stored inventory, browser DTOs and captured logs, not only redaction helpers.

## E2E tests

Use the managed runner, which rebuilds production assets and the backend. Run desktop and phone separately.
Use the packaged fixture with explicit feature enablement; no external cloud account is needed.
Profile and status E2Es cover desktop and phone UI. Task 09 covers provider gRPC/HTTPS conformance and
the feature-off browser flow; focused agentctl and gateway tests cover normal host-owned transports.

| File under apps/web/e2e/tests | Project | User outcome and criteria |
| --- | --- | --- |
| settings/plugin-executor-profiles.spec.ts | chromium | Create, edit, save/reload, select; 001.1-001.3, 002.2, 007.1-007.5 |
| settings/mobile-plugin-executor-profiles.spec.ts | mobile-chrome | Touch form and picker parity, long fields and validation; same criteria |
| session/plugin-executor-status.spec.ts | chromium | Idle expiry, unavailable provider, cleanup retry, blocked uninstall; 005.1-005.4, 006.1-006.4, 007.2-007.5 |
| session/mobile-plugin-executor-status.spec.ts | mobile-chrome | Real touch drawer and action parity; same criteria |
| plugins/remote-executor-plugin.spec.ts | chromium | Packaged provider retention disclosure and feature-off admission; provider RPC/HTTPS recovery is verified by the Go conformance test |

Phone tests use the configured Pixel 5 project and .tap(). Include 767/768px composition checks,
44px measured targets, safe-area containment, focus return and zero document horizontal overflow.
Inspect the rendered phone surface against the preview; record discrepancies or screenshots in task results.

## Work orders

- [x] [Task 01: Declare and gate remote executor providers](task-01-provider-contract.md)
- [x] [Task 02: Share remote authentication across clients and proxies](task-02-authenticated-transport.md)
- [x] [Task 03: Expose provider profiles and enforce remote admission](task-03-profile-admission.md)
- [x] [Task 04: Provision and bootstrap one session environment](task-04-provision-bootstrap.md)
- [x] [Task 05: Recover exact resources and fence cleanup](task-05-recovery-cleanup.md)
- [x] [Task 06: Protect resources during plugin lifecycle changes](task-06-plugin-administration.md)
- [x] [Task 07: Configure and select providers on desktop and phone](task-07-profile-ui.md)
- [x] [Task 08: Expose retention, expiry and cleanup outcomes](task-08-retention-status.md)
- [x] [Task 09: Prove packaged provider behavior and document the API](task-09-provider-conformance.md)
- [x] [Task 10: Close review findings for provider lifecycle and contract validation](task-10-review-remediations.md)

## Verification results

Design-package validation passed on 2026-09-26:

- `python3 scripts/list-docs.py validate`: 307 decisions and 1159 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Local traceability check: ten work orders cover all seven requirements and all acceptance criteria; local links resolve.
- Whitespace checks include newly created, untracked files; `git diff --check` passes for tracked changes.
- Catalog queries discover the requirement/design pair and accepted ADR.

Implementation validation is recorded in Tasks 01 through 09 and in the system design's
[implementation evidence](../../specs/executors/system-design/remote-executor-plugins.md#implementation-evidence).
The packaged Go conformance test covers provider RPCs, HTTPS lease rotation/revocation, provider-process
restart, exact resource recovery/attachment, and cleanup. Managed browser tests cover packaged provider
profile/status UI and feature-off admission. Focused client, lifecycle, and gateway tests cover session
transport and cleanup paths. The browser suite does not exercise every normal agent action through a live
remote fixture. No AWS or production-provider validation was performed.

Tasks 01 through 09 are done. Their recorded checks include successful PostgreSQL concurrency coverage
for inventory and ownership fencing. Cloud-specific behavior remains unverified and is outside this plan.

Task 10 closed the five initial and eight follow-up PR findings. Bootstrap credentials now authenticate WebSocket clients; rotated
profile credentials remain available to retained environments, and clear/delete is blocked while inventory
references the profile. Normal stopped-session reopen attaches to its exact retained resource. Every
durable partial-bootstrap phase has an explicit attach or cleanup outcome. Inventory checkpoints compare
the caller-observed envelope revision, validate phase transitions, and merge only the plugin envelope.
Cleanup recovers missing handles by the original operation ID, provider state and expiry are validated
before persistence, task reset checks authorization first, and desktop/phone controls meet their size
contracts. Exact post-fixup test, build, lint and CI results are recorded in
[Task 10](task-10-review-remediations.md).

The CI follow-up also synchronized the packaged and web E2E fixture bundles and replaced repeated
manifest schema strings with named constants. Plugin packaging, the CI-equivalent Go lint, and the
managed desktop profile E2E passed locally. The code-fixup head `67511e8bcc9be52330e37c9e46dbba3a1d60e3f5`
passed its terminal PR run with 60 passed, 10 skipped, and no failed or pending checks. The documentation
coverage publisher passed after a retry following GitHub API HTTP 429; the Kandev task plan records the
final result for the subsequent documentation-only update.

The final pre-push PR checks also exposed a nondeterministic workflow-step test helper and two E2E
fixtures that depended on stale or implicit executor-profile selection. Task 10 records the corrections
and exact local reruns: the workflow helper passes 20 race-enabled repetitions, both exact E2Es pass with
retries disabled, changed-code Go lint reports zero issues, the backend builds, and the E2E files pass
Prettier and ESLint. The updated-head CI run remains pending.

## Risks

- Cross-process allocation cannot be made atomic with SQLite/PostgreSQL. Provider operation recovery is mandatory.
- JSON metadata avoids DDL and uses typed validation, narrow CAS writes, and PostgreSQL concurrency checks recorded in Tasks 04 and 05.
- Provider disable and feature disable can prevent cleanup until re-enabled; inventory and credentials must survive.
- TLS and WebSocket proxy tests must cover the gateway path, not just agentctl client methods.
- Remote-to-host API reachability is an explicit deployment prerequisite; localhost-only installations are not automatically supported.
- Session-isolated v1 rejects shared workspaces and must not corrupt existing task-environment ownership.
- A fixed expiry can destroy uncommitted or unpushed files. This plan discloses loss and does not implement backup.
- Compatible plugin versions must continue to read retained state; upgrades cannot erase that obligation.
- Lambda hook/image/network behavior still needs a separate provider implementation and live verification.

## Public documentation impact

Task 09 updates executor and plugin authoring guides, the manifest reference, the SDK protocol reference,
and scoped engineering guidance. The docs distinguish the implemented generic provider contract and
fixture evidence from production cloud behavior. No production cloud example or provider claim is made.
