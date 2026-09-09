---
created: 2026-09-09
status: implemented
requirements:
  - REQ-EXECUTORS-K8S-RETAINED-001
  - REQ-EXECUTORS-K8S-RETAINED-002
system_design:
  - ../../specs/executors/system-design/kubernetes-retained-compute.md
legacy_specs:
  - ../../specs/kubernetes-executor/spec.md
---

# Implementation Plan: Kubernetes Retained Compute Visibility

## Overview

Show recorded session state alongside verified Pod state and main-container requests.
First extend the shared read-only backend projection. Then expose it in the
existing desktop table and mobile cards, with task navigation and retention guidance.

Deliver this after [worker presets](../kubernetes-worker-presets/plan.md) and
[startup timing](../kubernetes-startup-timing/plan.md) by preference.
There is no code dependency on either package. Shared documentation and Kind tests
make sequential integration preferable.

## Scope

### In scope

- Additive session, retention, and main-container request fields on the existing status response.
- Active sessions desktop/mobile presentation, task navigation, and localized guidance.
- Unit, authorization, responsive E2E, and real Stop/Resume evidence.

### Out of scope

- Auto-cleanup, new destructive controls, metrics-server access, aggregate cost, or changed resource ownership.
- New inventory tables, background pollers, or changes to the admin-only session-impact count.

## Technical approach

Extend `SessionRow` and `sessionRow` in `internal/kubernetes/sessions.go`.
Use its existing task-session read and identity-verified Pod GET.
Derive the design's finite retention state in a local helper and emit only
allowlisted main-container CPU/memory quantities.

Extend `lib/types/http-kubernetes.ts` with optional compatibility fields.
The existing `useKubernetesSessions` hook supplies both viewports.
Extend `KubernetesSessionsCard` through small helpers for labels, requests, and navigation.
Use backend classifications rather than another frontend state machine.

## Tests

| Acceptance | Evidence |
| --- | --- |
| RETAINED-001.1 through 001.3 | `TestKubernetesRetentionProjection` with every canonical session state and Pod outcome |
| RETAINED-001.4 | `TestKubernetesMainContainerRequests` with missing/zero quantities and admitted values |
| RETAINED-002.5 | Existing HTTP/WS authorization tests plus exact GET action counts |
| RETAINED-001.5 | Hook/component refresh, error, and stale-response cases |
| RETAINED-002.1 through 002.4 | Desktop/mobile E2E, translated copy, and semantic task navigation |

IDs use the `AC-EXECUTORS-K8S-` prefix. New tests precede production changes.

## E2E tests

- `tests/settings/kubernetes-executor.spec.ts`, `chromium`: recorded session and
  Pod state, request values, refresh/error transitions, safe guidance, and task navigation.
- `tests/settings/mobile-kubernetes-executor.spec.ts`, `mobile-chrome`: the same
  outcome through tappable phone cards, long identifiers, and viewport containment.
- `tests/kubernetes/kubernetes-executor.spec.ts`, `containers`: actual managed-PVC
  Stop/Resume updates the profile status while Pod/PVC UIDs remain unchanged.

The mobile exemplar is the current `MobileSessionList`, with direct task navigation
matching `kanban-with-preview.tsx`. No new overlay or vertical scroll owner is introduced.

## Work orders

- [x] [Task 01: Project retained Kubernetes resources](task-01-retention-projection.md)
- [x] [Task 02: Show retained resource status](task-02-retention-status.md)

## Verification results

Implemented. Backend projection, translated desktop/mobile presentation, and
real managed-PVC Stop/Resume/terminal cleanup evidence passed with exact task
and resource identities preserved.

## Risks

- A Pending Pod is retained but not proof of allocated compute or charges.
- Session state and Pod state are separate observations, so refresh must tolerate transitions.
- Sidecars and Pod-level resources are excluded. The UI must state that scope.
- Archive/Delete guidance describes possible workspace loss. It must not look like a harmless compute-release action.
