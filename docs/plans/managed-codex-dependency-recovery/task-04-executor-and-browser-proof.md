---
id: "04-executor-and-browser-proof"
title: "Prove executor and user outcomes"
status: pending
wave: 4
depends_on:
  - "02-task-recovery"
  - "03-probe-recovery"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-005
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.1
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.2
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.5
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.6
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.7
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.8
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-005.9
system_design:
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
---

# Task 04: Prove executor and user outcomes

## Summary

Exercise real private preparation through task resume and capability refresh,
including Docker/SSH execution hosts and phone interactions. Update public
guidance only after the behavior is implemented.

## In scope

- Extend existing deterministic npm/bridge fixtures with the actual missing
  platform-package error and a bridge that remains alive.
- Desktop/phone task resume, one accepted prompt, capability and profile
  refresh, failure details, cancellation, and draft/context preservation.
- Local Docker and SSH executor-local installation and healthy-sibling proof.
- Public managed-runtime guidance; reconcile final specification status and
  accurately record every work-order command/result.

## Out of scope

- Provider credentials, public-registry-dependent tests, seeded-card-only
  recovery proof, new UI composition/copy, and unrelated broad verification.

## Acceptance

1. Browser tests enter through visible Resume/Refresh actions and show the
   recovered original conversation or model choices, with persisted identity
   and single-prompt evidence. Desktop and phone have equivalent outcomes.
2. Failure/cancellation scenarios show one truthful existing error surface or
   prevent replacement. Docker/SSH tests prove the private install occurred
   remotely, not in the backend's cache, while a healthy sibling still reads
   and uses its runtime files.
3. Public docs explain the bounded automatic path without promising failure
   prevention, global cache deletion, or version fallback. All package
   commands/results and spec lifecycle states match actual delivery.
4. Desktop and phone tests exercise the missing platform-package initialize
   diagnostic while the bridge remains alive after its provider child exits.
   Independently verified package absence triggers exactly one private
   preparation and one retry, recovering in the original task/session.
   Existing transient npm-failure coverage alone does not satisfy this case.

## Verification

Run the E2E commands sequentially; the managed runner builds the production
assets and backend. Restore dependencies only if a chosen command reports they
are missing, then run `pnpm install --frozen-lockfile` from `apps/` once.
Docker/SSH cases require a real Docker daemon. If unavailable, record the
explicit blocker; do not mark this work order done.

Run the sequence below before staging or committing; `git diff --check`
therefore checks the unstaged implementation changes. After committing, also
run `git diff --check <delivery-base> HEAD` with the actual delivery base
revision to cover the complete committed delta.

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/session/managed-runtime-startup-retry.spec.ts tests/settings/host-utility-managed-runtime-recovery.spec.ts tests/settings/profile-capability-discovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-managed-runtime-startup-retry.spec.ts tests/settings/mobile-profile-capability-discovery.spec.ts)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project containers tests/docker/managed-runtime-npm-recovery.spec.ts tests/ssh/managed-runtime-npm-recovery.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/managed-codex-dependency-recovery
```

## Files likely touched

- `apps/web/e2e/helpers/managed-runtime-recovery.ts` and existing executor fixture scripts.
- `apps/web/e2e/tests/session/managed-runtime-startup-retry.spec.ts`.
- `apps/web/e2e/tests/session/mobile-managed-runtime-startup-retry.spec.ts`.
- `apps/web/e2e/tests/settings/host-utility-managed-runtime-recovery.spec.ts`.
- `apps/web/e2e/tests/settings/profile-capability-discovery.spec.ts`.
- `apps/web/e2e/tests/settings/mobile-profile-capability-discovery.spec.ts`.
- `apps/web/e2e/tests/docker/managed-runtime-npm-recovery.spec.ts`.
- `apps/web/e2e/tests/ssh/managed-runtime-npm-recovery.spec.ts`.
- `docs/public/agents-and-profiles.md`.
- This plan, all completed work-order Results sections, and the amended
  managed-runtime requirement/design status fields.

## Dependencies

Tasks 02 and 03.

## Risks

- A generic npx-exit fixture misses the live-bridge/provider-child distinction.
- API metadata alone does not prove a visible resume/refresh flow recovered.
- Never add unbounded workers or overlap container and browser suites.
- If generic copy is inaccurate, stop to update the design and affected-region
  preview before editing rendered UI; translate any then-required copy.

## Parallelism

`sequential`

## Inputs

- Requirement 005 and the design's Presentation, compatibility, and proof section.
- Plan test/E2E matrix and presentation/mobile contract.
- `/e2e`, `/mobile-parity`, `/docs-maintainer`, backend/web scoped guidance.
- Existing startup-retry, profile discovery, Docker, and SSH fixture patterns.

## Results

Pending.
