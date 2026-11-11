---
id: "01-scoped-auth-evidence"
title: "Validate scoped authentication evidence and declare Claude API keys"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-REMOTE-AUTH-ADMISSION-001
  - REQ-WORKSPACES-REPOSITORY-SECRETS-001
acceptance_criteria:
  - AC-AGENTS-REMOTE-AUTH-ADMISSION-001.1
  - AC-AGENTS-REMOTE-AUTH-ADMISSION-001.2
  - AC-AGENTS-REMOTE-AUTH-ADMISSION-001.4
  - AC-AGENTS-REMOTE-AUTH-ADMISSION-001.5
  - AC-AGENTS-REMOTE-AUTH-ADMISSION-001.7
  - AC-WORKSPACES-REPOSITORY-SECRETS-001.2
  - AC-WORKSPACES-REPOSITORY-SECRETS-001.4
  - AC-WORKSPACES-REPOSITORY-SECRETS-001.10
system_design:
  - ../../specs/agents/system-design/remote-auth-admission.md
  - ../../specs/workspaces/system-design/repository-secrets.md
---

# Task 01: Scoped authentication evidence and catalog

## Summary

Add Claude's distinct API-key method and a readiness checker that uses current scoped runtime bindings.
Keep this result independently verifiable before Task 02 connects it to guarded workflow admission.

## In scope

- Add `ANTHROPIC_API_KEY` to `ClaudeACP.RemoteAuth()` without a setup script or raw display copy.
  Preserve the OAuth method ID and script.
- Add a small checker in `agent/remoteauth`, accepting the concrete profile, executor profile, catalog, and user-visible secret reader.
  Require an established identity before reads. Positive synthetic identity retains authentication-disabled behavior.
- Reuse `environment.Validate`/`Resolve` and existing Global reference/reveal validation.
  Validate profile entry shape and source identities without permissive fallback or decrypted-value conflict comparison.
- Decode authentication configuration before any successful return.
  Reject malformed JSON/type shapes and invalid selected-family references.
  Unrelated agent methods cannot prove Claude authentication.
- Resolve every required selected-profile env binding and every explicitly configured selected-family authentication binding.
  Then require at least one supported, nonempty route.
  Cover a valid API key plus a broken unrelated required profile binding, and a valid API key plus an invalid selected OAuth binding.
- Return sanitized readiness/errors only. Keep credential values local and uncached.

## Out of scope

Backend service wiring, workflow mutation, provider network login, owner grants, model resolution, and public UI changes beyond the catalog data.
Do not change existing secret ownership or scope rules.

## Acceptance

1. A caller-owned synthetic Global API key supplies positive evidence without executor auth duplication. Missing, empty, inaccessible, wrong-scope, deleted, revoked, and corrupt values deny.
2. Mixed valid/invalid required bindings, conflicts, malformed config, missing actor/store, cancellation, and deadlines never produce success or disclose values/IDs.
3. Catalog tests preserve OAuth and existing file/methods-less catalogs. The API-key method has its own stable ID and no OAuth script or file target.

## TDD and verification

