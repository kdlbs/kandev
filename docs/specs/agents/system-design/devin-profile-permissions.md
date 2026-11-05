---
status: draft
system: agents
created: 2026-11-03
requirements:
  - REQ-AGENTS-DEVIN-PERMISSIONS-001
---

# Devin profile permissions system design

## Purpose and boundaries

Use existing profile modes and environment entries to configure Devin.
Do not add a persisted permission-mode field or reuse Kandev auto-approval as a provider mode.
The agent system owns this vertical contract, including its desktop and phone controls.

This design extends the existing [profile discovery contract](profile-capability-discovery.md).
Tasks 01/02 of the [combined Devin plan](../../../plans/first-draft-profile-discovery/plan.md) remove the missing-parent dependency before browser acceptance.
That plan also delivers grouped/composed models under a [separate catalog design](grouped-model-catalog.md).
Mode and sandbox controls remain outside model dimensions and Thinking/Speed options.

## Requirement mapping

| Acceptance criteria | Design section |
| --- | --- |
| 001.1, 001.2, 001.3 | Mode authority and defaults |
| 001.4, 001.5 | Sandbox override and execution |
| 001.6, 001.9 | Persistence and compatibility |
| 001.7 | Approval authority |
| 001.8 | Editor and mobile composition |

All entries refer to REQ-AGENTS-DEVIN-PERMISSIONS-001.

## Verified provider boundary

