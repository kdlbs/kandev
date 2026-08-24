---
spec: docs/specs/cli/requirements/native-kandev-cli.md
related_spec: docs/specs/integrations/requirements/github-authentication.md
decision: docs/decisions/2026-07-31-system-service-user-continuity.md
created: 2026-07-31
status: complete
---

# Fix Plan: Preserve Service Identity and Harden Managed Checkout Recovery

## Overview

Release `0.83.0` began reconciling the `origin` URL of every Kandev-managed GitHub checkout during
Local and Worktree repository preparation. A reported Homebrew system installation then failed to
launch both old and new tasks with `set repository origin: exit status 128`.

The repair addresses both layers exposed by that incident:

1. Preserve the established system-service account across reinstall and update commands, reject
   silent identity drift, and validate the service-home ownership boundary before restart.
2. Make managed checkout origin reconciliation idempotent, single-pass on resume, and capable of
   returning credential-safe Git diagnostics instead of a bare exit code.

No database migration, frontend state, or executor protocol change is required.

## Confirmed Root Cause

The reporter's system service resolved to root, while `/var/lib/kandev`, its managed repository,
`.git`, and `.git/config` were all owned by `brewuser`. Git run as root under Kandev's intentionally
sanitized configuration rejected the checkout as dubious ownership and returned status 128.

The update sequence can produce that state because `service install --system` currently derives
the account from the installer process: a command invoked with `sudo` can select `SUDO_USER`, while
the same command from a root login selects root. Reinstallation rewrites the service definition but
does not change the existing data owner.

Release `0.83.0` exposed the latent mismatch because `repoclone.Cloner.SetOriginURL` now executes
`git remote set-url` during repository preparation. The operation discards Git's combined output,
so the user sees only `exit status 128`. The resume path also prepares the primary repository in
`applyResumeRepoConfig`, resolves all repositories again for multi-repository configuration, and
resolves them once more for credential routing, repeating the mutation boundary.

Smallest reproductions:

- Run Git with `GIT_CONFIG_NOSYSTEM=1` and `GIT_CONFIG_GLOBAL=/dev/null` as root against a checkout
  owned by another UID: Git emits `fatal: detected dubious ownership in repository` and exits 128.
- Leave `.git/config.lock` in a managed checkout and call `SetOriginURL`: the current application
  error is the same opaque `set repository origin: exit status 128`.

## Design

### 1. Durable system-service identity

- Add `--run-as <user>` to native `service install --system`; reject it for user services and all
  non-install actions.
- Resolve the target account using ADR-2026-07-31: explicit flag, existing root-controlled managed
  unit/plist, non-root `SUDO_USER` on first install, otherwise an actionable failure requiring an
  explicit account.
- Parse only a Kandev-managed service definition. A missing systemd `User=` or launchd user key is
  the explicit root identity of an existing definition.
- Treat service-owned `install.json` as corroborating metadata, never as privileged authority.
- Resolve the selected account to a UID. If the system home is missing or has another owner UID,
  fail before writing or restarting the service with pre-create/ownership guidance. Do not chown
  recursively or add Git trust exceptions.
- Persist the effective account in the generated definition and install metadata.

### 2. Idempotent, diagnosable origin reconciliation

- Keep the existing per-repository serialization in `repoclone.Cloner`.
- Inspect `remote.origin.url` under the same sanitized Git environment. If it exactly matches the
  desired canonical URL, return without writing `.git/config`.
- Never log or return the successful current URL. On failure, bound the diagnostic and redact known
  authentication material plus HTTP(S) URL userinfo before wrapping it.
- Classify Git's dubious-ownership message into a stable ownership-specific error that advises
  checking the Kandev service account and managed checkout owner. Keep generic sanitized context
  for failures such as `config.lock`, missing repositories, or malformed configuration.
- Continue failing closed. Do not retry with `safe.directory`, a different user, or a stale origin.

### 3. One repository-resolution pass per resume

- Resolve all task repositories once in `buildResumeRequest` and pass that result to primary
  repository configuration, multi-repository request construction, and GitHub credential routing.
- Select the persisted session primary repository from that resolved set by repository ID. Preserve
  the current fallback for older sessions and resolve a persisted repository separately only when
  it is legitimately absent from the task attachment set.