Start with proposed `TestBuildCatalogClaudeAPIKeyAndOAuth` and `TestRemoteAuthAdmission` regressions.
The catalog test must fail on the current OAuth-only declaration.
The new checker must fail the auth matrix before its implementation, then pass after the correction.
Use real scoped SQLite under `t.TempDir()` for access/scope/deletion tests and injected read failures for cancellation/decrypt controls.
Use synthetic values only. Assert redaction with distinct secret-ID and value sentinels.

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/agent/remoteauth ./internal/agent/agents -run '^(TestRemoteAuthAdmission.*|TestBuildCatalog.*|TestClaude.*RemoteAuth.*)$' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/environment -count=1)
git diff --check
```

Run changed-Go lint from the repository root with the actual changed file list:

```bash
bash scripts/lint-go-changed apps/backend/internal/agent/agents/claude_acp.go apps/backend/internal/agent/remoteauth/admission.go apps/backend/internal/agent/remoteauth/admission_test.go apps/backend/internal/agent/remoteauth/catalog_test.go
```

Adjust that list only to match the implemented file ownership. Record every command and result.

## Files likely touched

- `apps/backend/internal/agent/agents/claude_acp.go`
- `apps/backend/internal/agent/remoteauth/admission.go` (new)
- `apps/backend/internal/agent/remoteauth/admission_test.go` (new)
- `apps/backend/internal/agent/remoteauth/catalog_test.go`
- Existing profile-env shape helper only if a minimal extraction is necessary. Preserve its controller behavior and cover its original callers.

## Dependencies

None. Re-read [the proposed ADR](../../decisions/2026-11-08-profile-auth-admission-evidence.md) before implementation.
Resolve current main and PR #4995/#5184 heads again. Neither unmerged branch supplies implementation authority.

## Risks

Metadata alone does not prove a readable value. A decrypt check does not prove provider-side validity.
Do not replace the user-visible store with a raw store or broaden scope to implement this checker.
Unknown method handling must remain compatible with unrelated registered agents.

## Parallelism

`sequential`

## Inputs

- [Design: catalog, effective settings, and actor boundary](../../specs/agents/system-design/remote-auth-admission.md)
- `apps/backend/internal/secrets/store.go`, `user_visible_store.go`, and `sqlite_store.go`
- `apps/backend/internal/agent/runtime/environment/{environment,profile,tier}.go`
- `apps/backend/internal/agent/settings/controller/profile_secret_scope_test.go`
- [Evidence](evidence.md#diagnostic-results)

## Results

Done.

- `agents/claude_acp.go` now declares `ANTHROPIC_API_KEY` beside the existing OAuth method. The API-key method has no setup script, hint, label, or file target.
- `agent/profileenv/validate.go` centralizes profile environment shape validation; settings writes and admission use the same rules.
- `agent/remoteauth/admission.go` implements scoped admission. It validates profile and executor environment entries, decodes both executor credential maps, rejects malformed selected-family references, validates every configured selected-family auth binding, reveals all required secret bindings under the current actor, and then requires at least one file or env route. Errors carry no secret ID or value.
- `remoteauth/admission_test.go` uses a real scoped SQLite store under `t.TempDir()` and covers a valid caller-owned key, executor API-key methods, literal keys, selected env methods recorded in `remote_credentials`, file routes, methods-less catalogs, missing identity, wrong owner, workspace scope, empty values, deletion, broken unrelated bindings, invalid selected OAuth bindings, malformed configuration, unknown methods, padded references, empty declarations, pre-canceled literal/file routes, cancellation during a successful resolution, duplicate keys, reserved keys, and conflicting profile/executor bindings.
- Indexed Git regression coverage proves zero-count blocks, indexes beyond the count, and composed profile/executor Git blocks are admitted alongside a valid auth route. The checker relies on `environment.Resolve` composition and validates secret emptiness at the reveal boundary rather than requiring every original indexed key to survive the final map.
- `remoteauth/catalog_test.go` proves the two Claude methods keep independent identities and that only OAuth retains its setup script.

Verification:

```text
go test -trimpath -race ./internal/agent/remoteauth ./internal/agent/profileenv ./internal/agent/agents -run '^(TestBuildCatalog.*|TestRemoteAuthAdmission.*|TestClaude.*RemoteAuth.*)$' -count=1
ok github.com/kandev/kandev/internal/agent/remoteauth
ok github.com/kandev/kandev/internal/agent/agents

go test -trimpath -race ./internal/agent/settings/controller ./internal/agent/runtime/environment -count=1
ok

bash scripts/lint-go-changed <changed-go-files>
0 issues.
```

Limits: no provider network authentication, real Anthropic credential, or live deployment was exercised.
