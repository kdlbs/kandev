---
status: current
system: platform
requirements:
  - REQ-PLATFORM-AGENTCTL-OPERATOR-PROFILING-001
---

# Agentctl operator profiling design

## Purpose and boundaries

The agentctl entry point calls `startPprofListenerIfEnabled` once after its
existing janitor starts. The helper reads an explicit environment variable,
trims whitespace, and returns immediately when it is empty.

## Address and routing

`isLoopbackAddr` uses `net.SplitHostPort` and accepts localhost or a
parsed loopback IP. Wildcards, non-loopback IPs, and missing ports are refused.
The server uses a dedicated mux with the standard pprof index, cmdline,
profile, symbol, and trace handlers and a ten-second read-header timeout.
Existing session routers and their tokens are unchanged.

The environment variable is an operator-controlled opt-in. The local listener
has no additional authentication, so other local users may access it while it
is enabled. Keep it disabled when profiling is not needed.

## Failure and lifetime

The listener runs in a goroutine for the process lifetime. Bind/serve failures
produce a warning and do not fail the existing agentctl runtime. No automatic
retry, process stop, configuration persistence, or operating activation is added.

## Requirement mapping

`REQ-PLATFORM-AGENTCTL-OPERATOR-PROFILING-001` is implemented by the
entry-point call, optional address check, dedicated mux, and warning-only serve
failure path. Explicit temporary checks cover the address matrix and an actual
HTTP request through the entry helper.
