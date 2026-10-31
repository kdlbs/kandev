---
status: current
system: agents
created: 2026-10-28
requirements:
  - REQ-AGENTS-COPILOT-PLUGINS-001
  - REQ-AGENTS-COPILOT-PLUGINS-003
  - REQ-AGENTS-COPILOT-PLUGINS-004
  - REQ-AGENTS-COPILOT-PLUGINS-005
  - REQ-AGENTS-COPILOT-PLUGINS-006
  - REQ-AGENTS-COPILOT-PLUGINS-007
---

# Copilot native policy authority and diagnostic evidence

## Scope and ownership

This part of the [profile plugin design](copilot-profile-plugins.md) owns
admission, native authority, diagnostic evidence, and recovery. Agents retains
the profile-to-process boundary; the parent design owns persistence, source
validation, process coverage, and the existing desktop/phone editor.

The user selected [native policy delegation](../../../decisions/2026-10-28-copilot-native-policy-authority.md)
instead of requiring a privileged enterprise-policy fixture before loading.
This supersedes the earlier exact-version/platform policy-proof gate. It does
not grant policy, credential, or administrator authority.

| Requirements | Sections |
| --- | --- |
| 001, 003 | Qualification and admission; Evidence contract |
| 004 | Native authority; Process-boundary verification |
| 005 | Qualification and admission; Failure recovery |
| 006 | Actionable profile probe feedback; Discovery projection and settings |
| 007 | Native authority; Fixtures and execution |

Numbers use prefix `REQ-AGENTS-COPILOT-PLUGINS-`.

## Evidence baseline

The inventory adapter supports `1.0.88` and `1.0.104`. WO-04 removed the empty
`policyVerifiedVersions` gate and `RequireMountPolicy`. Mount preparation now
uses the same supported-version predicate as discovery, not privileged
qualification results.

The [sole evidence ledger](../../../plans/copilot-profile-plugins/compatibility-evidence.md)
preserves historical Linux observations and macOS arm64 native attempts.
The corrected 1.0.104 fixture observed real tools/call and successful responses
from plugin, native-config, and ACP-injected sentinels. Its disable case had an
allowed sibling call and no target call, but no authoritative denial was
captured. Managed policy was not exercised. None of these limitations is
silently reclassified as passed enterprise certification.

## Native authority

Selection authorizes additive native `--plugin-dir` mounts, not a policy
exception. Copilot remains responsible for its MCP disables, plugin/component
availability, managed restrictions, hooks, and tool permissions.

Kandev must not enable/disable/install/update native plugins, edit native
activation/configuration, copy credentials, initiate login, reconstruct plugin
MCP definitions, or add permission-bypass flags. Do not retry native rejection
under a different runtime, transport, home, or identity.

Inventory `enabled` is metadata, not a policy grant. A selectable plugin does
not imply every component will activate or authenticate. Do not parse GitHub
policy or mirror it into a second Kandev policy engine.

If actual native incompatibility is demonstrated, preserve the evidence and
make an explicit support/remediation decision. Lack of a privileged test
context alone is not demonstrated incompatibility.

## Qualification and admission

Keep `RuntimeSupported` and its inventory/metadata version boundary. Unknown
versions still return `runtime_unsupported`; never fall back to a global CLI or
change the managed pin.

Remove `policyVerifiedVersions` and `RequireMountPolicy` as launch prerequisites.
`MountsPermitted(version)` may remain as a compatibility projection of supported
native preparation, using the same version predicate as launch. It must not be
a second policy allowlist or a platform-certification lookup.

`ResolveLaunchSelection` retains this order:

1. Empty selection is a no-op without inventory, filesystem, or policy reads.
2. Validate Copilot provider, local/worktree executor, wrapper, and identity.
3. Require the selected inventory/metadata adapter version.
4. Resolve all selected roots and validate matching readable manifests.
5. Preserve manual argument order, deduplicate equal mounts, reject conflicts.
6. Pass separate `--plugin-dir` pairs to the selected native process.

No browser input grants admission. Environment and profile options do not
override version, root, or executor validation. Native policy remains a
subsequent native decision, not a Kandev preflight authorization result.

| Context | Intended preparation |
| --- | --- |
| Supported adapter version, valid local/worktree identity and roots | Attempt native mounts; native CLI decides policy and consent |
| Unknown adapter version | `runtime_unsupported`, no global fallback |
| Empty selection on any executor | Existing behavior, no added native work |
| Nonempty isolated executor, changing wrapper, or invalid roots | Existing typed rejection before native launch/resource handoff |
| Missing enterprise fixture or untested platform policy | No additional policy-only rejection |
| Retained process or in-process reset | Preserve original mounts; no new preparation |

This is preparation eligibility, not certification that an enterprise
configuration, authentication method, or every plugin component was tested.

## Evidence contract

Native evidence remains useful diagnostics. Record exact package/binary
identity, OS/architecture, fixture revision, command, actual tool traffic,
controls, errors, and limitations in the existing ledger.

Separate inventory visibility, MCP initialization, tools/list, endpoint
tools/call, successful response delivery, and optional ACP tool-update events.
An older protocol omitting tool-update events is not itself proof that an
endpoint call failed. Do not treat model claims, absence of calls, timeout, or
a handshake as proof of a veto.

