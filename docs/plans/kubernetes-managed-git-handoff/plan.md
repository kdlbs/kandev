---
created: 2026-09-24
status: done
requirements:
  - REQ-EXECUTORS-KUBERNETES-TASK-POD-001
system_design:
  - ../../specs/executors/system-design/kubernetes-task-pod.md
legacy_specs: []
---

# Kubernetes managed Git credential handoff

## Overview

Repair the managed Git environment delivered to fresh and resumed Kubernetes
agent subprocesses. Implement the configuration-boundary regressions and fix
first, then validate private Git on a disposable instance and deliver the PR.
The executor system owns the failed process-environment handoff; provider
credential policy remains owned by integrations.

## Scope

Preserve current leases until an explicit per-run replacement is supplied,
normalize the Kubernetes helper path, and retain stale-credential removal,
repository isolation and TLS verification. Cover fresh launch, prepared workspace,
retained-pod Stop/Resume, and restarted-agentctl configuration.

No main service/profile changes, deployment, merge, token-in-image workaround,
provider-policy redesign, schema change, or UI change. Preserve the previous
`fix/kubernetes-resume-profile-env` worktree and merged PR #3835.

## Evidence and root cause

Base: `d60274528129dc17d413357c104c9c427c4e7af9`, fetched current `origin/main`.
Worktree: `/home/zeval/repositories/kandev-kubernetes-managed-git-handoff`.
Branch: `fix/kubernetes-managed-git-handoff`.

Prior live evidence is recorded in the supplied intelligence acceptance report
(commit `59d1df9585e70556ca1477a3a91ef12f4b94bd0a`) and sanitized
`/tmp/intel-git-evidence-20260924`. That runtime exercised `3ebd7381b`, not
this base. Preparation succeeded but actual fresh and resumed Codex commands
lacked the broker URL, lease and helper path.

On the current base, `ToAgentExecution` snapshots `req.Env`, while
`configureAndStartAgent` always composes that snapshot with metadata.
`runtimeEnvFromMetadata` collapses absence to an empty map and composition
unconditionally strips managed fields. Agentctl then intentionally clears its
inherited fields before applying this incomplete request. Kubernetes bootstrap
and session environment builders normalize the helper path, but that normalized
copy is not the request captured by `ToAgentExecution`. Existing-workspace
refresh similarly needs normalization after `SetExecutionEnv` composition.

The temporary reproduction `TestReproKubernetesManagedGitConfigureHandoff`
uses `ToAgentExecution`, `SetExecutionEnv` and the real configure HTTP client
with synthetic broker values. Record its exact result below. This is boundary
reproduction, not current-base live Kubernetes acceptance.

## Technical approach

Follow the owning design's absent-versus-explicit replacement semantics at the
lifecycle caller; retain the existing compose helper's stale-secret removal
contract. Normalize the final Kubernetes environment before snapshot and delivery.
Audit restarted-agentctl delivery and initial/existing-workspace orchestrator
credential producers. Remove owned managed helper/reset entries during credential
replacement without removing user entries. Keep agentctl cleanup authoritative.

## Tests

- AC .9: permanent lifecycle regression in `manager_managed_git_handoff_test.go`
  covering launch capture, current managed refresh, normalized helper and restart.
- AC .10: explicit nil/empty/partial replacements, replacement lease, no raw tokens,
  preserved user Git config, malformed configuration blocks startup, and both
  existing obsolete-managed-credential removal tests.
- Actual Git subprocess coverage through the agentctl configuration boundary,
  using deterministic fake helper/broker fixtures and a denied foreign repository.

## End-to-end acceptance

Task 02 owns real private `ls-remote` and shallow fetch on fresh and retained-pod
Stop/Resume against a disposable backend built from the tested head. Check Pod/PVC
UID continuity, foreign-repository denial, verified TLS, token absence and cleanup.
No browser layout change requires Playwright screenshots.

## Work orders

- [x] [Task 01: Repair and regress the configuration handoff](task-01-configure-handoff.md)
- [ ] [Task 02: Disposable acceptance and PR delivery](task-02-acceptance-delivery.md)

## Verification results

Design-checkpoint evidence (before implementation):

- `pnpm install --frozen-lockfile`: passed in the new worktree.
- Temporary `TestReproKubernetesManagedGitConfigureHandoff` on the exact base:
  fresh launch failed because broker URL was empty; existing-workspace refresh
  failed because helper was `/host/bin/agentctl` instead of `/opt/kandev/agentctl`;
  explicit removal passed. Command:
  `(cd apps/backend && go test ./internal/agent/runtime/lifecycle -run '^TestReproKubernetesManagedGitConfigureHandoff$' -count=1)`.
- Sanitized source and failure receipt retained outside the repository at
  `/tmp/kandev-managed-git-handoff-20260924/`; temporary test removed from source.
- `python3 scripts/list-docs.py validate`: passed (304 decisions, 1136 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- Existing removal guards passed on the base:
  `(cd apps/backend && go test ./internal/agent/runtime/lifecycle -run '^TestComposeExecutionRuntimeEnvironmentRemovesObsoleteManagedCredentials$' -count=1)`
  and `(cd apps/backend && go test ./internal/agentctl/server/process -run '^TestManagerConfigureRemovesObsoleteManagedCredentialEnvironment$' -count=1)`.
- `git diff --check`: passed.
- At the design checkpoint, only synthetic reproduction had run. Implementation
  and live results are recorded below.

Public documentation was unchanged at the design checkpoint. Task 02 adds
the verified public clarification. The previous repair package remains an
accurate record of #3835; this package does not mark it incomplete or rewrite
its historical results.

## Risks

- Treating absence as removal loses valid launch credentials; treating an explicit
  empty or partial replacement as absence revives stale credentials.
- Fixing only lifecycle fields can leave a host helper path or stale indexed helper.
- Current main has task-owned shared compute; historical harness scripts cannot
  be replayed without checking their resource ownership and setup assumptions.
- Live credential availability and cluster access remain to be checked; fake-token
  mock mode cannot establish real private-repository acceptance.

## Implementation validation

- Full targeted `go test` command in task 01: all five packages passed.
- Full targeted `go test -race` command in task 01: all five packages passed
  after the independently reproduced test-only co-residency correction.
- Changed-code Go lint command in task 01: passed, zero issues. The tool is
  `/home/zeval/go/bin/golangci-lint` (v2.9.0) on this host.
- Public docs tests: 62 passed; validator: 47 pages passed.
- Catalog validation: 304 decisions and 1136 specifications passed;
  specification lint and `git diff --check` passed.
- [Task 02 results](task-02-acceptance-delivery.md#results) record actual-agent
  fresh/retained-resume success, upstream failure, security negatives and teardown.
- Sanitized receipts and raw local check logs are under
  `/tmp/kandev-managed-git-handoff-20260924/`. No real credential values appear in
  the retained acceptance receipts. The private runtime directory was deleted.

Delivery completed in [PR #3909](https://github.com/kdlbs/kandev/pull/3909).
Implementation and review-fixup commits passed normal hooks. Fixup head
`bfd990f9604d471e39d2f979d371f0c642ff2e99` completed CI with 59 successful or
skipped checks, zero failures and zero pending checks; all five review threads
were addressed and resolved. See task 02 for the immutable delivery evidence.
No merge or deployment is authorized by this delivery package.
