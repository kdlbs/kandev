---
id: "04-backend-parity"
title: "Kubernetes backend parity"
status: completed
wave: 2
depends_on: ["01-backend-foundation"]
plan: "plan.md"
spec: "../../specs/kubernetes-executor/spec.md"
---

# Task 04: Kubernetes backend parity

- **Acceptance:** credentials, repository materialization, scripts/placeholders,
  skills, Office CLI injection, editor capability, liveness, and metrics treat
  Kubernetes as the intended remote containerized runtime.
- **Acceptance:** every explicit executor switch has a focused Kubernetes test
  and no behavior relies on a permissive default branch.
- **Verification:** `cd apps/backend && go test ./internal/orchestrator/executor ./internal/backendapp ./internal/task/service ./internal/scriptengine ./internal/agent/runtime/lifecycle/skill ./internal/editors/capabilities ./internal/office/service`
- **Files likely touched:** the explicit executor switches named in the plan's
  Remote/container parity section and adjacent tests; no lifecycle or API handler files.
- **Dependencies:** Task 01.
- **Parallelism:** parallel-safe with Tasks 02 and 03; owns parity switches and
  their tests, excluding files explicitly owned by those tasks.
- **Inputs:** spec What/Scenarios; existing Docker/Sprites/SSH switch behavior.

## Results

- Added Kubernetes to the remote/container executor taxonomy used by
  credential resolution, workspace-source materialization, default scripts,
  placeholders, skills, Office CLI injection, embedded editor capability, and
  execution metrics. Unknown executor types now fail closed at the affected
  delivery/materialization boundaries.
- Made the selected Kubernetes profile authoritative for all Pod-template and
  workspace launch keys, including empty values that clear task-supplied
  overrides. Non-Kubernetes profiles retain their previous metadata behavior.
- Added path-aware agentctl placeholders so Kubernetes installs
  `/opt/kandev/agentctl` while leaving process startup to the Pod bootstrap;
  the legacy provider remains byte-for-byte compatible.
- Verification passed:
  `cd apps/backend && go test ./internal/orchestrator/executor ./internal/backendapp ./internal/task/service ./internal/scriptengine ./internal/agent/runtime/lifecycle/skill ./internal/editors/capabilities ./internal/office/service -count=1`.
