---
id: "02-acceptance-delivery"
title: "Validate private Git and deliver reviewed PR"
status: done
wave: 2
depends_on:
  - "01-configure-handoff"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-KUBERNETES-TASK-POD-001
acceptance_criteria:
  - AC-EXECUTORS-KUBERNETES-TASK-POD-001.4
  - AC-EXECUTORS-KUBERNETES-TASK-POD-001.9
  - AC-EXECUTORS-KUBERNETES-TASK-POD-001.10
system_design:
  - ../../specs/executors/system-design/kubernetes-task-pod.md
---

# Task 02: Disposable acceptance and PR delivery

## Summary

Validate the implemented handoff with actual private Git operations from the
agent subprocess, then publish the isolated branch and follow CI/review fixups.
Report exact head, results and any remaining acceptance blockers. Do not merge.

## In scope

- Inspect and adapt the historical receipts and harness under
  `/tmp/intel-git-evidence-20260924`; never replay hardcoded deleted resources.
- Reproduce current-base live behavior first if access permits, then repeat the
  same fixture with backend and agentctl built from the fixed head.
- Use an isolated temporary home/database/ports and disposable cluster namespace,
  task, workspace, profiles and claims. Explicitly select managed task Git access
  with a `gh_cli` connection. Never set `KANDEV_E2E_MOCK=true`.
- Use the supplied pinned worker digest, non-root UID 1000, bounded resources,
  dedicated scheduling constraints and disabled service-account token mounting.
  Keep the HTTPS broker route restricted and validate its certificate with a
  temporary trusted CA; no insecure TLS switches.
- Ask the actual agent to run noninteractive `ls-remote` and a shallow fetch of
  private `zeval/intelligence`, first fresh, then after Stop/Resume. Record the
  dynamically observed advertised and fetched commits, not an old expected head.
- Verify same retained Pod/PVC UIDs, foreign `zeval/koi.git` helper denial,
  broker failure/revocation behavior, and presence-only subprocess diagnostics.
  Assert raw `GH_TOKEN`/`GITHUB_TOKEN` absence. Never print helper credentials.
- Tear down only owned resources, processes, credential copies, CA keys and
  temporary data. Preserve sanitized command/result receipts.
- Update public executor/Git documentation only for verified behavior and keep
  this package's status/results accurate. Load commit, PR and PR-fixup skills;
  retain hooks and the repository PR template. Resolve actionable reviews and CI
  failures and report the exact final head. No merge, rollout or deployment.

## Out of scope

Main Kandev service, main executor profiles, previous #3835 worktree, production
workspaces and credentials, scheduled automation and unrelated acceptance gaps.

## Acceptance

1. Real agent private reads/fetches succeed fresh and resumed, retaining expected
   resource identities; foreign-repository redemption fails with verified TLS.
2. Sanitized evidence identifies tested commit, resource continuity and cleanup;
   unavailable live prerequisites are explicitly reported rather than marked passed.
3. PR is open with required checks and review findings dispositioned on the exact
   head; task remains unmerged and undeployed.

## Verification

