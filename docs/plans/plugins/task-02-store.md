---
id: task-02
title: Filesystem registration store + credentials
status: done
wave: 1
depends_on: [task-01]
plan: docs/plans/plugins/plan.md
---

# Filesystem registration store + credentials

## Title
Persist plugin registrations to `~/.kandev/plugins/{id}.yml` with a bcrypt-hashed
api_key and an encrypted (recoverable) webhook secret, plus operator config at
`{id}.config.yml`.

## Inputs
- Spec `docs/specs/plugins/spec.md` → "Plugin registration (filesystem-backed)"
  and "Persistence guarantees".
- **Credential storage refinement (resolves a spec inconsistency — capture in the
  task-15 spec update):**
  - `api_key`: kandev only ever *verifies* inbound requests → store as **bcrypt
    hash** only. Cleartext returned once at registration.
  - `webhook_secret`: kandev must *HMAC-sign* outbound event deliveries →
    bcrypt (one-way) is impossible. Store the secret in the **encrypted secrets
    store** (`internal/secrets` via `internal/integrations/secretadapter.Adapter`,
    which is reversible via `Reveal`). The `{id}.yml` keeps only a secret
    *reference id*, not the value. Cleartext returned once at registration.
- Uses `Manifest` from task-01 (import `internal/plugins/manifest`).
- bcrypt: `golang.org/x/crypto/bcrypt`. Secret adapter interface: inject a small
  interface `SecretVault { Set(ctx,id,name,value) error; Reveal(ctx,id) (string,error); Delete(ctx,id) error }`
  so the store is testable with a fake (the real one is `secretadapter.Adapter`).

## Acceptance
1. `Record` struct = manifest + runtime fields (status, api_key_hash,
   webhook_secret_ref, registered_at, last_health_check).
2. `store.Store` interface: `List`, `Get(id)`, `Save(*Record)`, `Delete(id)`,
   `GetConfig(id)`, `SetConfig(id, map)`.
3. `NewFSStore(dir string, vault SecretVault)`; `Register(ctx, manifest) (*Record, Credentials, error)`
   generates random `api_key`+`webhook_secret` (crypto/rand), bcrypt-hashes the
   api_key, stores the webhook_secret in the vault under ref
   `plugin-webhook-secret:{id}`, writes `{id}.yml`, returns cleartext
   `Credentials{APIKey, WebhookSecret}` once.
4. `VerifyAPIKey(id, key) (bool, error)` (bcrypt compare);
   `RevealWebhookSecret(ctx, id) (string, error)` (vault Reveal).
5. Duplicate id on Register → typed `ErrAlreadyExists`. `Delete` also deletes the
   vault secret.

## Files
- `apps/backend/internal/plugins/store/store.go`
- `apps/backend/internal/plugins/store/fs_store.go`
- `apps/backend/internal/plugins/store/fs_store_test.go` (`t.TempDir()` + fake vault)

## Verification
- `go test ./internal/plugins/store/...` from `apps/backend`
- `make -C apps/backend lint`

## Output contract
Report: file format, the api_key(bcrypt)/webhook_secret(encrypted-ref) split, the
SecretVault interface shape (so task-05/06 can reveal the secret for signing).
Do not edit `internal/plugins/manifest/` (import only). Stay within `internal/plugins/store/`.

## Dependencies
task-01 (imports manifest types).
