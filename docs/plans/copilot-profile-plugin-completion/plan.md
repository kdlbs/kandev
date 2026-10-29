---
created: 2026-10-28
status: implemented
requirements:
  - REQ-AGENTS-COPILOT-PLUGINS-001
  - REQ-AGENTS-COPILOT-PLUGINS-002
  - REQ-AGENTS-COPILOT-PLUGINS-003
  - REQ-AGENTS-COPILOT-PLUGINS-004
  - REQ-AGENTS-COPILOT-PLUGINS-005
  - REQ-AGENTS-COPILOT-PLUGINS-006
  - REQ-AGENTS-COPILOT-PLUGINS-007
system_design:
  - ../../specs/agents/system-design/copilot-profile-plugins.md
  - ../../specs/agents/system-design/copilot-profile-plugins-native-qualification.md
legacy_specs: []
---

# Implementation Plan: Copilot profile plugin completion

## Overview

Complete merged PR #4882: selecting installed plugins on a Copilot profile
loads them natively into supported new processes. Portable tests, actionable
probe recovery, diagnostic investigation, and native process loading are
complete. Final desktop/phone settings and public support verification are
also complete.

The user chose
[native Copilot policy authority](../../decisions/2026-10-28-copilot-native-policy-authority.md)
instead of requiring privileged enterprise-policy qualification. No
administrator rights or system/MDM/enterprise-policy changes are authorized.
The amended package was handed off before implementation. On 2026-10-29 the
user explicitly authorized execution of WO-04 followed by WO-05, with the
primary conversation coordinating and owning delivery records.

## Baseline and confirmed root causes

- Worktree: `kandev-copilot-plugin-review-followup`, branch
  `fix/copilot-plugin-review-followup`, from main
  `fb4c2bc3ef98da2fc303fe05cb144bcd6c167d58`.
- Reviewed PR head: `7976cd6e41d012f38d6eac11687cb9e3836713df`;
  merge: `33d2f437c3e007ad1dab1d50d659b8c4d8d195bd`.
- macOS canonicalizes `/var` to `/private/var`. Three assertions compared
  canonical roots with raw temporary paths; WO-01 repaired the expectations.
- At baseline, the empty `policyVerifiedVersions` set blocked every nonempty start.
  That deliberate original certification gate is superseded by the new ADR.
  Supported inventory adapters remain exactly `1.0.88` and `1.0.104`.
- The reported generic failed model probe lost the bounded plugin-preflight
  reason. WO-02 repaired response metadata and inline recovery; no historical
  auth/model failure was established and no running instance was changed.
- Baseline caller tests mostly proved rejection. Reset preflight occurred after
  stop, host utility omitted final cwd and inventoried through the inference
  command, and late profile rereads needed coherent-snapshot coverage.
  Native 1.0.104 accepts `--acp plugin list --json`; the command discrepancy
  is not itself a demonstrated CLI parsing failure.

Agents retains ownership of profile intent/provider process configuration.
No migration or system README change is needed. The ADR records the changed
assurance decision, not a new owning system.

## Scope

### In scope

- Portable expected roots and legacy policy/probe recovery.
- Exact-package native diagnostics with actual limitations preserved.
- Remove the artificial policy-only gate while keeping version, root,
  executor, wrapper, authorization, and argument-conflict validation.
- Successful caller boundaries, coherent profile snapshots, final utility cwd,
  and Copilot preflight before replacement stop.
- Desktop/phone preparation eligibility, localization, and accurate how-to
  guidance after the loading implementation exists.

### Out of scope

- Reimplementing/certifying GitHub policy, consent bypasses, privileged setup,
  login, credential copying, and global plugin activation/management.
- Remote plugin transfer, isolated-executor support, runtime pin changes,
  automatic fallback, hot reload, Cursor refactors, or a feature toggle.
- Unrelated hardening/redesign, commits, pushes, PRs, and generic audit phases.

## Technical approach

### Completed independent repairs

WO-01 uses checked `filepath.EvalSymlinks` for three expected fixture roots,
without weakening production validation.

WO-02 adds bounded optional `reason_code` to both profile-probe responses,
allowlisted to `policy_unverified`. Internal capability metadata stays on
`ProfileCapabilityResult`, not shared `AgentCapabilities`; model options use
`ModelConfigResolution`. Generic sanitization/status behavior is unchanged.
Localized inline recovery preserves saved models/options, unrelated drafts,
and stale-response fencing. Clearing, Save, and refresh are explicit, not
automatic retries without plugins. Keep this compatibility after gate removal.