The disposable harness must be adapted after inspection; record its exact
command, environment contract, instance identifiers and cleanup commands before
running it. Current historical scripts are not an executable acceptance recipe.
Run these documentation gates from the repository root:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git diff --check -- docs/plans/kubernetes-managed-git-handoff
git status --short -- docs/plans/kubernetes-managed-git-handoff
```

After creating the PR, use `scripts/pr-await <PR>` for the authorized wait and
`scripts/pr-resolve list <PR>` for findings. Follow the loaded PR-fixup skill for
exact-head checks and actionable remediations. Backend fixups also run the
repository-required full changed-code lint against the actual PR base:
`golangci-lint run ./... --new-from-rev="<base-sha>" --timeout=5m` from `apps/backend`.

## Files likely touched

- `docs/public/executors.md` and `docs/public/git-operations.md`, if clarification is needed.
- This plan and its work orders, with actual final results.
- Only implementation files required by valid CI/review findings.

## Dependencies

Task 01 passing targeted validation.

## Risks

Private credentials and cluster access may be unavailable. Do not substitute
mock-token success or preparation-only success for actual-agent acceptance.
Any blocker must identify the missing prerequisite without exposing a secret.

## Parallelism

`sequential`

## Inputs

- User-supplied acceptance report and sanitized historical evidence.
- [Plan](plan.md), task 01 results and current repository delivery skills.

## Results

Live acceptance completed with real Codex in a disposable Kubernetes cluster,
using namespace
`git-handoff-o7u9z451`, using the pinned worker digest specified above.

- Built backend and agentctl from base `d60274528129dc17d413357c104c9c427c4e7af9`.
  Fresh and retained-pod resumed agent commands failed while preparation passed.
- Built backend and agentctl with this repair. Actual fresh and resumed agent
  commands both passed private `ls-remote` and shallow fetch. Both observed
  `59d1df9585e70556ca1477a3a91ef12f4b94bd0a` as advertised and fetched HEAD.
- Fixed Pod UID `3bd45feb-b1e4-4946-91ed-bc2d8bb98284` and PVC UID
  `07876735-30e8-4270-b8a2-281c66d90d9b` were unchanged across Stop/Resume.
- Foreign-repository helper lookup returned no credentials. Real Codex environment
  contained broker fields but no raw `GH_TOKEN`/`GITHUB_TOKEN`. A helper call with
  an untrusted CA failed without credentials. Deleting only the disposable
  workspace's GitHub connection also caused the existing lease to fail.
- Archived disposable tasks, deleted the namespace and all owned PVs, stopped
  the owned backend/proxy, and deleted the private home, copied credentials,
  kubeconfig, TLS key and databases. Main instance/profiles were not modified.

Adapted harness commands were run from
`/tmp/kandev-managed-git-handoff-20260924/`: `python3 setup.py`, `cluster.py`,
`launch.py base`, `register.py`, `start.py base`, `control.py base stop`,
`control.py base resume`, `control.py base message`, `switch.py`, `launch.py fixed`,
`start.py fixed`, and the matching fixed stop/resume/message commands. `status.py`
recorded sanitized receipts; `security-probe.py` ran inside the discovered Pod
for untrusted-CA and connection-revocation checks. `cleanup.py` verified teardown.
These scripts used loopback backend port 18347, Tailscale TLS port 18348, an
isolated credential home and explicit managed policy; no mock profile was enabled.
The exact scripts and sanitized JSON receipts remain in that evidence directory.
Private resource state and logs were deleted during cleanup.

Public docs: executor guide updated, no UI screenshots required. Public docs
validator passed for 47 pages; its 62 tests passed. Specification catalog,
specification lint and diff whitespace checks passed.

PR [#3909](https://github.com/kdlbs/kandev/pull/3909) was published at
`653520d6ad8af040b1c88dd087be7e1d6fc7bb64` with normal commit hooks passing.
The five initial review threads identified generated path-setting cleanup,
legacy helper ownership, key casing and direct-test coverage; the follow-up
fixes and regression results are recorded in task 01. CodeRabbit's docstring
coverage warning is informational: it reports no correctness finding and is
not a repository-required coverage gate. All five threads were replied to with
commit-specific evidence and verified resolved after the fixup push. The PR
remains open and is not merged or deployed.

Review-fixup validation: the full five-package race command passed, as did the
focused Configure-mode regression after adding the generated path setting.
Full backend changed-code lint passed with zero issues using
`golangci-lint run ./... --new-from-rev=5cb908e4adf53fc6bca9727dc4f3a7d534ee38fd --timeout=5m`.
A preceding attempt encountered missing shared Go cache export data, and the
first retry timed out; the final warm-cache run exited successfully.
Specification catalog validation, specification lint and whitespace checks passed.

### Delivery completion

- Initial head `653520d6ad8af040b1c88dd087be7e1d6fc7bb64`: 57 successful or
  skipped checks, no failed or pending checks.
- Review-fixup head `bfd990f9604d471e39d2f979d371f0c642ff2e99`: 59 successful
  or skipped checks, no failed or pending checks, zero unresolved review threads,
  and GitHub reported `MERGEABLE` / `CLEAN`. The all-terminal waiter exited 1
  solely for base drift, not a failed check or unresolved finding.
- Both implementation commits passed normal commit hooks without bypasses.
  Post-commit race regressions passed after the fixup.
- Synthetic merge `c59af61cb9c633dece6a337e808eeb7f600d6f00` combined the
  fixup with base `5cb908e4adf53fc6bca9727dc4f3a7d534ee38fd`; focused race
  checks across lifecycle, process, executor, agents and githubauth passed.
  Later base/head changes require renewed merge-result validation; the PR's
  final delivery receipt records that exact pair.
- CodeRabbit skipped incremental review by repository configuration. Historical
  aggregate findings were audited, and no additional actionable finding remained.

These are immutable validation snapshots. The documentation-completion commit
and any later PR head must pass their own required checks before handoff. Final
head, base, synthetic-merge and check receipts are recorded in the PR delivery
summary and the Kandev task plan, avoiding a self-referential commit identifier.

### CI follow-up: LSP release ordering

The documentation-only head `66cbe702645a58b13cbd9dac61d74ca9068cb294`
exposed a backend CI failure in run `36025037375`, job `107720501859`:
`TestLSPContinuityReconnectsToSameTaskHostStream` received close 4009 instead
of the release acknowledgment. Twenty focused race repetitions reproduced the
release race and an early fence assertion; the release implementation was
identical on the previously green head and current main.

A deterministic regression blocked browser writes, completed LSP shutdown/exit
and closed the fake upstream. Both Stop and editor-idle cases failed because
the upstream reader attempted browser termination before acknowledgment. The
release operation now owns expected upstream EOF and always finishes teardown,
including on acknowledgment failure. The original test checks fence release
after the first serviced browser request instead of racing admission's ready
notification. This remediation enforces the existing
[Stop contract](../../specs/platform/requirements/lsp-file-intelligence.md) and
[release design](../../specs/platform/system-design/lsp-file-intelligence-01.md);
it does not alter managed Git behavior or add a new user-facing feature.

Validation after remediation:

- `go test -race ./internal/gateway/websocket -run 'TestLSP(GracefulRelease.*|ContinuityReconnectsToSameTaskHostStream)$' -count=20`: passed.
- `go test -race ./internal/gateway/websocket -count=3`: passed.
- Full backend `golangci-lint run ./... --new-from-rev=9fae7ab50045990e1bec2d363be3c553f268946c --timeout=5m`: passed, zero issues.
- Catalog/spec lint and whitespace checks: passed.

### Current-main conflict resolution (2026-09-26)

Merged main `5c3dc31b2b65b35ff3121f3b5400c6c99dfb67d9` into the PR branch.
Upstream independently fixed expected LSP closure and synchronized its admission
fence test. Retained upstream's `expectedUpstreamClose` field and fence test,
with this PR's release-owner teardown and acknowledgement-failure regression.
Upstream now observes co-residency once through the shared process-start hook;
its synchronized one-observation test replaces the earlier two-observation
fixture. The managed Git configuration boundary and removal regressions remain
intact. No durable requirements changed during conflict resolution.

Post-merge validation from `apps/backend`:

- `go test -race ./internal/agent/runtime/lifecycle ./internal/agentctl/server/process ./internal/orchestrator/executor ./internal/githubauth ./internal/gitconfigenv ./internal/gateway/websocket`: passed.
- Specification catalog/lint, public documentation tests/validator and whitespace checks: passed.
- `go test -race ./internal/gateway/websocket -run 'TestLSP(GracefulRelease.*|ContinuityReconnectsToSameTaskHostStream)$' -count=20`: passed.
- `golangci-lint run ./... --new-from-rev=5c3dc31b2b65b35ff3121f3b5400c6c99dfb67d9 --timeout=5m`: passed, zero issues. The first run timed out loading packages; the post-compilation retry passed.
- Hooked-commit and new-head CI results are recorded in the PR description and task plan. Earlier live acceptance remains historical;
  no main service/profile changes or new live resources were needed.
