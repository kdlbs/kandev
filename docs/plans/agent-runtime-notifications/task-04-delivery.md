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
