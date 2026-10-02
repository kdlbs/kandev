---
id: "04-delivery"
title: "Documentation and delivery"
status: in_progress
wave: 4
depends_on: ['03-ui']
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-NOTIFY-001
  - REQ-AGENTS-RUNTIME-NOTIFY-002
acceptance_criteria:
  - AC-AGENTS-RUNTIME-NOTIFY-001.1
  - AC-AGENTS-RUNTIME-NOTIFY-001.4
  - AC-AGENTS-RUNTIME-NOTIFY-002.7
system_design:
  - ../../specs/agents/system-design/runtime-update-notifications.md
---
# Task 04: Documentation and delivery

## Summary and scope

Reconcile all-agent coverage and public recovery/ownership guidance. Promote completed artifact lifecycle, run documentation gates, then commit/push/PR/fixup and merge with exact-head required checks and reviews.

## Out of scope

No model discovery, worker delegation, or global developer CLI/login changes. Native unverified activation remains manual.

## Acceptance

- The linked acceptance criteria hold across multiple registered identities.
- Failures preserve authoritative state and expose truthful recovery.
- Exact verification below passes, with results recorded.

## Verification

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

docs/public/agents-and-profiles.md; docs/specs/agents/; docs/plans/agent-runtime-notifications/; relevant AGENTS.md; PR template

## Dependencies and inputs

03-ui. Read the linked requirements/design and nearest source/tests.

## Risks

See the plan for native ownership, source failure, consent/selection races, and overlapping PRs.

## Parallelism

sequential

## Results

Public agents/profile how-to guidance, root README, documentation coverage and the owning requirements/design are updated. New ownership ADR and every registered-agent coverage matrix are included. Existing automatic-update exclusions now refer to the new owning contract. Specification index validation, all-spec lint, harness tests/lint and 62 public-document validator tests passed; public validation covers 47 published pages.

Commit, push, PR publication and exact-head CI/review/merge gates remain external delivery work. Their current evidence is maintained in the Kandev task plan so a documentation commit does not claim checks for a different head.

PR review remediation covers shared-source caller cancellation, consent-preserving manual admission and durable-outcome release before refresh callbacks. Native-host managed fallback controls preserve package selection/recovery without global native mutation or false host capability publication. Red/green regression tests pass for cancellation, rejected manual requests, terminal admission, verified native fallback update/rollback/default, strict policy JSON, save contributor identity, original outcome identity, and bootstrap readiness. Backend controller/handler/registry/backendapp tests pass with -race -tags fts5; five directly changed frontend suites pass 39 tests, plus two card-destination snapshot tests. TypeScript, all seven shipped locales, 18 desktop and seven phone E2E cases pass. External exact-head CI/review/merge gates remain pending.

Current-head CI exposed nested-submodule review missing the root diff. All three CI error contexts show a blank root patch while child patches render. The isolated E2E passed once and four repetitions without retries, so timing alone was not accepted as a disposition. Current main already includes a focused regression and fix for child snapshots rejecting root enrichment. Its regression fails against the pre-integration handler and all 22 Git-status unit tests pass after integration. The merge tree also passed nine backend race packages, 132 frontend tests, typechecking and documentation validation before integration. A fresh production build passed both desktop nested-submodule checks; the phone parity check also passed, all with retries disabled. Specification catalog and full spec lint pass after updating this record. The runtime-update UI and source contracts are unchanged; existing runtime screenshots remain applicable.

The completed CI run passed all checks but its blob/retry audit identified seven first-attempt E2E failures. Five desktop cases passed local reproduction, while the restored-panel assertion reproduced a failure after reload: Files was reachable but not selected. The requirement covers pane visibility and reachable controls, so the test now selects Files before checking its content. The phone failure was a 44px target measured as 43.99993896484375px; the same minimum is checked after normalizing geometry to hundredth-pixel precision. No timeouts, production behavior, public contract, locale copy, or screenshots changed.

The synthetic current-main merge passed 197 frontend tests, typecheck, specification/public-doc validation, and runtime controller/backendapp/agentctl-client race tests. The complete agentctl process race suite passed with an isolated temporary HOME: the original environment's login profile aborted on a missing Cargo environment file before test fixtures ran. Global profiles and installations remain untouched.

Post-correction commands (one worker, production build, retries disabled):

```bash
cd apps/web
pnpm e2e:run --no-build --project chromium tests/layout/right-panel-visibility.spec.ts -- --retries=0
pnpm e2e:run --no-build --project mobile-chrome tests/session/mobile-session-refresh-efficiency.spec.ts -- --retries=0
```

The full right-panel spec passes ten cases. A separate six-case desktop run covers every desktop test reported in the retry audit and passes without retries. The full phone refresh spec passes three cases without retries. The next exact-head CI/review/merge gates are recorded in the Kandev task plan.
