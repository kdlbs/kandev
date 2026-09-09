---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-K8S-RETAINED-001
  - REQ-EXECUTORS-K8S-RETAINED-002
---

# Kubernetes Retained Compute Visibility System Design

## Purpose and boundaries

The [requirements](../requirements/kubernetes-retained-compute.md) extend the
existing executor session projection. The feature does not introduce a resource
inventory or infer process activity from Kubernetes readiness.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-EXECUTORS-K8S-RETAINED-001 | Backend projection and classification |
| REQ-EXECUTORS-K8S-RETAINED-002 | Permissions and responsive presentation |

## Backend projection and classification

`internal/kubernetes/sessions.go` already retrieves the task session, checks
task access, and gets the exact recorded Pod. Extend `SessionRow` at that seam.
The HTTP sessions route and `kubernetes.sessions.list` share the same projection.
Current fields retain their meaning. New fields are additive:

| Field | Meaning |
| --- | --- |
| `session_state` | Canonical known `TaskSession.State`, otherwise `unknown` |
| `retention_state` | `active`, `retained`, `terminating`, `terminal`, `missing`, or `unknown` |
| `main_container_requests.cpu` | Optional canonical Kubernetes quantity string |
| `main_container_requests.memory` | Optional canonical Kubernetes quantity string |

The request object appears only after exact identity verification and a matching
main container. It contains only CPU and memory, never a complete resource map.
The UI labels these fields as main-container requests. It does not aggregate
them or call them usage, reserved cluster capacity, or cost.

Classification follows this precedence:

1. Invalid inventory, failed access, malformed states, a missing main container,
   or identity mismatch yields `unknown`.
2. An exact recorded-Pod GET returning NotFound yields `missing`.
3. A verified Pod with a deletion timestamp yields `terminating`.
4. A verified Pod in Succeeded or Failed phase yields `terminal`.
5. A Pending or Running Pod plus session `CANCELLED`, `COMPLETED`, `FAILED`, or `IDLE` yields `retained`.
6. A Pending or Running Pod plus session `CREATED`, `STARTING`, `RUNNING`, or `WAITING_FOR_INPUT` yields `active`.
7. Any remaining combination yields `unknown`.

`CANCELLED` maps to stopped-session wording. Other retained session states keep
their distinct session labels. `WAITING_FOR_INPUT` is never labeled stopped.
The term `active` denotes the recorded session category, not verified agent connectivity.
An Office `IDLE` session can still retain Kubernetes resources under the current executor policy.
Neither category establishes whether a terminal or background process exists.

Derivation uses `TaskSession.State`, not `executors_running.Status`, task workflow
columns, or container readiness. The existing endpoint enumerates inventory rows
without discarding them solely because the agent session stopped.
Keep that behavior so ordinary Stop cannot hide its retained Pod.

Task state and Pod state are separate observations. The projection does not promise
an atomic cross-system snapshot. The next refresh resolves concurrent lifecycle changes.
No new observation timestamp uses `TaskSession.UpdatedAt` as a false stopped-since clock.

## Permissions and failure behavior

Existing task authorization and identity checks remain before Pod projection.
New fields do not bypass executor/task/session filters or expand Kubernetes RBAC.
The backend performs no additional Pod GET, list, metrics, log, or Event request.
The admin-only `session-impact` response remains count-only with its existing semantics.
This feature adds no global count, including for administrators.

An access or identity error omits resource requests and uses `unknown`.
An exact NotFound uses `missing` while preserving the existing sanitized failure field.
Raw session errors and Pod spec content never enter the response.
An unavailable client remains an endpoint error rather than an empty success.

## Responsive presentation

Extend `KubernetesSession` in `lib/types/http-kubernetes.ts` and the existing API tests.
New fields are optional in TypeScript for old responses and fixture compatibility.
The backend owns classification. Frontend helpers select translated labels and
fallbacks, without recreating the state machine.

`KubernetesSessionsCard` remains the only changed product surface.
Desktop keeps the table, with separate session/Pod status lines and a compact
main-container request summary. The task identity becomes a routing-adapter link
to `/t/<encoded-task-id>`. It does not select or resume a session automatically.

Phone presentation reuses the shipped `MobileSessionList` cards in this file and
the direct-navigation pattern from `components/kanban-with-preview.tsx`.
Each card orders Pod identity, session/Pod state, requests, workspace, and task navigation.
The card body navigates to its task because there are no competing inline actions.
The whole card is a semantic link with at least a 44px touch height.
The page remains the single vertical scroll owner. Long identifiers wrap.
No drawer, extra navigation step, nested vertical scroller, or fixed footer is needed.
Desktop link density remains unchanged. Focus indicators and accessible names
identify the task and session on both viewports.

Visible card-level guidance explains Stop retention and Archive/Delete workspace
consequences. It also states that requests exclude sidecars and do not show actual use.
No direct destructive action is added. Existing task routes own those actions and confirmations.

`useKubernetesSessions` retains its 90-second poll and manual refresh behavior.
Refresh failures must remain visible. A stale prior row must not appear as current
confirmed retention. Existing executor-scope generation guards discard late responses.
Missing new fields use unknown/unspecified labels, not inferred defaults.

New copy uses `executors` translation keys in English, Portuguese, and Chinese catalogs.
Traditional Chinese derives through the existing converter. Pseudo-locale tests
cover expansion without translating state discriminants or Kubernetes quantities.

## Verification and persistence

Backend tables cover every session state, Pod phase, deletion, NotFound, access
error, identity mismatch, missing request, explicit zero request, and admission-mutated request.
Actual Pod values win over current profile values and the original template.
HTTP and WebSocket responses share field tests. Fake-client action counts prove
unauthorized rows trigger no Pod read and visible rows require one exact GET.

Component tests cover optional fields, distinct labels, links, loading, and errors.
Desktop and mobile E2E extend the existing Kubernetes configuration specs.
They verify navigation, safe guidance, responsive geometry, and post-refresh changes.
A Kind lifecycle scenario proves Stop retains the Pod and refresh shows it,
then Resume clears the retained category without replacing the Pod.

No schema, lifecycle, poller, feature-toggle, or credential change is required.
Public documentation changes describe the new fields and retained-resource wording.

## Related decisions and delivery

- [Kubernetes resource ownership](../../../decisions/2026-08-24-kubernetes-executor-resource-ownership.md)
- [Kubernetes foundation](../../kubernetes-executor/spec.md)
- [Retained compute plan](../../../plans/kubernetes-retained-compute/plan.md)