Read the official [permissions](https://docs.devin.ai/cli/reference/permissions),
[commands](https://docs.devin.ai/cli/reference/commands), and
[sandbox](https://docs.devin.ai/cli/sandbox) references.
The current public installer manifest identified CLI version `3000.11.234` on 2026-11-03.
Investigation downloaded its Linux binary to a disposable directory and verified the manifest checksum before execution.
No global installation, authentication, or model prompt occurred.

Observed parser and handshake behavior:

- `devin acp --permission-mode smart --help` exits 2 with an unexpected-argument error.
- Root permission flags before `acp` parse, but the observed ACP sessions still start in `accept-edits`.
- `DEVIN_PERMISSION_MODE=smart` also did not establish Smart as the ACP session's current mode in those probes.
- Some fresh handshakes advertise Smart, while others omit it. The documentation describes a gradual Smart rollout.
- An advertised Bypass selection succeeds through `session/set_config_option` and returns `currentValue=bypass`.
- An unadvertised Smart selection returns `-32602`, with `Mode 'smart' is not available`.
- `DEVIN_SANDBOX=true` reaches ACP initialization. The observed sandbox session fails because `socat` is absent.

These results establish argument placement and explicit capability validation.
They do not establish authenticated Smart judgment, successful sandbox execution, or complete OS/network isolation.
Keep those limits distinct from mocked integration evidence.

Kandev already uses `https://cli.devin.ai/install.sh` in `DevinACP.InstallScript()`.
Retain its checked download, PATH setup, and installed-binary validation.
No installer implementation change is required.

## Mode authority and defaults

Store ACP mode intent in `AgentProfile.Mode` and `ProfileFormData.mode`.
Reuse the current Start mode picker and generic ACP mode/config-option application.
Do not add Smart or Bypass to `PermissionSettings()` as independent boolean CLI flags.
Do not add default `--permission-mode` tokens to `cli_flags` or assume a root flag establishes an ACP mode.

For a genuinely new Devin profile:

- Use `smart` when the creation request has no explicit non-empty mode.
- For create requests, omitted, null, and empty `mode` all request this new-profile default.
- Preserve an explicit non-empty mode. A request with an enabled sandbox and no explicit mode uses `autonomous`.
- Add `DEVIN_SANDBOX=false` only when creation inputs contain no sandbox override.
- Retain the existing false default for `auto_approve`. Honor an explicit true choice.

Apply these defaults to UI drafts, first provisioning, and the create-profile controller.
Use a narrow helper in `internal/agent/settings/profileconfig` for backend defaults.
Mirror its cases in a pure frontend helper rather than changing general permission metadata.
No defaults run during PATCH, DTO projection, duplication, or startup reconciliation of an existing profile.

Discovery remains the authority for selectable modes.
Retain a desired or saved Smart value when the current provider does not advertise it.
Show localized unavailability and require an explicit supported choice for a new draft.
Do not relabel Code/Accept Edits as Smart or retry inference in another mode.
Retain saved-profile editing and failure semantics under the existing discovery contract.

## Sandbox override and execution

Represent the override with the existing `env_vars` entry `DEVIN_SANDBOX`:

| Editor choice | Stored entry | Mode effect |
| --- | --- | --- |
| Use CLI settings | No profile entry | Preserve explicit mode and discover the effective provider context |
| Disabled | Literal `false` | Offer discovered non-sandbox modes |
| Enabled | Literal `true` | Request `autonomous` and refresh the same complete draft |

Use a three-choice control because absence and explicit false have different meanings.
A boolean switch cannot accurately show an existing inherited setting.
Preserve unrelated environment entries, CLI flags, secret references, and command prefixes.
Do not treat a secret-backed or non-boolean custom sandbox value as a known disabled state.
Show custom configuration and leave it unchanged until the user explicitly replaces it.

When the user enables sandbox, update the override and mode together.
Mark discovery stale through the existing launch-setting path.
After refresh, require the sandbox context to advertise and accept Autonomous.
If it does not, keep the requested state and show the existing unsupported/failure path.
The UI cannot leave an active Smart or Bypass selection beside an enabled override.

When the user disables sandbox, remove an Autonomous-only mode selection and ask for a non-sandbox mode.
Prefer Smart only when the current matching discovery advertises it.
Do not restore an unknown old mode after reload or invent another persisted preference.

Validate explicitly changed mode/override combinations in create, update, and contextual probe resolution.
An explicit true override with Smart or Bypass is invalid before provider execution.
An inherited setting remains provider-owned and can fail when its effective policy conflicts with the requested mode.
Do not reinterpret unrelated edits to legacy custom settings as a request to normalize them.

The resolved profile environment already reaches host utility, task launch, and CLI passthrough.
Reuse that path for `DEVIN_SANDBOX`.
Keep `ACP_BACKEND` stripping, credential selection, managed launch identity, and HMAC context isolation intact.
Remote execution evaluates prerequisites on the actual executor, independently of host discovery.

The current documentation requires `bwrap` and `socat` on Linux, uses Seatbelt on macOS, and does not support native Windows sandbox execution.
Show the provider's sanitized startup failure and preserve retry/edit actions.
Never unset the sandbox override and retry automatically.
Do not promise network isolation without Devin's separately configured domain policy.

CLI passthrough does not use ACP mode application.
Until a separately verified terminal-mode mapping exists, identify the curated mode control as ACP-only and retain native terminal settings.
The sandbox environment override still applies to the terminal child.
Do not display a terminal Smart guarantee based on a saved ACP mode.

## Approval authority

Keep `CatalogPermissionSettings` and the universal `auto_approve` control independent.
Its existing default is false for Devin.
Add contextual localized help that explains the effect of an explicit auto-approval choice.
Smart evaluates provider actions, while Kandev auto-approval answers the permission requests which still reach the host.
Autonomous can also emit direct edit/write permission requests.
Neither control overrides Devin organization policy.

## Editor and mobile composition

Keep model and Start mode controls in the existing capabilities row.
Place the sandbox override in the profile's extra settings, alongside its permission controls.
Reuse `ProfileFormFields`, `ProfileCapabilitiesSection`, and a small `DevinSandboxPreference` component.
Use a pure `lib/devin-profile-settings.ts` helper for state derivation and atomic edits.
Do not add a second mode picker or independent Smart/Bypass switches.

On phones, keep direct settings navigation and the existing page scroll owner.
Stack the sandbox label, short explanation, and selector so the field fits the viewport.
Reuse shared Select controls, with 28px fine-pointer desktop and at least 44px phone/coarse-pointer hit targets.
The nearby phone profile configuration spec is the interaction exemplar.
Preserve keyboard operation, accessible field labels, error announcements, and focus return.

New explanatory copy uses `t()` in all seven locales.
Provider-reported mode names remain provider data.
Retain the existing literal-code representation for environment names in advanced settings.

## Persistence and compatibility

Use the existing `mode`, `env_vars`, and `auto_approve` fields. No database migration is required.
Defaults affect only new records and new drafts.
An existing empty mode remains empty, and an existing absent sandbox override remains inherited.
Duplication copies those values exactly without applying new defaults.
An explicit PATCH clear retains its existing meaning.

Existing profile discovery continues to accept full draft snapshots without saving them.
No profile UI control writes Devin's on-disk configuration.
The requirement and design retain the default and compatibility rationale, so this local provider extension needs no separate ADR.

## Delivery and documentation

- [Requirements](../requirements/devin-profile-permissions.md)
- [Plan and work orders](../../../plans/devin-profile-permissions/plan.md)
- Update the Devin section in `docs/public/agents-and-profiles.md` during implementation.
- Document the official curl installer, Smart availability, sandbox prerequisites, independent auto-approval, and ACP/terminal distinction.
