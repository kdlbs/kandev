---
status: current
system: agents
created: 2026-11-05
requirements:
  - REQ-AGENTS-COPILOT-PLUGINS-001
  - REQ-AGENTS-COPILOT-PLUGINS-002
  - REQ-AGENTS-COPILOT-PLUGINS-003
  - REQ-AGENTS-COPILOT-PLUGINS-004
  - REQ-AGENTS-COPILOT-PLUGINS-005
  - REQ-AGENTS-COPILOT-PLUGINS-006
  - REQ-AGENTS-COPILOT-PLUGINS-007
---

# Copilot plugin compatibility through observed contracts

## Authority and status

Agents owns the profile-to-process plugin contract, including settings and
observable startup failures. The user confirmed this policy on 2026-11-05.
The [decision](../../../decisions/2026-11-05-copilot-plugin-contract-admission.md)
replaces version/platform certification with observed compatibility.
Work orders 04 and 05 track implementation and evidence in the
[existing package](../../../plans/copilot-runtime-plugin-compatibility/plan.md).

The [parent design](copilot-profile-plugins.md) still owns persistence,
installation identity, root validation, caller coverage, and profile drafts.
The [native-authority design](copilot-profile-plugins-native-qualification.md)
retains policy delegation and diagnostic evidence. Its historical version maps
are not admission authority.

## Requirement mapping

- 001, 003, 004, 005: bounded capability checks, strict adapters, and process admission.
- 002, 006: truthful preview, preserved selections, and durable/UI failure recovery.
- 007: native authority, read-only checks, privacy, and bounded safe failures.

Numbers use prefix `REQ-AGENTS-COPILOT-PLUGINS-`.

## Selected runtime and capability evidence

Remove `inventoryQualifiedVersions`, `platformQualifiedVersions`, and production
admission calls to `RuntimeSupported`, `RuntimeSupportedOn`, and version-derived
`MountsPermitted`. Remove the matching version-list projection from current UI behavior.
Version and platform remain diagnostic identity, never a success or failure predicate.
Do not alter `ManagedNPMRuntimeSpec`, selected package/version, defaults, or runtime source.

Add a provider-owned capability helper in `copilotplugins/compatibility.go`.
It invokes the selected `LaunchContext.RuntimeCommand` with bounded native
help arguments under the final authorized environment and working directory.
The command must successfully advertise the native `--plugin-dir` option as
an option token. A matching word in prose or a partial output buffer is insufficient.
Supported help syntax accepts the option with a separate or equals-style argument.
Keep this small protocol parser independent of version strings and translated descriptions.

Help is evidence of the loading interface, not proof of authentication,
policy consent, valid installations, or successful model/tool execution.
Native acceptance keeps the real repeated-mount check against harmless fixtures.
The documented interface allows repeated `--plugin-dir` arguments; Kandev
continues to preserve separate argv pairs rather than infer a different syntax.

Run the existing `plugin list --json` command and strict production parser
after the capability check. Read installation metadata only through the
existing bounded adapters, then validate matching manifests and canonical roots.
Unknown required fields/shapes, ambiguous identities, missing roots, unsafe
paths, and malformed manifests keep their specific failures. Additive unknown
fields retain the parser's existing policy.

Keep native inventory rows unavailable individually when their source/root is
unsupported. For launch, every selected identity must resolve successfully.
A mixed set with one valid selection and one invalid selection rejects the whole preparation.
An invalid unselected row does not silently remove a selected valid row.

Use a shared overall 20-second request deadline for help plus inventory.
Preserve the current 2 MiB stdout and 32 KiB stderr bounds per command and
the runner's process-tree cleanup. Cancellation terminates owned work and
does not publish a partial successful snapshot. No detached shared probe is added.
Temporary native fixture installation is test-only, never production discovery.

Capability absence uses a new bounded `plugin_loading_unsupported` reason.
Malformed inventory remains `invalid_inventory`; metadata/root failures retain
their existing bounded reasons. `runtime_unsupported` remains a legacy response
decoder, not the result of an unfamiliar version number.

## Shared discovery and admission

`Discover` owns one coherent capability/inventory snapshot for a concrete
profile context. Extend `Snapshot` with observed plugin-loading eligibility.
`DiscoverCopilotPlugins` projects `mount_permitted` from that observation,
not a lookup on `runtime_version`. Failed checks never imply enabled/disabled plugin state.

`ResolveLaunchSelection` follows this order:

