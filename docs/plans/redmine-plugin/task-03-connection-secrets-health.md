---
id: "03-connection-secrets-health"
title: "Connection lifecycle, workspace-scoped secrets, health poll"
status: completed
wave: 2
depends_on: ["02-plugin-repository-bootstrap"]
plan: "plan.md"
spec: "../../specs/redmine-plugin/spec.md"
---

# Task 03: Connection, secrets, health poll

## Intent

Implement the Redmine REST client's auth path and the plugin's own connection
lifecycle: validate a base URL + API key against `GET /users/current.json`,
distinguish invalid-credentials / API-disabled / unreachable failures, encrypt the key
with workspace-derived material under its workspace-composed key before host-vault
storage, and run a ~90s jittered health-poll loop per
connected workspace that flips `plugin_state` health without deleting the key on
failure.

## Owned paths

Attached `yattdev/kandev-plugin-redmine` worktree: REST client, connection service,
secret key-composition + encryption layer, health-poll loop.

## Dependencies

Task 02.

## Acceptance

1. A valid base URL + API key validates via `GET /users/current.json` and persists;
   v0.1.0 encrypts the key with workspace-derived AES-256-GCM key material and stores
   that ciphertext under `redmine.<workspace_id>.api_key` in Kandev's encrypted secret
   vault. It is not a declared config field: plaintext reaches only the authenticated
   plugin action and plugin process, while `SetSecret`/`GetSecret` carry the inner
   ciphertext. The host vault adds encryption at rest; settings responses, frontend
   payloads, logs, and task metadata never expose the key.
2. An invalid API key is reported as a distinct plugin error (not a bare host-level
   401 — see spec Failure modes on why the native implementation had to avoid 401 for
   this exact case); the stored config is unchanged.
3. A REST-API-disabled instance (403 on `/users/current.json`) is reported distinctly
   from an invalid key.
4. An unreachable host is reported distinctly from both of the above.
5. Rotating the API key replaces the host-vault value under the same
   `redmine.<workspace_id>.api_key` key; deleting the connection removes both the
   secret and connection `plugin_state`.
6. The health-poll loop runs on its own `Start(ctx)`/`Stop()` lifecycle, selects on
   `ctx.Done()` in every wait (no bare `time.Sleep` in backoff), flips `last_ok`/
   `last_error` in `plugin_state` without deleting the stored key on failure, and does
   not leak goroutines across plugin disable/enable cycles.
7. Two different workspaces' connections cannot read, delete, or affect each other's
   secret or state, even though the host's secret RPCs are namespaced only by plugin
   ID. Tests cover `SetSecret`, `GetSecret`, and `DeleteSecret` using separate,
   deterministic workspace-composed keys, and prove that no API key appears in an
   action response, state value, or log fixture.

## Verification

```sh
# From the attached plugin worktree:
go test ./internal/redmineclient/... ./internal/connection/... -race
go test ./... -run TestHealthPoll -race
```

## Risks

Cross-workspace secret isolation is entirely this task's responsibility (see plan
Risks); a bug here is a credential leak, not a missing feature. The 401-vs-plugin-error
distinction matters even though this plugin never talks to the Kandev host's own HTTP
API directly — get the error taxonomy right at the plugin-action-response layer so the
frontend never needs to special-case a Redmine-specific 401.
