# ADR-2026-11-05-copilot-plugin-contract-admission: Admit Copilot plugins through observed contracts

**Status:** accepted
**Date:** 2026-11-05
**Area:** backend, frontend, protocol
**Amends:** [Native policy authority](2026-10-28-copilot-native-policy-authority.md), only its exact-version admission restriction.

## Context

The operator can select a Copilot runtime that passes ACP validation while
Kandev rejects its saved plugins before the process starts. Exact-version
qualification requires another source change for each compatible release.
A later platform map also restricts 1.0.105 to Linux x64, preserving the Mac failure.

The user explicitly confirmed capability and schema checks after
[the issue comment](https://github.com/kdlbs/kandev/issues/5064#issuecomment-6660108800)
changed the earlier request to retain exact-version qualification.
This changes plugin admission, not managed runtime selection or native policy ownership.

## Decision

Plugin admission uses bounded observations from the exact selected runtime,
strict inventory/installation adapters, and verification of every selected root.
A new version string or an unrecorded OS/architecture tuple does not independently reject a launch.
Native capability failures or incompatible formats still reject preparation with a specific safe reason.

Keep the managed package, exact selected version, default generation, final
profile environment, executor restrictions, identity checks, and native policy authority.
Do not guess metadata layouts, drop selected plugins, switch executables, or add bypass permissions.
Platform information records actual observations; it is not a certification allowlist.

Preview separates unchecked compatibility from a demonstrated failure.
Ordinary ACP readiness does not establish plugin capability.
Typed plugin failures survive bootstrap wrapping into durable task/session feedback.
Only allowlisted reasons and bounded safe fields reach the browser.

## Consequences

Compatible future releases can work without another version constant.
Unknown shapes still require an explicit adapter change.
The plugin path performs a bounded native capability check in addition to inventory.
No enterprise-policy fixture, model prompt, login, or native configuration mutation is required.

The original three work orders and their native evidence remain historical delivery records.
They do not complete the revised issue scope. New work orders replace their admission rule and extend failure feedback.

## Alternatives considered

- Extend exact version/platform maps: repairs one combination but repeats the failure on compatible updates.
- Permit a SemVer range: assumes compatibility without evidence from the selected runtime.
- Remove validation entirely: accepts malformed metadata and unsafe or unresolved roots.
- Change pins or fall back to a global CLI: changes the operator's executable and identity contract.

## Related design

- [Owning requirements](../specs/agents/requirements/copilot-profile-plugins.md).
- [Contract admission design](../specs/agents/system-design/copilot-plugin-contract-compatibility.md).
- [Updated delivery package](../plans/copilot-runtime-plugin-compatibility/plan.md).