1. Return immediately for an empty selection, with no added native or filesystem work.
2. Validate provider, executor, wrapper, and final profile identity.
3. Observe the selected runtime's capability and parse its inventory.
4. Resolve every selected identity through matching metadata and manifests.
5. Deduplicate equal manual roots, reject conflicts, and construct separate mount arguments.
6. Let the selected native process own policy, authentication, and consent.

Discovery and launch use the same helper/parser contract. Each new process
rechecks capability, inventory, and roots; it does not trust a browser result.
Preserve captured profile snapshots, managed-command identity, retained-process
behavior, and preflight before stopping a replacement's live predecessor.

Settings can reuse its existing successful snapshot cache. Keep its HMAC
context revision, authorization scope, runtime generation, 30-second TTL,
256-entry bound, explicit-refresh bypass, and late-response fences.
Cache only safe metadata, not raw help, state, secrets, or resolved roots.
Launch does not acquire a new persistent capability cache or reuse stale roots.

## Truthful runtime preview and profile recovery

The PR #5078 preview contract changes with its browser consumers.
Replace version-derived `supported` and qualified-version lists with
`state: not_checked | compatible | incompatible` and an optional bounded reason.
Retain the exact target version and nullable `selected_profile_count`.
Only a fresh observation for the relevant context can establish compatible or incompatible.
An older response without the new state is unchecked, never a demonstrated rejection.

The agent-wide runtime preview has no concrete profile environment and must
normally report `not_checked`. Keep its read-only profile count, including
disabled non-deleted profiles with selections. Unknown count remains null.
It must not download a candidate, probe every profile, or treat ACP success
as plugin proof. Show that checks occur in profile discovery and new-process preparation.
No new runtime approval gate is added.

Profile discovery shows the actual bounded capability/schema/root reason for
the current context. Keep legacy exact-version guidance only for legacy
responses. Current feedback does not recommend a downgrade solely for a new
version string. Stale or failed observations preserve unknown native status,
removable saved selections, unrelated drafts, and existing Save/Reset behavior.

## Durable startup failure

Before this amendment, `copilotplugins.Error` survived in the backend cause
chain, but `bootstrapFailureFor` converted it to generic bootstrap detail.
`handleAgentStartFailed` then persisted and published that generic detail.
This was the confirmed error-loss boundary for the reported task failure.

Recognize the typed provider error before the generic bootstrap fallback.
Add a closed `plugin_preparation_failed` agent-error cause with an allowlisted
plugin reason and a bounded exact runtime version. Thread that version as
structured evidence from the preparation context; never recover it by parsing error prose.
Use constant safe recovery details, not the provider's raw output or `Error()` text.
Extend the existing bootstrap and `AgentErrorCause` normalizers and frontend type accordingly.

Preserve that cause through the existing start-failure event, durable
`last_agent_error`, status summary, HTTP/WS projection, and transcript status.
Use the existing task/session recovery card and localized cause/reason labels.
Reload must retain the same meaningful explanation. Do not add another error store or retry system.
Keep attempt stamps, execution/session ownership, and stale failure fences.
Cover MCP-created `start_agent: true` and ordinary task creation through their
shared real startup/error projection path. Model, authentication, quota, npm,
and unrelated generic failures retain their current classification.

Only known reason codes and validated runtime versions cross this boundary.
Omit plugin identities, absolute roots, raw commands, environment values,
stdout/stderr, native documents, and credentials from the new failure evidence.
Invalid or foreign-provider evidence falls back to existing sanitized behavior.
No automatic alternate runtime, profile, transport, or identity retry is introduced.

## UI and verification

Keep the existing desktop runtime dialog, phone bottom drawer, direct profile
route, and task recovery card. Preserve scroll owners, safe areas, and control sizes.
All copy uses the seven shipped catalogs and generated Traditional Chinese/pseudo.
Previews in work orders 04/05 show the changed regions only.

Require compatible unknown versions as positive controls through host-native
production-shaped launch contexts. No platform simulation or override is needed
to prove removal of the certification predicate. Record other native platforms
as not run when no matching host is available. Test missing loading capability, malformed
inventory, incompatible installation metadata, mixed valid/invalid selections,
and failed request cancellation as negative controls. Version/platform alone
cannot decide a result. Real native evidence uses the production parser and resolver.

Startup tests assert typed cause/reason/version, persistence/reload, correct
attempt ownership, absence of private data, and unchanged unrelated failures.
Desktop and phone tests prove unchecked preview, actionable profile failure,
and durable task feedback. The task package names exact commands and records their results.

## References

- [GitHub command reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-command-reference): native plugin-directory option.
- [GitHub plugin reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-plugin-reference): inventory containers and read-only mounts.
