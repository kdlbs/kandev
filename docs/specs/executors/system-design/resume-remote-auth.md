---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-RESUME-REMOTE-AUTH-001
---

# Remote credential resume design

## Boundaries and requirement mapping

The orchestrator executor builds resume requests. Lifecycle consumes their environment definitions when launching remote processes.

| Criteria | Design section |
| --- | --- |
| AC-EXECUTORS-RESUME-REMOTE-AUTH-001.1, .2 | Request construction |
| AC-EXECUTORS-RESUME-REMOTE-AUTH-001.3 | Identity and failures |
| AC-EXECUTORS-RESUME-REMOTE-AUTH-001.4 | Current Kubernetes profile selection |

## Request construction

After restoring recorded executor identity and the current Kubernetes profile selection, `buildResumeRequestAtCredentialBoundaryWithOptions` uses `executorNeedsResolvedCredentials` and `applyContainerCredentials`, as fresh launch does. This resolves selected `remote_auth_secrets` before host GitHub bridge selection, GitHub policy routing, and `resolveLaunchEnvironment`.

The existing resolver maps method IDs to environment keys and supplies both GitHub token aliases. Environment construction carries the resolved values into lifecycle definitions. This path does not copy ambient control-plane credentials or rerun preparation scripts.

## Current Kubernetes profile selection

The recorded Kubernetes profile ID selects the current profile row. Resume reloads its `remote_auth_secrets` with its environment definitions before the persisted running metadata is merged into the launch request. The current value replaces an older binding; an empty value remains present so the merge cannot restore the old selection. A missing profile or selection leaves remote authentication unselected. Credential values are still resolved later through the existing scoped secret store and managed GitHub policy.

## Identity and failures

Recorded Kubernetes metadata remains authoritative for executor, profile, and workload selection. Restoring credentials does not replace the recorded Pod or storage snapshot. [Profile environment restoration](kubernetes-failure-recovery.md#resume-environment-sources) remains a separate source of environment definitions.

`revealGlobalSecret` keeps the incoming context and uses the scoped store when available. Missing or inaccessible `remote_auth_secrets` selections retain the existing resolver's skip behavior. These method selections are separate from profile and repository environment secret references, which continue to block launch or cold resume when unavailable.

The fix preserves secret visibility and the GitHub credential policy. In executor mode, an unreadable token does not select managed authentication.

No new persistence, migration, cache, logging, or public API is required. Existing diagnostics remain unchanged; credentials must not be added to diagnostics.