- Ensure each attached managed repository reaches `ensureRepoLocalPath` at most once per resume.
  Initial launch already uses one resolved set and remains behaviorally unchanged.
- Preserve repository ordering, base/checkout branch metadata, repo-less tasks, inherited task
  environments, user-managed local checkout behavior, and credential snapshot semantics.

## Tests

### Native service installer

- Table-test argument validation and first-install identity precedence, including explicit root.
- Render Linux/macOS definitions with `--run-as` and prove reinstall without the flag preserves the
  existing managed account when the current installer user differs.
- Prove untrusted service metadata cannot override the root-controlled definition.
- Prove owner-UID mismatch fails before the service file write/restart boundary and reports both
  identities; a matching pre-created home continues.

### Repository clone boundary

- Real-Git test that an already-canonical origin does not rewrite `.git/config` (mtime/content or an
  injected command boundary), while a changed origin still updates it under the existing lock.
- Classifier tests for dubious ownership, stale `config.lock`, bounded output, URL-userinfo
  redaction, and known-secret redaction.
- Preserve concurrent-update serialization and invalid-input coverage.

### Resume orchestration

- Instrument repository preparation and assert one call per attached repository during resume,
  including a multi-repository task.
- Cover persisted session primary selection, an older-session fallback, a repo-less task, and the
  exceptional persisted repository not present in task attachments.
- Retain the existing managed/executor transport transitions and user-managed-local no-op tests.

## Documentation

- Update `docs/public/run-as-a-service.md` with identity preservation, `--run-as`, first root-shell
  install behavior, ownership-mismatch recovery, and the explicit no-auto-chown policy.
- Update `docs/public/cli.md` with the new flag and valid command combinations.
- Update `docs/public/integrations.md` to explain actionable managed-checkout ownership errors
  without recommending broad Git trust overrides.
- Run both public-doc validators.

## Implementation Tasks

- [x] [task-01-preserve-system-service-user](task-01-preserve-system-service-user.md) — done
- [x] [task-02-harden-origin-reconciliation](task-02-harden-origin-reconciliation.md) — done
- [x] [task-03-resolve-resume-repositories-once](task-03-resolve-resume-repositories-once.md) — done
- [x] [task-04-document-service-identity-recovery](task-04-document-service-identity-recovery.md) — done

Tasks 01 and 02 have disjoint production boundaries and no dependency on each other. The primary
conversation should execute them sequentially unless the user explicitly authorizes delegation.
Task 03 depends on task 02's finalized origin-reconciliation contract. Task 04 follows all behavior
tasks so public wording matches the implemented CLI and errors.

## Validation Strategy

Run task-focused RED/GREEN commands first, then:

```bash
set -euo pipefail
(
  cd apps/backend
  go test ./internal/launcher ./internal/repoclone ./internal/orchestrator/executor -count=1
  make test
  make lint
)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Recorded Results

- Backend focused tests: `cd apps/backend && go test ./internal/launcher ./internal/repoclone ./internal/orchestrator/executor -count=1` passed.
- Backend verification: `cd apps/backend && make test` and `cd apps/backend && make lint` passed.
- Public documentation: `node --test scripts/validate-public-docs.test.mjs` passed 58 tests; `node scripts/validate-public-docs.mjs` validated 41 published pages.
- Operator migration caveat: an existing system home owned by a different account must be reconciled explicitly before reinstall; Kandev does not recursively chown it or add Git trust exceptions.

No browser E2E is planned because the change adds no UI state or interaction. Existing backend
error propagation displays the improved sanitized message in the current session error surface.

## Risks and Out of Scope

- Hand-edited or drop-in-extended service definitions may be ambiguous. Preservation is limited to
  definitions that Kandev can identify and parse safely; ambiguity fails closed.
- UID/name changes managed outside Kandev may require operator reconciliation. Kandev will not
  infer equivalence from filesystem access or ACLs.
- Automatic recursive ownership repair, ACL management, Windows services, global/per-repository
  `safe.directory` mutation, and privilege dropping for individual Git subprocesses are out of
  scope.
- The fix does not weaken Kandev's isolated Git configuration. Ambient global/system Git config
  remains intentionally unavailable to repository-clone subprocesses.
- User-managed local repository origins remain outside Kandev's mutation boundary.
