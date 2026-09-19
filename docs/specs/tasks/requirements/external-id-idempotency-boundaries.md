---
status: draft
system: tasks
created: 2026-08-07
owners:
  - nova28
---


# External task ID idempotency boundaries Requirements



## Overview



The external identity contract has explicit exclusions and records unresolved questions without silently expanding its guarantee.



## Out of scope

### Deferred write surfaces (designed, not built)

Neither has a named consumer today, and they are **not equally cheap** to
enable. Each is additive.

| Surface | Cost to enable later |
|---|---|
| WebSocket `task.create` | **Low.** One request-struct field, one line in the service-call literal, and skip the launch and last-used recording on a found outcome. One file. Deferred because it is the UI's own path: nothing retries on the UI's behalf. |
| Plugin host | **Meaningfully higher.** `plugin.pb.go` and `plugin_grpc.pb.go` are generated and committed, so a proto edit requires regenerating them through the Makefile proto target (installs `protoc-gen-go`, needs network). It also needs a new SDK method — **do not change the existing `TaskReader.Create` signature**, which is source-breaking for shipped plugins — a mapper change, and a release in each separate plugin repo. |

### Other non-goals

- **Caller-visible liveness or ownership.** Preparing creation has an internal
  lease/handle, but Found responses expose neither and do not claim work is dead.
- **Caller-driven repair, resume, adoption, reclamation, or garbage collection.**
  Only the mandatory internal Runtime may recover the durable manifest.
- **Caller-selected timeout or expiry for an unsettled task.**
- **A tombstone for deleted identities.** See *Idempotency is scoped to the
  task's lifetime*.
- **An MCP lookup tool.** MCP gets an idempotent create-if-absent, not a probe;
  no in-scope MCP flow needs to ask without being willing to create. Adding a
  read-only tool later is small and additive.
- **Changing an external ID in place.** Write-once; re-key via release + create.
- **System-generated external IDs.**
- **Idempotency for anything other than task creation.** `spawn_session_kandev`
  is not covered.
- **Retiring the existing integration dedupe tables.**
- **Replacing the office `runs.idempotency_key` mechanism.**
- **Request-payload fingerprinting.**
- **Cross-workspace uniqueness or a global namespace.**
- **A UI surface** for entering, displaying, or releasing external IDs.
- **Bulk lookup or bulk release.**
- **Backfilling external IDs onto existing tasks.**

## Open questions

None. Every decision is settled; downstream steps implement as written and route
back here if a change is needed.

## Requirements



### REQ-TASKS-EXTERNAL-ID-BOUNDARIES-001: External task ID idempotency boundaries



**Intent:** The external identity contract has explicit exclusions and records unresolved questions without silently expanding its guarantee.



#### Acceptance criteria



- **AC-TASKS-EXTERNAL-ID-BOUNDARIES-001.1:** A caller relying on creation
  idempotency shall not infer liveness, ownership, or authority to repair,
  complete, abort, reclaim, expire, or automatically release an unsettled task.
- **AC-TASKS-EXTERNAL-ID-BOUNDARIES-001.2:** Handles, CreationPlan step state/
  evidence, and recovery leases shall remain absent from REST, MCP, Office
  runtime, and authenticated agentctl responses and usable only by the Created
  coordinator or mandatory Runtime.
- **AC-TASKS-EXTERNAL-ID-BOUNDARIES-001.3:** Deferred WebSocket/plugin
  external-ID create support and integration/runs dedupe replacement shall
  remain outside this package; lookup-first REST, MCP, Office runtime, and
  authenticated agentctl handling plus aggregate release are in scope.