### Native diagnostics, not privileged admission

WO-03 delivered seven build-tagged `native_acceptance*_test.go` files.
Eight native attempts ran on macOS arm64, three initial plus one corrective
run per version. Corrected 1.0.104 used all four plugin/sibling/native-config/
ACP-injected MCP sentinels. Its disable case had zero target traffic and one
successful sibling call, but no authoritative denial.

1.0.88 reached all four endpoints; the then-current ACP-update assertion
stopped its paired disable case. Managed policy was not run. A user tempdir
is not a macOS managed-policy source. Preserve actual commands, nonzero
native acceptance exits, private captures, and these limitations in the
[original ledger](../copilot-profile-plugins/compatibility-evidence.md).
WO-03 is done as diagnostic delivery, not as enterprise certification.

### Atomic native-loading slice

WO-04 removes `policyVerifiedVersions` and the `RequireMountPolicy`
prerequisite in `context.go`. Discovery's `mount_permitted` and
`ResolveLaunchSelection` share supported-version preparation eligibility.
Do not replace the gate with platform certification or unknown-version
default-allow. Native policy/consent remains authoritative; no bypass flags
or retry through another runtime, transport, or identity.

Cover actual configure/start or inference payloads for fresh ACP, promotion,
replacement, retry, new-process resume/recovery, passthrough
initial/resume/fallback/reset, utility prompts, and both profile probes.
Assert selected version, separate argv, final environment/cwd, deduplication/
conflicts, and unchanged ACP MCP attachments, not merely a helper's output.

Capture one authorized profile snapshot for selection, command, and environment.
Use a deterministic edit barrier to expose mixed revisions. A later preparation
uses the new complete snapshot. Host utility inventories through
`InteractiveCommand(version)` with its real workdir.

Copilot preflight precedes `stopPassthroughForReset` and isolated SSH stop.
Rejected replacement preserves the current process/identity. Preserve
Cursor/SSH remote-MCP stop-before-retirement ordering and owned-resource cleanup.
Empty/retained/agent-wide paths do no new plugin work; isolated/wrapped/unknown
contexts still reject. Inject legacy errors only for compatibility coverage,
not by reviving global policy maps.

### Settings and public guidance

WO-05 reuses the inline picker and ordinary Save/Reset. Supported preparation
has no policy-certification warning; unsupported and legacy responses retain
clear removal/recovery. `mount_permitted` is not native policy approval,
authentication, or guaranteed component activation.

Update the how-to in `docs/public/agents-and-profiles.md` only after WO-04.
List supported adapter versions, next-process/local-worktree semantics,
native authority, and untested enterprise controls. Search README/screenshots
for conflicts without unrelated edits.

### Compatibility matrix

| Provider/context | Transport/identity | Intended behavior | Evidence | Fallback |
| --- | --- | --- | --- | --- |
| Supported Copilot adapter | Local/worktree ACP, concrete profile | Supported native preparation | WO-03 diagnostics and WO-04 caller tests | Atomic preparation error or native rejection |
| Copilot profile terminal/utility/probe | Selected runtime, same snapshot | Same mounts and context | WO-04 real-boundary tests | No default-profile fallback |
| Native disable/policy/consent | Native CLI authority | No Kandev bypass or retry | Actual diagnostic limits plus argv/no-retry tests | Native rejection |
| Unknown Copilot version | Unsupported adapter | No global CLI fallback | Provider/discovery negatives | `runtime_unsupported` |
| Nonempty isolated/wrapped context | Host profile selection | Reject before host access/stop | Admission/reset tests | Typed unsupported error |
| Empty/retained/agent-wide | Existing identity/path | No new discovery or remount | Caller spies and retained controls | Existing behavior |
| Legacy policy gate | Older response contract | Preserve bounded recovery | WO-02 tests | `policy_unverified` |
| Other agents | Existing provider configuration | Unchanged behavior | Negative controls and Cursor/SSH ordering | Existing behavior |

## ASCII UI preview

### UI-01: Native-authority profile selection

Entry: Settings > Agents > Copilot > saved profile. Desktop and phone retain
the shipped inline composition; only preparation eligibility changes warning
visibility. Copy is illustrative/localized, not a policy or authentication grant.

