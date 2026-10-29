# ADR-2026-10-28-copilot-native-policy-authority: Delegate Copilot plugin policy to the native CLI

**Status:** accepted
**Date:** 2026-10-28
**Area:** backend

## Context

Copilot profile plugins already have inventory, directory, executor, and
launch-identity validation. A separate empty policy-verification allowlist
blocks every nonempty selection until an exact runtime/platform passes
enterprise-policy qualification.

Disposable native tests demonstrated real plugin/native-config/ACP-injected MCP
coexistence for Copilot 1.0.104 on macOS arm64. They did not establish a managed
prohibition. Copilot's documented macOS policy sources require system-owned
configuration, MDM, or enterprise-account settings, not a user temporary
directory. The user declined administrator/system-policy changes and explicitly
chose to rely on Copilot's native policy enforcement instead of requiring this
enterprise-policy test before loading.

## Decision

Copilot is the authority for its native plugin/MCP policies and tool consent.
Kandev passes selected, validated directories through native `--plugin-dir`
arguments. Missing enterprise-policy fixtures or an untested OS/architecture
tuple are not an additional Kandev policy gate.

Keep the supported inventory/metadata-version boundary, verified roots,
authorized profile snapshot, executor restrictions, and manual-argument conflict
checks. Do not copy credentials, reconstruct MCP definitions, change native
activation, or supply policy/permission-bypass flags. Native rejections are not
retried through another runtime, transport, or identity.

Native tests remain diagnostic evidence. Preserve their actual outcomes and
limitations without describing unobserved managed-policy enforcement as tested.
A demonstrated upstream incompatibility still requires an explicit support
decision; it must not be hidden by this delegation.

## Consequences

Supported profiles can attempt native loading without a privileged test fixture.
Discovery eligibility describes Kandev's preparation capability, not a native
policy grant, verified authentication, or guaranteed component activation.

The empty policy-only allowlist and its artificial preflight rejection must be
removed in the loading work order. Inventory support does not become
default-allow for unknown versions. Existing response handling for legacy
`policy_unverified` failures remains compatible.

Kandev does not independently certify every enterprise policy on every platform.
The user accepted that assurance trade-off; no native authority or privacy
boundary is relaxed.

## Alternatives Considered

- Require exact-version/platform managed-policy proof before every admission:
  prevents all loading when a privileged or enterprise test context is absent.
- Create temporary system-owned policy: declined; no administrator rights or
  real system/MDM/enterprise policy changes are authorized.
- Label a user tempdir as managed policy: invalid evidence because the CLI does
  not consume it as that policy source.
- Reimplement GitHub policy or bypass native restrictions: outside Kandev's
  ownership and not authorized.

## Related design

- [Copilot profile plugins](../specs/agents/system-design/copilot-profile-plugins.md).
- [Native policy authority and diagnostic evidence](../specs/agents/system-design/copilot-profile-plugins-native-qualification.md).
