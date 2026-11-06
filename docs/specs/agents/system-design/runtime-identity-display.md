---
status: current
system: agents
requirements:
  - REQ-AGENTS-RUNTIME-IDENTITY-001
---

# Agent runtime identity display design

## Boundary and requirement mapping

Agents owns provider runtime identity and the settings projection. This presentation change
uses existing metadata without introducing storage, launch policy, or an independent UI contract.

| Requirement | Design sections |
| --- | --- |
| REQ-AGENTS-RUNTIME-IDENTITY-001 | Identity projection; Card and diagnostics; Verification |

Related contracts: [runtime updates](runtime-updates-01.md),
[runtime ownership](runtime-update-notifications.md), and
[host CLI discovery](host-cli-model-discovery.md).

## Existing evidence

`CopilotACP.IsInstalled` discovers `copilot` on PATH; `CopilotACP.BuildCommand`
uses `ManagedNPMRuntime().ACPCommand`. Detection and execution are different facts.
`AvailableAgentDTO.RuntimeUpdate` contains managed package and effective version;
`AgentUpdateStatus` also carries runtime owner, mechanism, management, and selected source.
`InstalledAgentCard` already receives both projections but `InstalledAgentIdentity`
currently renders `matched_path` directly and uses `presentHostCLIVersion` for the next line.

## Identity projection

Add a small pure presentation reducer in `apps/web/lib/agent-runtime-identity.ts`.
It consumes the card's runtime metadata and returns structural variants for managed npm,
native, harness-owned, custom, and unknown. Return data and discriminants; translate during render.
Do not branch on Copilot's provider ID, PATH contents, translated strings, or the mere presence
of an npm package. Update ownership describes the selected structured runtime through
`RuntimeUpdateCapabilities`, rather than the installer of a detected authentication CLI.

Use current `runtimeUpdateStatus` when available: `owner=kandev`,
`mechanism=npm_candidate`, and `management=managed` identify managed npm; use its package
and effective version. `source=native` or native/manual ownership identifies native selection;
`update_mode=self_update` identifies harness-owned updating, not managed npm ownership.
For external/custom/unsupported entries, keep those distinct rather than fabricating managed identity.
An OpenCode native source takes priority over npm fallback metadata. Never use `managed_fallback`
on the status as a general native detector: its value can depend on validated fallback activation.

While the status is absent, a supported pinned `runtimeUpdate` with no managed fallback can
identify managed npm only where selection is unambiguous. OpenCode's selected source is not
fully expressed by that DTO, so await its status instead. Any ambiguous or unrecognized
combination returns unknown. A known runtime with no effective version shows unknown version.
Never substitute `current_version`, host `cli_version`, latest release, or an observed fallback
version for the effective managed selection. Status read failure preserves diagnostics access.

## Card and diagnostics

Pass runtime metadata into `InstalledAgentIdentity`. Render the session runtime summary below
the agent heading, independently of collapsed profile state. For managed npm, use two lines:
`Session runtime: Managed by Kandev` and `package · effective version`.
Provide a short localized indication that the selection is for new sessions; avoid a health claim.
Native and harness-owned variants name their source without claiming an exact executable path
for a running process. Unknown has an explicit localized label.

Move the existing detected path and host CLI version presentation into one initially closed
semantic `details`/`summary` region labelled `Diagnostics`. Preserve existing version error
handling; render paths as plain text with wrapping rather than relying on a title attribute.
No new network request, mutation, shell command, or polling loop is needed for expanding it.

Desktop keeps actions beside the identity; phone places actions below a full-width identity. Reuse the current agent settings
card as the mobile exemplar and the mobile runtime settings tests as the route precedent.
The page keeps its existing scroll owner; no overlay or new navigation is introduced.
The summary is 28px on desktop and at least 44px on phones/coarse pointers. Long technical
values wrap inside a min-width-zero container. Keep essential identity visible without disclosure.
Add English, pt-pt, zh-cn, ja, ko, generated zh-tw/zh-hk, and pseudo copy via existing tooling.

## Verification

Reducer tests cover Copilot with a detected native binary, native with managed fallback,
harness-owned updates, custom, unknown/missing data, and differing selected/observed versions.
Existing component tests retain local version/error coverage. Browser tests prove diagnostics
start closed, expand by keyboard or touch, and contain the former path/version content. Desktop and mobile Playwright tests render the real settings card with
controlled API fixtures, assert managed identity and hidden diagnostics, expand the disclosure,
and check a long path without horizontal page overflow. Record screenshots of both states.
No production or permanent test changes occur in the design turn.
