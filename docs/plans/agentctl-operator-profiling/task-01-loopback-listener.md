---
id: "01-loopback-listener"
title: "Port optional loopback profiling"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-AGENTCTL-OPERATOR-PROFILING-001
acceptance_criteria:
  - AC-PLATFORM-AGENTCTL-OPERATOR-PROFILING-001.1
  - AC-PLATFORM-AGENTCTL-OPERATOR-PROFILING-001.2
  - AC-PLATFORM-AGENTCTL-OPERATOR-PROFILING-001.3
system_design:
  - ../../specs/platform/system-design/agentctl-operator-profiling.md
---

# Optional loopback profiling

## Scope

Add the existing helper and one entry-point call. Preserve the native session
routers, token authentication, process startup, and all existing defaults.

## Validation

The explicit disposable overlay run passed with race detection (agentctl
1.315s): the original seven-address matrix, an actual HTTP pprof index response
from the startup helper, disabled/invalid address calls, and the three existing
parent-liveness checks. The actual index included the goroutine profile.

The existing documentation coverage evaluator returned covered with no errors.
Existing control and instance HTTP servers supply their own handlers, so pprof's
package registration does not expose the default mux through those servers.
The warning-only serve failure path was reviewed in the two-file source diff.
No operating listener was enabled; disposable checks are removed after delivery.
