---
status: active
system: agents
created: 2026-10-27
owners:
  - kandev
---

# Copilot profile plugins requirements

## Overview

Users select installed Copilot CLI plugins for an individual Copilot profile.
New Copilot processes load those plugins through the CLI's native plugin support.
The Agents system owns this contract because it owns profile state and provider process configuration.
Cursor supplies the interaction reference. Its MCP import and credential bridge remain separate capabilities.

Discovery, profile state, settings, and supported native process loading are
implemented. Copilot owns native policy decisions; Kandev retains version,
root, executor, and profile-identity validation.
Implementation was authorized on 2026-10-27. The [system design](../system-design/copilot-profile-plugins.md),
[implementation plan](../../../plans/copilot-profile-plugins/plan.md), and
[compatibility evidence](../../../plans/copilot-profile-plugins/compatibility-evidence.md) record delivered behavior and remaining native limitations.

The [completion plan](../../../plans/copilot-profile-plugin-completion/plan.md)
addresses review findings from PR #4882 and the remaining native-loading work.
On 2026-10-28 the user confirmed that this follow-up includes native
qualification and loading, not only the test and recovery-message fixes.
The user subsequently chose native Copilot policy authority without requiring an
enterprise-policy fixture as a Kandev release gate, and prohibited administrator
or system-policy changes. The [decision](../../../decisions/2026-10-28-copilot-native-policy-authority.md)
records that assurance trade-off. Native policy and credential boundaries remain.
The same follow-up includes the reported blocked-selection warning and generic
model-probe failure: users must see the plugin restriction and recovery before
native loading itself is integrated.

## Confirmation and design boundaries

**Confirmed by the user:**

- Discover the inventory with `copilot plugin list --json`.
- Let users select plugins on individual Copilot profiles using the Cursor settings interaction pattern.
- Load whole plugins natively into ACP processes through repeated `--plugin-dir` arguments.
- Preserve global Copilot activation preferences and existing ACP MCP injection.
- Do not extract plugin MCP definitions or copy the Cursor credential bridge.

**Recommended design decisions, delivered:**

- Start with an empty selection. Add selected plugins to native loading without claiming an exclusive whitelist.
- Support the backend host's local/worktree processes first.
- Keep missing selections and fail a new process start until the user repairs or removes them.
- Cover profile-owned terminal processes, utility calls, and capability probes as well as ACP task processes.
- Support installed entries only when a qualified metadata adapter supplies a verified directory.
- Use the existing Save/Reset and external-conflict flow. Reset discards the draft.

Acceptance criteria below describe the delivered design, including these recommendations.
They do not claim that the user approved each recommendation.
Inventory compatibility remains version-bounded. Native policy evidence is
diagnostic, not a prerequisite for attempting a supported native launch.

## Terminology

- **Selection:** Stable plugin identities saved on one concrete Copilot profile.
- **Native enabled status:** The inventory command's activation field, independent of the Kandev selection.
- **Mountable plugin:** An installed plugin with a verified directory and manifest supported by the selected runtime.
- **New process:** A fresh subprocess, including a replacement or resume that starts another subprocess.
- **Retained process:** A live subprocess that Kandev reconnects to without starting another subprocess.

## Requirements

### REQ-AGENTS-COPILOT-PLUGINS-001: Profile inventory

**Intent:** A settings manager can discover plugins under the profile's host launch identity.

#### Acceptance criteria

- **AC-AGENTS-COPILOT-PLUGINS-001.1:** Inventory shall come from `copilot plugin list --json` under the selected Copilot runtime and authorized profile environment.
- **AC-AGENTS-COPILOT-PLUGINS-001.2:** A native `enabled: false` entry shall remain visible and selectable when its plugin is mountable.
- **AC-AGENTS-COPILOT-PLUGINS-001.3:** Saved profiles and supported unsaved launch settings shall use their effective home, config directory, executable search path, and server-resolved secrets.
- **AC-AGENTS-COPILOT-PLUGINS-001.4:** Failed, cancelled, truncated, malformed, and unsupported inventory shall remain distinct from a successful empty inventory.
- **AC-AGENTS-COPILOT-PLUGINS-001.5:** Inventory shall expose only safe plugin metadata and bounded reason codes. Settings-management authority shall gate discovery before process execution.
- **AC-AGENTS-COPILOT-PLUGINS-001.6:** A response for an earlier profile, draft, refresh, or runtime selection shall not replace the current inventory.

### REQ-AGENTS-COPILOT-PLUGINS-002: Durable selections

**Intent:** A profile preference survives normal editing and does not depend on a plugin's current version or installation directory.

#### Acceptance criteria

