---
created: 2026-10-22
id: "01-provision-credentials"
title: "Provision V2 session credentials"
status: done
wave: 0
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-OPENCODE-AUTH-001
acceptance_criteria:
  - AC-EXECUTORS-OPENCODE-AUTH-001.1
  - AC-EXECUTORS-OPENCODE-AUTH-001.2
  - AC-EXECUTORS-OPENCODE-AUTH-001.3
  - AC-EXECUTORS-OPENCODE-AUTH-001.4
  - AC-EXECUTORS-OPENCODE-AUTH-001.5
  - AC-EXECUTORS-OPENCODE-AUTH-001.6
system_design:
  - ../../specs/executors/system-design/opencode-v2-credentials.md
---

# Task 01: Provision V2 Session Credentials

Historical completion record for the 2026-10-22 implementation, before the
maintainer's review and subsequent rebase. Its counts and lint result describe
that revision only. Revised bootstrap semantics and fresh validation are owned
by Tasks 02 and 03; this record is not evidence that the review is resolved.

## Outcome and scope

Implement [the design](../../specs/executors/system-design/opencode-v2-credentials.md)
in the agent descriptor/catalog, lifecycle launch and Kubernetes refresh hooks,
and agentctl client/server/process boundary. Update the V2 default and public
executor reference. Preserve the V1 file method and stable selection identity.

This work order covers criteria `AC-EXECUTORS-OPENCODE-AUTH-001.1` through
`.6`. Tasks 02 and 03 cover the remaining criteria. Cache lifecycle,
volatile bootstrap repair, provider-history durability, demo configuration,
and PR submission are excluded.

## Acceptance conditions

1. Initial and subsequent session imports complete before ACP admission, with
   errors preventing launch and no export for unselected remote methods.
2. Repeated real-CLI import preserves destination credentials and active accounts.
3. Bounded secret transport, cancellation, and configuration/start exclusion pass
   regression and race checks; matching binaries build and run in an image.

## Verification commands

From `apps/backend`:

```sh
env LC_ALL=C LANG=C go test -tags fts5 -trimpath ./internal/agent/managedruntime ./internal/agent/agents ./internal/agent/remoteauth ./internal/agent/runtime/agentctl ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/lifecycle
env LC_ALL=C LANG=C go test -tags fts5 -race -trimpath ./internal/agent/managedruntime ./internal/agent/agents ./internal/agent/remoteauth ./internal/agent/runtime/agentctl ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/lifecycle -run 'OpenCodeCredential|OpenCodeV2|OpenCodeACP|ConfigureAndStartAgentProvisions|ConfigureAndStartAgentRejectsUnsupported' -count=1
go build -trimpath ./cmd/kandev ./cmd/agentctl
```

From the repository root:

```sh
bash scripts/lint-go-changed $(git diff --name-only -- '*.go') $(git ls-files --others --exclude-standard -- '*.go')
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Results

Completed checks:

- Affected-package tests: 8,879 passed initially; 57 existing Git diagnostic
  assertions failed under the host's Korean locale. Rerunning their owning test
  groups with `LC_ALL=C LANG=C` passed all 121 cases.
- Focused race run: 39 cases passed across all seven affected packages. The
  expanded lifecycle selection/failure test also passed under the race detector.
- Controller and agentctl Linux builds passed. Lifecycle/process Windows builds
  passed with `GOOS=windows GOARCH=amd64 CGO_ENABLED=0`.
- Specification/catalog and public-documentation validators passed.
- Disposable image `kandev-native-credentials:validation`, containing the rebuilt
  controller and agentctl, ran the new agentctl endpoint with real OpenCode
  2.1.2 and networking disabled. First and subsequent session HOMEs imported
  synthetic OAuth accounts; repeated and post-ACP-stop imports preserved refreshed
  tokens and active-account selection. An additional account stayed inactive.
  Captured worker diagnostics contained no synthetic tokens.

The image check exercises the worker's native provisioning and ACP start/stop.
Controller launch ordering is covered by lifecycle integration tests. No real
provider request or OAuth refresh exchange was attempted, and no demo deployment
or credential workaround was changed. Changed-package lint passed with zero
issues using Go 1.26.0 and golangci-lint 2.9.0. Self-review and `git diff --check`
passed. The final source revision passed the 39-case race run and rebuilt-image
check again after the lint-driven refactor.
