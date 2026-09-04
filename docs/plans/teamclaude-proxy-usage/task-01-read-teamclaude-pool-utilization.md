---
id: "01-read-teamclaude-pool-utilization"
title: "Add proxy usage resolver and TeamClaude pool adapter"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.3
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.4
system_design: []
---

# Task 01: Read TeamClaude Pool Utilization

## Summary

Add an ordered proxy-usage resolver layer, then register a TeamClaude status
adapter for Claude ACP profiles that explicitly use a loopback TeamClaude
upstream URL.

## Acceptance

- Only explicit `USAGE_PROXY_VENDOR=teamclaude` plus a loopback
  `ANTHROPIC_BASE_URL` enables TeamClaude status discovery.
- New proxy adapters can register a resolver without adding proxy-specific
  branching to the common usage adapter.
- The usage result contains only the best usable pool capacity per quota
  window and no account names or credentials.
- Invalid payloads, unavailable status endpoints, and no usable accounts fail
  without falling back to a direct Anthropic OAuth request.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/agent/usage ./internal/backendapp
```

## Files likely touched

- `apps/backend/internal/agent/usage/client_teamclaude.go`
- `apps/backend/internal/backendapp/{usage_adapter,teamclaude_usage}.go`
- Focused unit tests for parsing and loopback discovery.

## Results

- Added the ordered `usageProxyResolver` seam ahead of direct provider usage
  clients. A future proxy adds a resolver; the common usage adapter remains
  proxy-neutral.
- TeamClaude is selected only when the profile sets
  `USAGE_PROXY_VENDOR=teamclaude`. The resolver then derives the status URL
  from a loopback `ANTHROPIC_BASE_URL` and refuses remote or credential-bearing
  URLs.
- Pool utilization reports the least-used currently usable account for each
  known quota window, without retaining account names or credentials.