- **AC-AGENTS-COPILOT-PLUGINS-002.1:** Existing and newly created profiles shall default to no additional selected plugins.
- **AC-AGENTS-COPILOT-PLUGINS-002.2:** Omitted updates shall preserve selections, including an explicit selection committed concurrently before an unrelated update. An explicit empty list shall clear selections.
- **AC-AGENTS-COPILOT-PLUGINS-002.3:** Save, reload, duplication, application restart, and database upgrades shall preserve selections. Plugin upgrades shall retain the same selection identity.
- **AC-AGENTS-COPILOT-PLUGINS-002.4:** Checkbox changes shall participate in dirty state, Reset, in-flight edit preservation, and the existing external-conflict recovery flow.
- **AC-AGENTS-COPILOT-PLUGINS-002.5:** Invalid, duplicate, or excessive identities and nonempty selections on non-Copilot or dynamic routing profiles shall fail without changing saved state.
- **AC-AGENTS-COPILOT-PLUGINS-002.6:** HTTP, authorized settings MCP operations, settings discovery, and browser editing shall expose the same preference semantics.

### REQ-AGENTS-COPILOT-PLUGINS-003: Directory and source eligibility

**Intent:** Selected identities resolve to the current installation without accepting browser-supplied directories.

#### Acceptance criteria

- **AC-AGENTS-COPILOT-PLUGINS-003.1:** Names shared by different marketplaces shall have distinct stable identities. Selection shall not store versions or absolute directories.
- **AC-AGENTS-COPILOT-PLUGINS-003.2:** A mountable installed entry shall resolve through matching native installation evidence and a matching readable manifest.
- **AC-AGENTS-COPILOT-PLUGINS-003.3:** Cached, direct, and live local sources shall be eligible only through explicitly supported metadata mappings. A live directory can be outside Copilot's home.
- **AC-AGENTS-COPILOT-PLUGINS-003.4:** Built-in entries, ambiguous installations, unsafe roots, and unsupported sources shall show an unavailable reason and prevent new selection.
- **AC-AGENTS-COPILOT-PLUGINS-003.5:** Removed or unreadable selected plugins shall remain visible and removable. A new process shall fail with a repair action instead of silently omitting them.
- **AC-AGENTS-COPILOT-PLUGINS-003.6:** Resolution shall not scan unrelated directories, guess installation layouts, download plugins, or return native configuration contents to the browser.

### REQ-AGENTS-COPILOT-PLUGINS-004: Native process loading

**Intent:** The profile's explicit plugins accompany each relevant new Copilot process.

#### Acceptance criteria

- **AC-AGENTS-COPILOT-PLUGINS-004.1:** Each selected plugin shall load through a separate native `--plugin-dir` argument pair. Spaces and shell metacharacters in directories shall remain part of one argument.
- **AC-AGENTS-COPILOT-PLUGINS-004.2:** Fresh ACP launch, workspace promotion, subprocess replacement, context-reset fallback, and new-process resume/recovery shall apply the same selection rules.
- **AC-AGENTS-COPILOT-PLUGINS-004.3:** Profile-owned CLI passthrough, utility prompts, and capability probes shall apply selections. Agent-wide boot probes shall not inherit a profile's selection.
- **AC-AGENTS-COPILOT-PLUGINS-004.4:** Saving a profile shall affect subsequent process starts. It shall neither reload nor terminate a retained process, including a reset within that process.
- **AC-AGENTS-COPILOT-PLUGINS-004.5:** Manual plugin-directory flags shall retain their existing meaning. Equal roots shall avoid an extra typed mount. Conflicting roots for the same selected plugin shall fail visibly.
- **AC-AGENTS-COPILOT-PLUGINS-004.6:** Command preview shall identify deferred plugin mounts without exposing resolved host directories or presenting placeholders as an executable command.
- **AC-AGENTS-COPILOT-PLUGINS-004.7:** Copilot shall load plugin components and use its native authentication. Existing native user MCP configuration and Kandev ACP-injected servers shall remain usable together.
- **AC-AGENTS-COPILOT-PLUGINS-004.8:** When plugin preparation rejects a replacement before process handoff, the existing live process shall remain running with its current identity and configuration.

### REQ-AGENTS-COPILOT-PLUGINS-005: Executor and compatibility boundaries

**Intent:** Unsupported execution contexts cannot silently discard the saved preference or receive host resources.

#### Acceptance criteria

- **AC-AGENTS-COPILOT-PLUGINS-005.1:** Supported local/worktree execution shall resolve plugins under the final process environment rather than an earlier ambient backend home.
- **AC-AGENTS-COPILOT-PLUGINS-005.2:** Nonempty selections on Docker, SSH, Sprites, Kubernetes, remote Docker, or unknown isolated executors shall fail before plugin loading.
- **AC-AGENTS-COPILOT-PLUGINS-005.3:** Empty selections shall preserve current executor behavior and require no new plugin command or metadata read.
- **AC-AGENTS-COPILOT-PLUGINS-005.4:** Inventory and process launch shall preserve Kandev's runtime selection. Unsupported runtimes shall report a reason without switching to a global executable or changing the runtime pin.
- **AC-AGENTS-COPILOT-PLUGINS-005.5:** Unsupported or identity-changing launch wrappers shall fail clearly for nonempty selections. They shall not trigger a default-home fallback.
- **AC-AGENTS-COPILOT-PLUGINS-005.6:** Superseded by AC-AGENTS-COPILOT-PLUGINS-005.8 and the native-policy-authority decision; this identifier is not reused. Historical criterion: mount-policy admission was scoped to the exact selected package version, operating system, and architecture. Qualification on another platform did not admit the current host.
- **AC-AGENTS-COPILOT-PLUGINS-005.7:** Legacy policy-unqualified start responses shall explain an available recovery action. When no qualified runtime exists for that legacy backend, guidance shall explain how to clear the profile selection rather than promise that switching versions will fix the failure. This compatibility behavior does not establish a policy-qualification gate for new loading.
- **AC-AGENTS-COPILOT-PLUGINS-005.8:** A runtime with a supported inventory/metadata adapter and an eligible launch context shall be able to attempt native plugin loading without an enterprise-policy fixture or separate operating-system/architecture policy certification. Unknown runtime versions and invalid launch contexts shall still fail visibly.

