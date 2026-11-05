---
created: 2026-11-05
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

# Implementation plan: Copilot runtime plugin compatibility

## Overview

Repair [issue #5064](https://github.com/kdlbs/kandev/issues/5064): an operator can
activate Copilot `1.0.105`, but saved plugin selections then block new processes.
Expose compatibility before runtime approval, qualify the exact new adapter,
and show accurate profile recovery on desktop and phone.

All three work orders are implemented. Production and permanent test changes
were applied after the package's design handoff.

## Confirmed root cause and evidence

The checkout is `a8b8ac18e0f08cafc48f303eafc88ffd546f31f4`, matching the issue.
`copilotplugins/context.go` qualifies exactly `1.0.88` and `1.0.104`.
`Discover` and `ResolveLaunchSelection` reject any other version before native
inventory or root resolution. Empty selections bypass that preparation.
The managed updater ACP-probes a candidate without qualifying profile plugins.
Those are separate compatibility boundaries.

The temporary production-path diagnostic passed with these observations:
`runtime_unsupported` for discovery and nonempty launch, success for empty
launch, and zero native commands. The existing support test also passed.
See the [sole evidence ledger](../copilot-profile-plugins/compatibility-evidence.md#issue-5064-planning-investigation-2026-11-05)
for the exact command and limitations. This evidence confirms the gate; it does
not prove upstream incompatibility or qualify `1.0.105`.

`RuntimeUpdatePreviewDetails` currently has no plugin-impact projection.
`buildCopilotPluginSelectionRows` synthesizes `nativeEnabled: false` for saved
identities without inventory. `PluginMetadata` then labels those rows disabled.
The profile warning ignores the response's version and bounded failure reason.

## Requirement conformance and assumptions

Agents owns this repair because it owns profile plugin preparation and managed
runtime identity. Adjacent Platform settings parity and UI control sizing are
consumers; they do not need duplicate feature requirements.

- Existing 005.3/005.4/005.8 require empty-selection parity, exact runtime
  identity, and rejection of unknown adapters. The gate conforms to those
  criteria. It is not an accidental policy-only block.
- Existing 001.4, 006.1/006.2, and 007.4 require accurate inventory states,
  actionable settings, and safe metadata.
- New 005.9/005.10 define runtime-impact feedback without another approval
  gate. New 006.9/006.10 define version recovery and unknown native status.
- The accepted [native-policy ADR](../../decisions/2026-10-28-copilot-native-policy-authority.md)
  and [validated-runtime-selection ADR](../../decisions/2026-08-12-validated-managed-runtime-version-selection.md)
  remain unchanged. The existing design preserves enough rationale without another ADR.

The issue settles the intended behavior: qualify updates before admitting
explicit mounts, preserve selections, expose incompatibility, and avoid false
native-status claims. Native compatibility is an evidence prerequisite for
work order 02, not an unanswered product choice.

## Scope

### In scope

- One exact-version source for launch, discovery, and compatibility feedback.
- Copilot runtime preview feedback with qualified alternatives and a safe
  count of saved profiles with nonempty selections.
- Native adapter qualification for `@github/copilot@1.0.105`, followed by exact
  admission only if its observed contract passes.
- Profile warnings with current version/reason, truthful unknown native status,
  existing Save/Reset behavior, localization, and desktop/phone evidence.
- Update existing Copilot public guidance with the implementation.

### Out of scope

- Version ranges, automatic qualification, changing the shipped `1.0.88`
  default, runtime pins, automatic rollback, or automatic selection removal.
- General plugin management, remote executor plugin support, authentication
  transfer, native configuration edits, or enterprise-policy certification.
- Changes to live user data, processes, system policy, or credentials.
- Reworking model-probe diagnostics beyond the existing typed plugin reasons.

## Technical approach

### Shared compatibility and runtime preview

Follow the native-authority design's
[runtime amendment](../../specs/agents/system-design/copilot-profile-plugins-native-qualification.md#runtime-compatibility-amendment).
Add `SupportedRuntimeVersions` to `copilotplugins/context.go`, derived from the
existing map and filtered to the current host platform. Do not broaden
`RuntimeSupported` during work order 01.

Add a response-only `copilot_plugin_compatibility` object to
`AgentUpdatePreviewDTO` and `AgentUpdatePreview`. It names the resolved target,
support predicate, qualified versions, bounded reason, and nullable
`selected_profile_count`. Reuse `GetAgentByName` and `ListAgentProfiles`.
Every non-deleted concrete Copilot profile with a nonempty selection counts
once, including disabled profiles. A read error means unknown impact.

Use `Controller.previewAgentUpdate`, a focused controller helper, and
`RuntimeUpdatePreviewDetails`. The preview performs no native inventory or
profile write. Reuse `useAgentUpdateDialogState` request fencing and existing approval semantics.
Other providers and harness-owned updaters omit the projection.

### Native version qualification

Work order 02 uses a focused `copilot_native` adapter test with the exact npm
package and disposable native state. Cover installed/live metadata and repeated
mounts with safe paths. Add only `1.0.105` after evidence passes.

The planning npm lookup returned E404, but the registry later published the
exact package. The focused Linux x64 adapter test then passed, so admission was
added. A package-resolution problem must remain visible, without an unversioned
or global executable fallback. A schema mismatch returns to design before a
production adapter expansion. Existing authenticated diagnostics remain
diagnostics, not a new enterprise-policy release gate.

### Profile recovery

Expose `supported_runtime_versions` on concrete Copilot discovery responses.
Use the current response's version and reason in profile recovery. Keep safe
fallbacks for older servers, stale responses, wrappers, and legacy policy codes.
No browser compatibility map is necessary.

Use `boolean | null` for the browser row's native status. Current successful
rows retain true/false, including unavailable roots. Missing or unobserved rows
use null. Preserve selection intent and unrelated drafts through refresh/errors.

### Compatibility matrix

| Provider/context | Identity and transport | Result | Evidence/fallback |
| --- | --- | --- | --- |
| Copilot 1.0.88/1.0.104, valid host context | Exact managed package; native inventory and ACP | Keep current explicit mounts | Existing ledger and caller tests |
| Copilot 1.0.105, nonempty selection | Exact managed package; linux/amd64 host local/worktree | Admitted after focused adapter qualification | Linux x64 ledger evidence; other platforms stay unsupported |
| Any unqualified Copilot version | Exact selected package | Preview impact and block nonempty preparation | No alternate runtime or identity |
| Empty Copilot selection | Any existing executor | Preserve current launch behavior | No added inventory/root reads |
| Isolated/wrapped Copilot selection | Existing executor/wrapper identity | Preserve typed rejection | No host resource transfer |
| Retained Copilot process | Existing process identity | Preserve its mounts and lifetime | Settings changes apply to a later process |
| Other providers/self-update modes | Existing runtime contracts | Omit Copilot compatibility object | Existing preview and update behavior |

## ASCII UI preview

### UI-01: Runtime compatibility before approval

Entry: Settings > Agents > Copilot > runtime update. Current source shows the
version picker and command preview without plugin impact. Proposed unqualified
target state, illustrated with `9.9.9` so it remains valid after 1.0.105 support:

```text
Desktop dialog                 Phone bottom drawer
Update Copilot                 Update Copilot
Current -> Target 9.9.9        Target 9.9.9
[Version picker]              [Version picker]
Plugin loading unavailable    Plugin loading unavailable
2 saved profiles affected     2 saved profiles affected
Qualified: <server list>       Qualified: <wrapping list>
[Command details]             [Command details]
             [Cancel] [Update]       [Cancel] [Update]
```

Known zero still shows the version limitation. A null count shows unknown
impact. A qualified target shows preparation support without a blanket plugin
success claim. A missing object shows an unavailable-compatibility fallback for
Copilot. No additional approval or checkbox is introduced.

The existing body owns scrolling; actions remain reachable with safe-area
clearance. The phone uses the shipped update drawer, not a squeezed dialog.
Targets remain at least 44px on phones and 28px for ordinary desktop actions.
Criteria: 005.9/005.10 and 006.5/006.6/006.7.

### UI-02: Profile recovery with an unobserved saved identity

Entry: Settings > Agents > Copilot > Profile. The composition is shared:

```text
Before                         After
Plugins [Refresh]              Plugins [Refresh]
Runtime unsupported            Copilot <current version>:
Uncheck plugins and Save       plugin adapter unavailable
[x] example                    Qualified: <server list>
Native status: disabled        Select a qualified runtime, or
                               uncheck plugins and Save.
                               [x] example
                               Native status: unknown
             [Reset] [Save]                [Reset] [Save]
```

Phone labels and versions wrap on the direct profile route. The existing
settings scroller and safe-area Save/Reset remain. Selected unavailable rows
remain removable. Fresh observed disabled rows still say disabled. Stale data
requests Refresh without claiming that Refresh can qualify a version.
Criteria: 001.4/001.6, 002.4, and 006.2/006.3/006.5 through 006.10.

These views define order, placement, and status meaning. English copy and
spacing are illustrative. All final text uses translated catalogs.

## Tests

All numeric AC references use `AC-AGENTS-COPILOT-PLUGINS-`.

| Criteria | Regression evidence | Work order |
| --- | --- | --- |
| 005.4/005.8/005.9 | New provider registry test and `TestCopilotRuntimeCompatibilityPreview` in controller `runtime_copilot_compatibility_test.go` | 01 |
| 005.9/005.10, 007.4 | Mixed profiles, unknown count, read-only preview, target-change fencing, and omitted foreign-provider DTO tests | 01 |
| 001.1, 003.2/003.3, 004.1, 005.4/005.8 | `TestCopilotPluginNativeAdapterAcceptance`, exact package/native-written metadata/repeated mounts | 02 |
| 004.2/004.3/004.5/004.7, 005.3/005.8 | `TestCopilotPluginRuntime105Admission` plus selected-version caller tests in lifecycle/hostutility | 02 |
| 001.4/001.6, 002.4, 006.9/006.10 | Existing row/selector/discovery suites: absent rows, failure, stale responses, true/false controls, safe older-server fallback | 03 |
| 006.5/006.6/006.7 | Rendered desktop/phone tests and i18n gates | 01 and 03 |

The initial diagnostic is not permanent regression coverage. Each work order
requires its own causal Red before implementation and exact final Green results.

## E2E tests

| Flow | File under `apps/web/e2e/tests/settings/` | Project |
| --- | --- | --- |
| Unqualified target, mixed-profile count, target change, explicit approval | `agent-runtime-update-copilot.spec.ts` | chromium |
| Same advisory flow by tap, known/unknown impact, drawer containment | `mobile-agent-runtime-update-copilot.spec.ts` | mobile-chrome |
| Current runtime/reason, unknown row status, removal/Save/reload, known disabled control | `copilot-profile-plugins.spec.ts` | chromium |
| Same recovery, wrapping, touch targets, no horizontal overflow | `mobile-copilot-profile-plugins.spec.ts` | mobile-chrome |

Extend existing disposable fixtures and preserve their scenarios. Browser
interception proves presentation, not native adapter support. Run the two
projects sequentially through the managed runner with fresh builds.

## Work orders

- [x] [Task 01: Expose Copilot runtime plugin impact](task-01-runtime-compatibility-preview.md)
- [x] [Task 02: Qualify and admit Copilot 1.0.105](task-02-qualify-runtime-105.md)
- [x] [Task 03: Correct profile compatibility recovery](task-03-profile-recovery.md)

Default order: 01, 02, 03. Task 02 and task 03 each required 01. Native
artifacts became available, so task 02 completed rather than remaining blocked.
No delegation or parallel execution was used.

## Companion records

The original plugin package and completion package remain implemented. This
package adds a new version and recovery contract; it does not reopen historical
tasks or rewrite their test results. Update the sole evidence ledger in task 02.
The parent design links this package, and the native-authority design owns the
technical amendment. Public support guidance changes with task 03.

## Verification results

Planning diagnostic: two focused Go tests passed through a temporary overlay.
The configured npm lookup later returned `@github/copilot@1.0.105`, and the
focused Linux x64 adapter test passed exact package identity, fresh production
inventory, a native-written remote marketplace install with `source=installed`
cached metadata, a matching installed root, a `source=live` directory
marketplace control, production discovery/admission for both shapes, and two
repeated special-character mounts. Admission is platform-scoped to
`linux/amd64`; other platforms remain unsupported. Production admission was added only after
that run.
See the [evidence ledger](../copilot-profile-plugins/compatibility-evidence.md#native-adapter-qualification-for-copilot-10105-2026-11-05).

Implementation evidence:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/copilotplugins ./internal/agent/settings/controller ./internal/agent/settings/handlers -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle ./internal/agent/hostutility -run '^TestCopilotPlugin' -count=1)
(cd apps/web && pnpm exec vitest run components/settings/copilot-plugin-selection.test.ts components/settings/copilot-plugin-profile-selection.test.tsx components/settings/agent-runtime-update-copilot-compatibility.test.tsx components/settings/use-agent-update-dialog-state.test.ts hooks/domains/settings/use-copilot-plugin-discovery.test.tsx components/settings/agent-runtime-update-control.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/settings/agent-runtime-update-copilot.spec.ts tests/settings/copilot-profile-plugins.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/settings/mobile-agent-runtime-update-copilot.spec.ts tests/settings/mobile-copilot-profile-plugins.spec.ts)
bash scripts/lint-go-changed <changed Go files>
node scripts/validate-public-docs.mjs
```

The focused Go suites passed. Six frontend Vitest files passed 64 tests. The
chromium run passed 10 tests across both Copilot runtime and profile specs. The
mobile-chrome run passed 6 tests. Typecheck, i18n check/ratchet, changed-Go lint,
public-doc validation, specification validation, and all-spec lint passed.

Design-package checks passed:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/agents docs/plans/copilot-profile-plugins docs/plans/copilot-runtime-plugin-compatibility
```

The catalog accepted 431 decisions and 1920 specifications. All 36 linter
regression tests and all-spec lint passed. The design-package preflight checked
only the eight documentation paths that existed when the package was created
and returned `exempt`, with zero errors. That result does not cover the later
Go, web, locale, or E2E implementation paths. An independent traceability audit
accepted three work orders and 36 criterion references, including design
declarations, plan coverage, and dependency IDs. Nine relative links in the
four new package files, untracked-file whitespace, and conflict markers
passed.

## Risks

- Exact npm artifacts must be available before native adapter qualification.
  A GitHub release or ACP handshake alone cannot replace that evidence. The
  current 1.0.105 evidence covers linux/amd64 only; other platforms remain
  unsupported until separately qualified.
- A changed installation schema can require a bounded design amendment.
  Do not admit 1.0.105 while that contract remains unknown.
- Runtime-impact counts are read-only snapshots, not activation-time locks.
- Existing full native diagnostics require macOS arm64 and authorized native
  context. Their missing controls remain explicit and do not certify policy.
- Translation changes span all shipped locales. Both UI work orders share
  catalogs, so their default execution remains sequential.