```text
Supported preparation              Unsupported runtime/context
+-------------------------------+  +-------------------------------+
| Plugins     [Refresh plugins] |  | Plugins     [Refresh plugins] |
| [x] fixture-plugin            |  | Plugin loading is unavailable.|
|     Native status: disabled   |  | Uncheck plugins and Save.     |
| Local/worktree. Next process. |  | [x] fixture-plugin            |
|       [Reset] [Save]          |  |       [Reset] [Save]          |
+-------------------------------+  +-------------------------------+
```

Phones keep direct profile navigation, wrapping metadata, 44px touch targets,
one settings scroller, and safe-area Save/Reset. Fine-pointer desktop keeps
28px ordinary actions. No drawer, activation checkbox, or extra scroll region.
Nearest exemplar: shipped `CopilotPluginProfileSelection` and
`mobile-copilot-profile-plugins.spec.ts`. Criteria: 002.4, 004.6, 005.8,
006.2 through 006.6, and 007.5/007.6.

### UI-02: Actionable policy and model-probe recovery

Completed WO-02 behavior, retained for legacy responses on the same route.
Not the normal supported-runtime state:

```text
Start model [saved model] [Refresh]
Discovery is blocked by selected plugins.
Uncheck plugins, Save, then refresh models.

Plugins [x] fixture-plugin
Plugin loading is blocked. Uncheck and Save.
                           [Reset] [Save]
```

Inline guidance wraps on phones; no required hover or automatic draft mutation.
Saved model/options and unrelated edits remain intact. Nearest exemplar:
existing picker and `CapabilitiesRow` status. Criteria: 002.4, 005.7,
006.2/006.3/006.5/006.6/006.8.

## Tests

Numeric criteria below use `AC-AGENTS-COPILOT-PLUGINS-`. Backend paths are
relative to `apps/backend/internal/agent/`; web paths to `apps/web/`.
WO-04's named caller-boundary tests now provide implementation evidence.

| Criteria | Test file/method | Owner |
| --- | --- | --- |
| 003.2/003.3 | `copilotplugins/copilotplugins_test.go`: `TestInstallationMetadata`, `TestPluginRootValidation`, `TestLivePluginRoot` | WO-01 |
| 005.7, 005.3, 007.4 | `copilotplugins/arguments_error_test.go::TestPolicyUnverifiedRecovery` | WO-02 |
| 004.3, 006.8, 007.4 | `hostutility/copilot_plugins_test.go::TestCopilotPluginPolicyProbeFeedback`, controller `profile_discovery_test.go::TestCopilotPluginProfileProbeFailureReason`, handler `copilot_profile_probe_reason_test.go::TestCopilotPluginProfileProbeReasonContract` | WO-02 |
| 002.4, 006.2/006.3/006.5/006.6/006.7/006.8 | Capability/model hooks and status/selection suites named in WO-02: recovery, preserved model/options, fallback, delayed rejection; i18n gates | WO-02 |
| 001.1, 003.2/003.3, 004.1/004.7, 005.4, 007.1/007.3/007.6 | `copilotplugins/native_acceptance_test.go::TestCopilotPluginNativeAcceptance` and offline `TestNative*`; partial diagnostics, not certification | WO-03 |
| 005.8, 007.6 | `copilotplugins/copilotplugins_test.go::TestCopilotPluginNativeAuthorityAdmission`, controller `copilot_plugin_discovery_test.go::TestCopilotPluginDiscoveryMountPermission` | WO-04 |
| 003.5, 004.1/004.2/004.5/004.7, 005.1 through 005.5, 007.3/007.4/007.6 | Lifecycle `copilot_plugins_launch_test.go`/`copilot_plugins_process_test.go`: `TestCopilotPluginSupportedLaunchPaths`, `TestCopilotPluginNativeInventoryFailurePreventsAlternateRuntimeOrProfileRetry`; provider `TestPluginArguments`, `TestPluginArgumentConflict`, `TestPluginReadOnlyOperations` | WO-04 |
| 004.3/004.4 | Lifecycle `copilot_plugins_passthrough_test.go` and utility `copilot_plugins_test.go::TestCopilotPluginSupportedProfileProcesses` | WO-04 |
| 004.8, 001.3 | Lifecycle launch/process and passthrough/process suites: `TestCopilotPluginResetPreflightPreservesProcess`, `TestCopilotPluginPassthroughResetPreflightPreservesProcess`, `TestCopilotPluginProfileSnapshotCoherence` | WO-04 |
| 004.4 retained control | `manager_passthrough_resume_reuse_test.go::TestPromotedPassthroughResumeAdoptsTerminalReadiness`, `TestPassthroughAdoptionPreservesGenerationZeroProjection`, `TestPassthroughAdoptionRejectsReplacedExecution` | WO-04 |
| 002.4, 004.6, 005.8, 006.2 through 006.7, 007.5 | Selection/discovery/preview suites named in WO-05, i18n and public-doc validators | WO-05 |

