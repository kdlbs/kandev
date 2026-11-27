---
id: "01-preserve-binding"
title: "Preserve Kubernetes MCP binding ownership"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-BUILTIN-MCP-003
acceptance_criteria:
  - AC-AGENTS-BUILTIN-MCP-003.1
  - AC-AGENTS-BUILTIN-MCP-003.2
  - AC-AGENTS-BUILTIN-MCP-003.4
system_design:
  - ../../specs/agents/system-design/builtin-mcp-management.md
---

# Task 01: Preserve Kubernetes MCP binding ownership

## Outcome and boundaries

Keep helper registration, creation profile, and execution metadata consistent
through Kubernetes recovery. Change the existing lifecycle and client packages,
their regressions, and the owning documentation. No credential-delivery fix,
new authority policy, schema, protocol, UI, or deployment is included.

## Acceptance

1. Authenticated retained helpers keep their original binding and creation
   profile. Foreign task/session owners and malformed bindings refuse without
   recreating a helper or mutating the caller's desired metadata.
2. A normal launch with a recreated helper uses its prepared selection. Shared
   task inventory cannot overwrite it or grant a sibling session's binding.
3. Active transport and container recovery preserve the current admitted
   binding across repeated cycles. Refusal diagnostics identify the stage and
   binding mismatch without exposing raw profiles, credentials, or error bodies.

## Verification

Run from the repository root, using the repository toolchain:

```sh
(cd apps/backend && go test -race -trimpath -tags fts5 ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl -run '^Test(Kubernetes|GetOrCreateSharedKubernetes|Builtin|Standalone.*Builtin|IdleActivityRecovery)' -count=1)
git ls-files --modified --others --exclude-standard -z -- 'apps/backend/**/*.go' | xargs -0 bash scripts/lint-go-changed
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

On macOS, use a canonical temporary directory such as `TMPDIR=/private/tmp`
for Git mount-root fixtures. Formatting, typechecking, tests, and lint use the
existing Make targets. Record exact broad-check gaps rather than inferring
whole-repository success from focused results.

## Lifecycle evidence and results

Regression tests cover retained helper adoption, new helper creation, rejected
owners, invalid bindings, and environment/session separation. They also cover
retained A with desired B followed by active restart, then admitted B followed
by transport refresh and another restart. The old behavior failed these
assertions before correction.

A disposable Kind v0.33.0 / Kubernetes v1.36.1 cluster on Linux ARM64 ran real
Kubernetes resource/stream clients and agentctl helpers in both ownership modes.
Actual MCP initialize and tools/list passed after retained A, active container
restart preserving A, normal recreation with B, transport recovery preserving B,
and another container restart preserving B. Task idle StopInstance preserved
Pod/PVC UIDs before helper recreation. Final canary exited zero in 80.974s,
including goroutine-leak checks; the dedicated cluster was removed.

No ACP child or model was launched. Full policy timer, controller suspension,
provider conversation continuation, and resume were not exercised as one live
chain. Existing focused orchestrator selection/admission tests passed separately;
those upper-layer mechanisms are unchanged. Stale/duplicate work and prompt
ownership fences remain governed by the existing idle-activity regressions.

On upstream base `5b41bbde6b6d91462fc2ed253488f9a50f82e209`, the focused
race command passed for lifecycle (14.877s) and agentctl (1.535s). Repository
formatting, generated web inputs, full TypeScript typechecking, documentation
catalog/spec lint, all 63 public-doc validator tests, all 56 public pages, and
the deterministic PR documentation-coverage validator passed. Typechecking used
`NODE_OPTIONS=--max-old-space-size=8192` after the normal generated-input step.

Broad `make test` reported failures and was stopped; it did not pass. Plugin
fixture TLS, launcher containment, and missing-checkout failures reproduced with
matching errors on clean upstream main. Worker preparation passed both baseline
and an isolated candidate rerun after its broad-run failure. Other broad failures
remain unclassified; no whole-repository pass is claimed. Broad `make lint` was
stopped before a result. Changed-package lint remains the scoped verification;
normal commit hooks must pass before publication. Hosted CI, upstream integration,
and deployment remain unverified.
