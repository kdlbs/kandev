---
id: "01-identity-discovery-endpoint"
title: "Identity discovery endpoint"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-IDENTITY-DISCOVERY-001
acceptance_criteria:
  - AC-EXECUTORS-SSH-IDENTITY-DISCOVERY-001.1
  - AC-EXECUTORS-SSH-IDENTITY-DISCOVERY-001.2
  - AC-EXECUTORS-SSH-IDENTITY-DISCOVERY-001.3
  - AC-EXECUTORS-SSH-IDENTITY-DISCOVERY-001.5
system_design:
  - ../../specs/executors/system-design/ssh-identity-discovery.md
---

# Task 01: Identity Discovery Endpoint

## Summary

Add `GET /api/v1/ssh/identities`, which reports the private key files on the
backend host that Kandev could use as a `file` identity source, with no path
input and no key material in the response.

## Scope

- `apps/backend/internal/ssh/identities.go` (new): enumerate `$HOME/.ssh`
  non-recursively, plus `IdentityFile` values from `$HOME/.ssh/config`;
  classify each candidate; de-duplicate by resolved path.
- Classification per system design D2: bounded 4 KiB header read, then
  `ssh.ParseRawPrivateKey`, with `*ssh.PassphraseMissingError` meaning
  `encrypted: true` and any other parse error meaning "not a candidate".
- Register the route in `apps/backend/internal/ssh/handlers.go` behind
  `authn.RequireAdmin()`.
- Resolve `$HOME` with `os.UserHomeDir()`, matching
  `executor_ssh_connection.go`.

## Exclusions

- No path, glob, or root parameter on the endpoint, now or as a hidden option.
- No change to the gating of the existing `/api/v1/ssh/*` routes.
- No fingerprint computation, no `.pub` reading, no agent enumeration.
- No frontend work.

## Implementation acceptance conditions

1. With a temp `$HOME` containing an unencrypted key, an encrypted key, a
   `.pub`, `known_hosts`, `config`, `authorized_keys`, a subdirectory, and a
   non-key text file, the endpoint returns exactly the two keys, with
   `encrypted` correct for each, and `key_type` set for the unencrypted one.
2. No response field contains any byte read from a key file: a test asserts the
   serialized JSON does not contain a substring of either fixture key's
   contents.
3. A non-administrator caller is rejected by the gate, and an unreadable or
   absent `$HOME/.ssh` returns 200 with an empty list rather than an error.

## Verification commands

```
make -C apps/backend test ARGS='-run TestIdentities ./internal/ssh/...'
make -C apps/backend lint
```

## Likely files

- `apps/backend/internal/ssh/identities.go` (new)
- `apps/backend/internal/ssh/identities_test.go` (new)
- `apps/backend/internal/ssh/handlers.go`

## Dependencies and risks

- `golang.org/x/crypto/ssh` is already a direct dependency; no new module.
- The `ssh_config` root comes from the same `kevinburke/ssh_config` parse the
  resolver uses. If that parse fails, degrade to the `$HOME/.ssh` root only
  rather than failing the request.
- Symlinked or agent-backed pseudo-keys may classify as non-candidates. That is
  the intended safe direction; the custom path in task-02 covers them.

## Results

Implemented. `apps/backend/internal/ssh/identities.go` enumerates `$HOME/.ssh` and `~/.ssh/config` IdentityFile values, classifies via a bounded header read plus `ssh.ParseRawPrivateKey`, and serves `GET /api/v1/ssh/identities` behind `authn.RequireAdmin()`.

- `go test ./internal/ssh/...` — ok (5 identity tests, all subtests pass)
- `golangci-lint run ./internal/ssh/...` — 0 issues