## E2E tests

| Flow | Playwright file/project | Criteria |
| --- | --- | --- |
| Supported selection, Save/reload, no artificial policy warning, deferred preview | `tests/settings/copilot-profile-plugins.spec.ts`, `chromium` | 002.4, 004.6, 005.8, 006.2 |
| Unsupported preparation stays removable/saveable | Same desktop file | 005.8, 006.2/006.3 |
| Same states by tap, Save/Reset/reload, wrapping and touch targets | `tests/settings/mobile-copilot-profile-plugins.spec.ts`, `mobile-chrome` | 002.4, 005.8, 006.2/006.3/006.5/006.6 |
| Legacy blocked capability/options, clear/Save/reload/refresh, preserve model | Same desktop/mobile files; WO-02 implemented | 002.4, 005.7, 006.8 |
| Real tool/coexistence and disable observations, managed policy untested | Native ACP harness, not intercepted browser discovery | 004.7, 007.6 |

Browser mocks prove settings behavior only. Run managed projects sequentially,
with fresh assets, disposable identity, and existing worker limits.

## Work orders

| Order | Work order | Actual prerequisite | Status |
| --- | --- | --- | --- |
| 1 | [Canonical expectations](task-01-canonical-path-expectations.md) | None | done |
| 2 | [Actionable probe recovery](task-02-actionable-policy-recovery.md) | None | done |
| 3 | [Native diagnostics](task-03-native-qualification.md) | None | done |
| 4 | [Native process loading](task-04-qualified-process-loading.md) | 01 and 02; no privileged fixture gate | done |
| 5 | [Settings/support guidance](task-05-settings-and-support-docs.md) | 04 | done |

All five work orders completed sequentially under the authorized execution.
WO-03's completed investigation is context, not a runtime-admission prerequisite.

## Companion package synchronization

The original [package](../copilot-profile-plugins/plan.md) preserves historical
Linux evidence. Its WO-00 closes as diagnostic delivery under the new ADR,
not as passing its former certification criterion. WO-01/02/05 stay done.
Original WO-03/04 map to completed WO-04. Original WO-06 maps to WO-02's
completed recovery update plus WO-05's completed support guidance.
Requirements are active and designs current after the complete approved
behavior and settings checks were delivered. The pre-existing SSH teardown
race exception and native diagnostic limitations remain explicit.

## Verification results

