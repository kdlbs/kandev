---
id: "15-per-org-secret-keys"
title: "Per-org secret encryption keys"
status: todo
wave: 4
depends_on: ["04-service-layer-org-scoping"]
plan: "plan.md"
spec: "../../specs/multi-tenancy/spec.md"
---

# Task 15: Per-Org Secret Encryption Keys

Row scoping decides who may read a secret; it does nothing about the
ciphertext. Today one master key seals every row on the instance, so at rest
every tenant collapses to one key sitting beside the database. This is the one
place the spec's "strong enough for untrusted co-tenants" claim does not hold.

What this task buys is blast-radius reduction on a partial compromise, and the
envelope structure that a real crypto-shred later requires. It does NOT make
deletion shred an org out of existing backups; see the spec's Out of scope for
why and for what would.

## Acceptance

- `org_keys` stores one wrapped data encryption key per org, created with the
  org and removed with it. The plaintext DEK is never persisted.
- `MasterKeyProvider` and `<data-dir>/master.key` are unchanged: the master key
  wraps DEKs instead of sealing rows, so a self-hosted upgrade changes nothing
  operationally and no KMS is introduced.
- Secret writes seal under the owning org's DEK; reads unwrap that org's DEK.
- Deleting an org destroys its DEK, and a test proves its secrets are
  unreadable **in the live database** afterwards.
- A second test pins the limit rather than hiding it: restoring a pre-deletion
  backup onto the same host makes that org's secrets readable again, because
  `org_keys` is inside the snapshot and `master.key` is not. A test named for a
  shred that the design does not deliver would be worse than no test at all.
- A missing or unusable `org_keys` row fails every secret read and write in
  that org with a named error and **never** falls back to the master key. A
  fallback would silently restore the shared-key property this removes.
- The tenancy migration re-wraps existing secrets under the default org's DEK
  in the same first-boot pass that assigns `org_id`, and is idempotent.
- A cross-org test proves org A's DEK cannot decrypt org B's ciphertext.
- With `features.multiTenancy` off, encryption is byte-identical to today.

## Verification

- `go test ./internal/secrets/... ./internal/org/...` from `apps/backend`
- `go test ./internal/secrets/... -run 'TestCrossOrgDecryptFails|TestDeleteDestroysDEK|TestNoMasterKeyFallback|TestRestoredBackupStillDecrypts'`
- `KANDEV_TEST_POSTGRES_DSN=... go test ./internal/secrets/...`

## Files Likely Touched

- `apps/backend/internal/secrets/crypto.go`, `sqlite_store.go`
- `apps/backend/internal/org/keys.go`, `store.go`, `service.go`
- `apps/backend/internal/tenancy/registry.go` (classify `org_keys`)

## Inputs

- Spec: What (per-org key bullet), Data model (`org_keys`), Failure modes,
  Persistence guarantees, Out of scope (the root key is explicitly not moved).

## Output Contract

Report the cross-org decryption test, the live-database deletion test, the test
that pins the restored-backup limit, the no-fallback test and its documented
recovery route, the migration's re-wrap counts, RED/GREEN commands, and set
this task plus its plan checkbox to done.