| Diagnostic | Evidence |
| --- | --- |
| Repeated mounts | Separate argv pairs and two real fixture components, including a special-character path |
| Coexistence | Actual plugin, native-config, and ACP-injected MCP calls/results in one session |
| Native disable | Same otherwise-working target and allowed sibling, with observations distinguished from authoritative denial |
| Managed prohibition | Only a genuine native policy context can establish this; otherwise record not run |

The diagnostic matrix does not populate production admission. Missing or failed
enterprise certification must not be mislabeled as passed, nor become an
implicit replacement release gate.

## Fixtures and execution

Use the existing `copilot_native` build-tagged external acceptance harness.
Ordinary tests remain offline and credential-free. Native runs use temporary
plugin/MCP fixtures and structured argv under an existing authorized identity.
Do not copy an authentication store or bootstrap credentials.

The explicit full diagnostic test still fails if its required controls are
missing. That failure is a diagnostic result, not a production permission bit.
Do not skip a case and claim it passed.

Temporary user configuration is not enterprise-managed policy. No
administrator rights, real system-policy file, MDM setting, or enterprise
policy mutation is authorized. Do not keep retrying to manufacture an isolated
macOS source that the CLI does not support.

Use Copilot's actual `server-tool` selector convention, verified at the native
boundary, rather than another client's namespace. Grant allow-once only for
the exact known harmless read-only sentinel operation with empty input. Reject
unexpected requests; never add a blanket permission approver.

Own subprocesses, loopback listeners, and temporary directories through all
error paths. Bound waits/output, join goroutines, stop exact owned processes,
and keep raw captures, native configuration, and private paths outside tracked
files. Commit only sanitized observations.

## Process-boundary verification

Native diagnostics do not prove caller wiring. Hermetic tests must exercise
successful preparation at actual configure/start or inference boundaries:

- Fresh ACP, promotion, replacement, retry, and new-process resume/recovery.
- Passthrough initial, resume, fallback, and context-reset replacement.
- Profile utility prompts and capability/model-option probes.

Assert selected version, final environment/cwd, separate argv, manual-mount
deduplication/conflict handling, and intact ACP MCP attachments. Keep
empty-selection, unknown-version, isolated/wrapped, agent-wide, and retained
negative controls.

Use one authorized profile snapshot for selection, command, and environment.
A channel/barrier test proves a concurrent edit cannot mix revisions and that
the next preparation sees the updated complete snapshot. Host utility inventory
uses the selected `InteractiveCommand(version)` and final workdir.

Use a narrow internal preparation dependency only where a production-shaped
test needs it; the default remains `ResolveLaunchSelection`. Never introduce
a test-only production switch or globally enable invalid roots/versions.

## Failure recovery

Unsupported metadata versions, launch contexts, missing roots, and manual
conflicts remain actionable preflight failures. A rejected replacement must
not stop or clear the current process identity.

Preflight Copilot before local stop and before the isolated SSH branch.
Preserve Cursor/SSH remote-MCP retirement ordering and failed-attempt resource
ownership. Successful handoff still stops/starts exactly once.

Keep legacy `policy_unverified` recovery compatible, but do not produce that
error merely because enterprise-policy evidence is absent. Native runtime,
authentication, and component failures use existing sanitized provider
diagnostics rather than a synthetic Kandev policy certification failure.

## Actionable profile probe feedback

WO-02's optional response-only `reason_code` remains backward compatible.
Only typed Copilot preflight errors may project `policy_unverified` for a
selected Copilot profile with unsupported status. The capability reason lives
in `ProfileCapabilityResult`; model-option reasons live in
`ModelConfigResolution`. Settings DTOs independently allowlist projection.

Keep current status/HTTP/auth semantics and sanitized errors. Success,
agent-wide requests, foreign providers, and unrelated failures omit the field.
Unknown/absent reasons retain their fallback. Never parse raw error text or
broaden the response into provider diagnostics.

For legacy responses, render localized inline uncheck/Save/refresh recovery.
Preserve models, option values, dirty state, unrelated drafts, request fences,
and delayed-response protection. Unverified discovery is not evidence that a
saved model is unavailable. Phone recovery remains inline and wrapping, not
tooltip-only, with existing touch targets and Save/Reset.

Supported preparations now proceed to real probing with the selected plugins.
Do not silently remove plugins to make a probe pass. Actual child failures
remain owned by [probe failure diagnostics](probe-failure-diagnostics.md).

## Discovery projection and settings

`DiscoverCopilotPlugins` returns `mount_permitted` from the same supported-version
preparation predicate as launch. It is not a native policy/consent assertion.
Retain runtime/context cache identity and refresh fencing.

The existing inline Copilot editor hides the artificial policy warning for
eligible preparation. Unsupported states and unavailable entries remain
visible/removable with accurate recovery. Preserve native-enabled metadata,
deferred preview, localization, dirty state, keyboard behavior, 44px phone
targets, desktop density, the existing scroll owner, and safe-area Save/Reset.

Browser mocks prove UI behavior, not native enforcement. Public docs distinguish
supported adapter versions from platform-specific observations and explicitly
state that Copilot governs component availability and authentication.

## Delivery records

- [Original package](../../../plans/copilot-profile-plugins/plan.md).
- [Completion package](../../../plans/copilot-profile-plugin-completion/plan.md).
- [Diagnostic evidence](../../../plans/copilot-profile-plugins/compatibility-evidence.md).

WO-03 records the diagnostic investigation. WO-04 removes the artificial gate
and completes caller/preflight coverage. WO-05 verifies settings and publishes
the native-authority boundary.