### REQ-AGENTS-COPILOT-PLUGINS-006: Copilot settings interaction

**Intent:** Users manage plugins through the existing profile editor on desktop and phone.

#### Acceptance criteria

- **AC-AGENTS-COPILOT-PLUGINS-006.1:** Copilot concrete profile editing shall show an inline Plugins section with checkbox rows, marketplace/version metadata, native status, and Refresh.
- **AC-AGENTS-COPILOT-PLUGINS-006.2:** Loading, empty, discovery error, unsupported runtime, unavailable entry, stale context, and missing-selection states shall explain the next action.
- **AC-AGENTS-COPILOT-PLUGINS-006.3:** Failed discovery or refresh shall preserve selections and unrelated drafts. It shall never save or clear a selection automatically.
- **AC-AGENTS-COPILOT-PLUGINS-006.4:** The section shall explain whole-plugin execution, additive loading, local/worktree scope, and application at the next process start.
- **AC-AGENTS-COPILOT-PLUGINS-006.5:** Phone users shall use the same settings route and save flow. Labels shall wrap, targets shall measure at least 44px, and the page shall have no horizontal overflow.
- **AC-AGENTS-COPILOT-PLUGINS-006.6:** Controls shall support keyboard operation, accessible names, visible unavailable reasons, and status announcements. Desktop controls shall retain standard density.
- **AC-AGENTS-COPILOT-PLUGINS-006.7:** New UI copy shall use the seven shipped language catalogs, generated Traditional Chinese, and the pseudo-locale.
- **AC-AGENTS-COPILOT-PLUGINS-006.8:** When a legacy native-plugin policy-qualification response blocks a profile capability or model-option probe, settings shall explain that the plugin selection blocked discovery and how to clear the selection, save, and refresh. The same recovery shall be visible inline on desktop and phone, without requiring logs or hover. The UI shall not attribute this preflight rejection to an unavailable model, failed authentication, or the native enabled status, and shall preserve the configured model and unrelated drafts.

### REQ-AGENTS-COPILOT-PLUGINS-007: Native authority and privacy

**Intent:** A profile controls explicit plugin loading without acquiring global configuration or credential authority.

#### Acceptance criteria

- **AC-AGENTS-COPILOT-PLUGINS-007.1:** Discovery and profile saves shall not run native plugin enable, disable, install, or update commands or edit Copilot configuration.
- **AC-AGENTS-COPILOT-PLUGINS-007.2:** Superseded by AC-AGENTS-COPILOT-PLUGINS-007.6 and the native-policy-authority decision; this identifier is not reused. Historical criterion: "Plugin loading shall preserve native MCP disables and managed prohibitions. Runtime support shall require evidence that explicit mounts respect these boundaries."
- **AC-AGENTS-COPILOT-PLUGINS-007.3:** Kandev shall not copy credentials, initiate login, reconstruct plugin MCP definitions, or add permission-bypass flags for this feature.
- **AC-AGENTS-COPILOT-PLUGINS-007.4:** Inventory, errors, logs, caches, and browser contracts shall exclude secret values, raw native output, full configuration files, hook bodies, and MCP definitions.
- **AC-AGENTS-COPILOT-PLUGINS-007.5:** UI and documentation shall describe explicit additions without promising an exclusive whitelist or guaranteed authentication.
- **AC-AGENTS-COPILOT-PLUGINS-007.6:** Copilot shall remain the authority for native MCP disables, managed prohibitions, and tool consent. Kandev shall use native plugin loading without changing those controls, adding bypass permissions, or retrying a native rejection through another runtime, transport, or identity. Missing enterprise-policy test evidence shall not independently block a supported native launch, and shall not be represented as verified enforcement.

## Out of scope

- Plugin installation, updates, marketplace management, or global activation controls.
- An explanation of Copilot's automatic managed-plugin activation differences between CLI and ACP.
- Remote plugin or credential transfer through portable configuration bundles.
- Hot reload, automatic session termination, or a new ACP MCP transport.
- Cursor preference changes or a shared generic Cursor/Copilot selector framework.
- A runtime version bump or a new release toggle.
