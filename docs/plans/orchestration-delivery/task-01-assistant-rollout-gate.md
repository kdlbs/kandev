---
id: "01-assistant-rollout-gate"
title: "Unified Orchestrator rollout gate"
status: done
wave: 2
depends_on: ["00-private-publication"]
plan: "plan.md"
requirements:
  - REQ-ORCHESTRATION-ASSISTANT-010
acceptance_criteria:
  - AC-ORCHESTRATION-ASSISTANT-010.1
  - AC-ORCHESTRATION-ASSISTANT-010.2
  - AC-ORCHESTRATION-ASSISTANT-010.3
  - AC-ORCHESTRATION-ASSISTANT-010.4
system_design:
  - ../../specs/orchestration/system-design/personal-assistant.md
---

# Task 01: Unified Orchestrator rollout gate

## Summary

Make coordinator daily use possible while unfinished assistant execution remains
off. This changes admission/launch behavior, so implement it with explicit
requirements and tests before building the first live candidate.

## In scope

1. Recheck active and retired flag identities; use typed `features.orchestration`
   and `KANDEV_FEATURES_ORCHESTRATION` for the complete Orchestrator. Default
   false in root and embedded prod/dev/e2e profiles; expose effective source
   through existing flag diagnostics.
2. Compute all Orchestrator capability enablement from the Orchestration flag. Apply it
   at composition and every assistant route, CLI/tool discovery, binding selection,
   intake admission, queued dispatch, callback/wake and native launch entry.
3. Enumerate all ingress paths in a test matrix. A valid old runtime token,
   persisted queue item or direct HTTP request cannot bypass disablement. An
   owned assistant conversation cannot fall back to ordinary coordinator launch.
4. Leave required-store construction, retained ownership, scoped history reads,
   context guards and queue provenance active with either/both flags off. Keep
   existing records intact; disabling is not deletion or migration rollback.
5. Add frontend flag typing/contract coverage and effective configuration copy.
   New assistant UI work must consume this flag. Existing coordinator routes/chat
   remain available under the same Orchestrator boundary.
6. Document restart semantics from the actual composition lifecycle. Re-enable
   only through current binding/account/context admission, not blanket queue replay.

## Out of scope

Assistant capability/authority implementation, hiding historical data from its
owner, Office feature changes and live enablement.

## Acceptance

- The full four-combination flag matrix passes via direct API and queued launch
  tests, with defaults off and visible effective configuration provenance.
- Coordinator chat/delegation works with assistant off; retained privacy/history
  and required schemas remain enforced with both flags off and after restart.
- Every enumerated assistant mutation/dispatch ingress refuses unavailable work;
  enabling restores only currently authorized operations.

## Verification

Add `TestAssistantFeatureGate*` cases for the entry matrix and regression cases
to the existing runtime-flag and retained-owner test suites. Names below must
select actual tests after implementation.

```bash
(cd apps/backend && go test -tags fts5 -count=1 ./internal/runtimeflags ./internal/common/config ./internal/profiles)
(cd apps/backend && go test -race -tags fts5 -count=1 ./internal/backendapp ./internal/orchestration/... ./internal/orchestrator/executor -run 'TestAssistantFeatureGate|Test.*(Ownership|Owner|OrchestrationFlag)')
pnpm --dir apps/web exec vitest run lib/state/slices/features/features-contract.test.ts lib/state/slices/features/features-slice.test.ts app/actions/features.test.ts
pnpm --dir apps/web run typecheck
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the existing coordinator browser specs together with the new feature matrix
scenario under the managed runner; preserve its one-worker limit.

## Files likely touched

- `apps/backend/internal/runtimeflags/registry.go`, related registry tests.
- `apps/backend/internal/common/config/config.go`, `profiles.yaml`,
  `apps/backend/internal/profiles/profiles.yaml`.
- `apps/backend/internal/backendapp/` composition/admission adapters.
- `apps/backend/internal/orchestration/runtime/` human/runtime routes and dispatch.
- `apps/backend/cmd/agentctl/` assistant command exposure and tests.
- `apps/web/lib/state/slices/features/`, `docs/features.md`.

## Dependencies

Delivery 00. Use the repository's runtime-feature-flags skill during implementation.

## Risks

Gating only UI leaves executable backend routes. Gating store/ownership setup can
expose old private history. This task must prove both directions independently.

## Parallelism

`sequential`

## Inputs

Assistant design: Rollout isolation and Existing invariants; existing
`TestOrchestrationFlagGuardsRuntimeAndLegacyRoutes` and ownership regressions.

## Results

Implemented on 2026-09-17. Assistant admission now requires both flags; native
launch, queued dispatch, runtime tokens, context validation, intake and CLI
exposure enforce the gate. Retained owners, schemas and authorized history remain
available. The root profile is a symlink to the embedded profile; one default
entry supplies all three environments.

Validation: focused admission tests passed after the expected red failures;
all tests in runtime, runtimeflags, config, profiles and agentctl passed. The
race command above exercised backend composition, runtime admission and native
executor ownership tests (the other selected packages contain no matching test
names and are not counted as coverage). Three frontend files passed all 12 tests,
and typecheck, specification lint and whitespace checks passed. A fresh managed
host browser build ran both orchestration specs together: 2 passed, one worker,
zero retries. The workspace spec includes all four flag combinations across
restarts, unchanged private history, rejected disabled comments, ordinary chat,
automation delivery and mobile navigation. Local logs are retained in the
implementation evidence directory; no live configuration was changed.
