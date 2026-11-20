---
id: "04-task-git-access"
title: "Enterprise task Git and CLI access"
status: complete
wave: 4
depends_on: ["03-provider-workflows"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-ENTERPRISE-004
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-004.1
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-004.2
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-004.3
  - AC-INTEGRATIONS-GITHUB-ENTERPRISE-004.4
system_design:
  - ../../specs/integrations/system-design/github-enterprise-hosts.md
---

# Task 04: Enterprise task Git and CLI access

## Summary

Make saved Enterprise credentials usable by managed tasks and preserve explicit executor inheritance. Complete broker, clone, shim, launch/resume, and prepared-workspace paths without widening credential authority.

## In scope

- Match GitHub broker scopes to connection host, repository identity, and generation; retain destination binding and renewable App installation tokens.
- Set managed `GH_HOST` and exactly the correct token family, scrub unrelated environment credentials, and validate explicit CLI targets before child launch.
- Generalize managed checkout URLs and per-host protocol resolution while preserving user-managed origins and SSH transport rules.
- Correct GHE.com classification in the executor host CLI bridge and validate launch/resume/prepared workspace replacement.
- Audit active container/remote setup paths for public-only rewrites and credential-bearing URLs; preserve executor network/trust ownership.

## Out of scope

No automatic CA distribution, SSH alias inference, new executor types, arbitrary agent network sandbox, or changes to unrelated runtime download hosts.

## Acceptance

1. Real Git subprocess and fake CLI tests prove clone/fetch/push credential lookup, CLI routing, renewal, and rejection of foreign host/path/generation on all three host families.
2. Local/Worktree inheritance uses current host protocol and proper public/GHE versus GHES explicit-token precedence; remote paths receive only their authorized managed/executor contract.
3. Launch, resume, and prepared reuse replace stale generated configuration without accumulating helpers or rewriting user-managed checkouts.

## Verification

Start at repository root. Read scoped AGENTS.md and use `/tdd` for changed
logic. New worktrees first run `(cd apps && pnpm install --frozen-lockfile)`.
Backend changes also run `bash scripts/lint-go-changed` with the explicit changed
Go file paths; frontend changes run ESLint on the explicit changed TS/TSX paths.
Record the expanded commands in Results. Do not mark skipped database tests as passed.

```bash
(cd apps/backend && go test ./internal/github -run 'TestEnterpriseBrokerScope|Test.*CredentialBroker' -count=1)
(cd apps/backend && go test ./internal/gitcredentials ./internal/githubauth ./internal/repoclone -count=1)
(cd apps/backend && go test ./cmd/agentctl -run 'Test.*(GitHubCLI|GitHubCredential|Enterprise)' -count=1)
(cd apps/backend && go test ./internal/orchestrator/executor -run 'Test.*(HostGHBridge|ExplicitGitHubTokens|HostGitHub|Enterprise)' -count=1)
(cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'Test.*(GitHub|Credential|Enterprise|Clone|Origin)' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/github/credential_broker.go` and its tests.
- `apps/backend/internal/gitcredentials/identity.go`, `apps/backend/internal/githubauth/environment.go`, and `apps/backend/internal/repoclone/clone.go`, `protocol.go`.
- `apps/backend/cmd/agentctl/github_cli_shim.go`, `github_credential.go`, and associated tests.
- `apps/backend/internal/orchestrator/executor/executor_host_gh_bridge.go`, `apps/backend/internal/backendapp/git_credentials.go`, and their tests if identity wiring changes.
- `apps/backend/internal/agent/runtime/lifecycle/container.go`, `default_scripts.go`, `executor_sprites_helpers.go`, and applicable lifecycle tests.

## Dependencies

03-provider-workflows. Follow the plan's acceptance order and authorized implementation lanes.

## Risks

GHE.com uses `GH_TOKEN`, not `GH_ENTERPRISE_TOKEN`. An explicit CLI target can override `GH_HOST`; compare it to authorized scope before passing a token. Add the backendapp package test command if that wiring changes; do not claim filtered tests cover newly modified suites they do not discover.

## Parallelism

Parallel-safe only within the explicitly owned implementation lane in plan.md; integration acceptance remains dependency-ordered.

## Inputs

- [Requirements](../../specs/integrations/requirements/github-enterprise-hosts.md), using the exact IDs above.
- [System design](../../specs/integrations/system-design/github-enterprise-hosts.md) and [host/OAuth ADR](../../decisions/2026-10-14-github-enterprise-host-boundaries.md).
- Existing source and neighboring tests named in Files likely touched; the plan's test matrix defines new regression names.

## Results

Implementation is complete and verified. Regression tests reproduced the public-only broker guard,
ambient CLI host/token leakage, wrong GHE.com token precedence, and initial clone
credential resolution that dropped the requested host. These boundaries are now
implemented with passing focused tests. Managed Git also strips
`GITHUB_ENTERPRISE_TOKEN`; HTTPS web ports no longer become inferred SSH ports.

Passed (all Go commands run from `apps/backend` with
`GOMAXPROCS=2 /usr/local/go/bin/go test -trimpath -p 1`):

- `./cmd/agentctl -run 'Test.*(GitHubCLI|GitHubCredential|Enterprise)' -count=1`.
- `./internal/github -run 'TestEnterpriseInitialCloneCredentialHost|TestEnterpriseBrokerScope|TestResolveGitCredential' -count=1`.
- `./internal/backendapp -run 'TestEnterprise|TestGitHubBrokerScope|TestGitCredentialBrokerFactory|TestRepositoryCloneCredentialProvider' -count=1`.
- `./internal/gitcredentials ./internal/githubauth ./internal/repoclone -count=1`.
- `./internal/orchestrator/executor -run 'Test.*(HostGHBridge|ExplicitGitHubTokens|HostGitHub|Enterprise)' -count=1`.
- `./internal/agent/runtime/lifecycle -run 'Test.*(GitHub|Credential|Enterprise|Clone|Origin)' -count=1`.
- `./internal/repoclone -run TestEnterpriseRealGitCloneFetchPush -count=1`: real
  Git clone/fetch/push plus revocation against a disposable HTTPS Git server,
  routed through a test-only CONNECT proxy for public, GHE.com, and GHES names.
  This is fixture evidence, not a live GitHub provider check.

Runtime audit: managed remote clones already receive explicit HTTPS origins and
broker environment; legacy public rewrite scripts and explicit public profile
credentials do not route Enterprise managed credentials. Existing lifecycle
checks cover remote helper propagation and broker reconnect refresh.

Runtime changed-Go lint passed with zero issues for `cmd/agentctl`, `repoclone`
and `orchestrator/executor` using `PATH=/usr/local/go/bin:$PATH GOMAXPROCS=2 bash
scripts/lint-go-changed` and the changed runtime Go paths. CLI target validation
was split into origin and path checks to satisfy complexity limits. The final combined changed-Go lint, including backendapp and GitHub, passed
with zero issues.

Additional integration evidence: the complete `cmd/agentctl` suite passed;
`internal/gitcredentials`, `internal/githubauth` and `internal/repoclone` also
passed under `-race`. A broader `backendapp` run exposed a custom HTTP transport
constructor panic; the transport owner corrected it. The rerun
`./internal/backendapp -run 'Test.*(GitHub|GitCredential|RepositoryClone|Enterprise)'
-count=1` passed in 121.540 seconds. This is affected-route evidence, not a passing
full backendapp run.

Final clone-boundary regressions reject a foreign repository path before credential
resolution, even on the same host. Remote branch-source reads bind the selected
host before fetching and recheck the captured workspace generation before returning.
Focused backendapp, GitHub host-boundary and workflow-sync tests passed after these
integration changes.