PR review remediation corrected passthrough selection refresh, replacement
wrapper admission, restart retry mounts, and first-seen diagnostic permission
requests. The [WO-04 results](task-04-qualified-process-loading.md#pr-review-remediation)
record regression evidence and final local verification. Support records now
distinguish delivered preparation from native policy and component activation.
Remote CI and review completion remain delivery gates for the pushed head.

- WO-01 reproduced all three macOS failures; focused Green, provider tests,
  changed-file lint, and whitespace passed.
- WO-02 completed four Go packages (45 named events), expanded focused web
  tests (47), typecheck with 8GB heap after default-heap OOM, lint/i18n,
  desktop 6/6, phone 3/3, and public docs 63/63 tests/54 pages.
  The missing historical Go Red was recorded honestly; separate retrospective
  baseline-overlay Red produced 11 pass/4 fail terminal events, then 15/15 Green.
  Exact commands and limitations remain in its completed work order.
- WO-03's eight native acceptance invocations exited nonzero. Its final
  tagged offline run passed 10 top-level tests and 20 subtests with zero skips;
  ordinary provider tests, three focused race tests, native-test discovery,
  exact seven-file lint, whitespace, and owned-resource cleanup passed.
  See its results and the ledger for the actual native observations.
- Earlier primary document checks passed: 413 decisions/1790 specifications,
  36 linter regression tests, all-spec lint, nine-work-order coverage with zero
  errors, and 77 then-current relative links. These are historical counts,
  not validation of the new ADR/amendment.
- WO-04 delivered native loading and real caller-boundary coverage. Provider,
  settings, and lifecycle/hostutility non-race checks passed; the final added
  native-inventory rejection/success pair passed targeted and race runs.
  The extended race command fails in existing SSH teardown, reproduced on
  pristine HEAD in all three baseline runs. Its completed Results record exact
  commands/counts, skips, lost historical Red details, canonical-path correction,
  and expanded lint inputs. No running instance, runtime pin, system policy,
  staging, commit, or push changed.
- WO-05 added only fixture/test/public-how-to changes: 67 Vitest tests across
  nine files, 16-path ESLint, final desktop 7/7 and phone 5/5 managed host runs
  with fresh builds, 63 public-doc tests, and 54 published pages passed.
  Initial browser failures and the corrected request/removed-row assertions
  remain recorded separately; no production Red is invented. The primary's
  36 linter regression tests also passed.

Final native-authority amendment checks passed:

```bash
python3 scripts/list-docs.py decisions --text copilot-native-policy-authority --format paths
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/decisions docs/plans
git status --short -- docs/specs docs/decisions docs/plans
```

The new ADR is discoverable; catalog validation accepted 414 decisions/1790
specifications, all 36 linter regression tests passed, and all-spec lint passed.
The final metadata reconciliation also passed catalog/all-spec checks.

PR-documentation `validateCoverage` from `.github/scripts/pr-docs.cjs` used
all 59 actual tracked/untracked changed paths and Markdown contents, including
owning requirements: `covered`, nine changed work orders, 13 accepted design
references, zero errors. No synthetic application trigger was used.
The companion-document scan checked 20 Markdown files, 94 relative links
including anchors, no retired criteria in active work-order frontmatter,
and tracked/untracked whitespace. All 23 existing verification inputs were
present. The parent design is 31,884 bytes, below its 32,768-byte limit.

At the amendment checkpoint, production `context.go` was identical to HEAD and
only documentation changed, so product tests were not repeated. The subsequent
authorized WO-04 implementation removed the artificial gate; its own results
above supersede that historical source-state observation. Changes remain
unstaged/uncommitted at the implementation handoff. The user's later PR
request authorizes the separate publication step.

## Final implementation reconciliation

The completed package passed catalog validation (414 decisions and 1790
specifications), all 36 specification-linter regression tests, and all-spec
lint. The actual tracked/untracked implementation diff contains 74 changed
paths. PR-documentation `validateCoverage` from
`.github/scripts/pr-docs.cjs` returned `covered`: nine changed work orders,
13 accepted design references, zero errors. No synthetic application path
was added to trigger the gate.

The companion-document check covered 20 Markdown files and 95 relative links,
including anchors. All 18 unique Vitest input paths existed; no active
work-order frontmatter references superseded criteria. The parent design is
31,908 bytes, below its 32,768-byte limit. Tracked and untracked whitespace
checks passed. The requirements/design/work-order statuses above agree with
the delivered revised contract, not enterprise-policy certification.

Final catalog/spec commands:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

The user subsequently requested a PR. A disposable capture spec used the
canonical mobile project plus the existing desktop fixture in one isolated
managed host run. It passed and produced four fresh synthetic screenshots:
supported selection and legacy blocked-probe recovery on desktop and phone.
Assets were checked, PNG-compressed, and inspected; the temporary spec was
removed. Screenshots remain ignored PR assets, not repository documentation
images. Publication does not change the running user instance.

## Risks

- Enterprise enforcement is delegated, not certified on every version/platform.
  Missing managed-policy testing remains explicit.
- Native syntax/metadata can change. Keep adapter-version bounds; never change
  the pin or fall back to a global CLI to hide incompatibility.
- A model answer or missing call is not protocol proof. Distinguish real
  endpoint responses, optional ACP updates, and missing authoritative denial.
- Reset reordering can disturb remote MCP claims; limit changes to Copilot
  preflight and preserve Cursor/SSH ordering.
- Late profile rereads can mix revisions; use deterministic caller-boundary
  coverage. Default settings/browser mocks must not hide a closed production gate.
- Install dependencies only after a targeted missing-dependency failure.
