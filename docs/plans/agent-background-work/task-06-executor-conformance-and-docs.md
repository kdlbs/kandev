---
id: "06-executor-conformance-and-docs"
title: "Executor conformance fixtures and public guidance"
status: pending
wave: 8
depends_on:
  - "08-steering-composer"
plan: "plan.md"
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-001
  - REQ-AGENTS-BACKGROUND-WORK-003
  - REQ-AGENTS-BACKGROUND-WORK-004
  - REQ-AGENTS-BACKGROUND-WORK-005
  - REQ-AGENTS-BACKGROUND-WORK-006
  - REQ-PLATFORM-EXPLICIT-STEERING-001
  - REQ-PLATFORM-EXPLICIT-STEERING-002
  - REQ-PLATFORM-EXPLICIT-STEERING-003
acceptance_criteria:
  - AC-AGENTS-BACKGROUND-WORK-001.3
  - AC-AGENTS-BACKGROUND-WORK-001.4
  - AC-AGENTS-BACKGROUND-WORK-003.1
  - AC-AGENTS-BACKGROUND-WORK-003.3
  - AC-AGENTS-BACKGROUND-WORK-004.4
  - AC-AGENTS-BACKGROUND-WORK-005.5
  - AC-AGENTS-BACKGROUND-WORK-006.1
  - AC-AGENTS-BACKGROUND-WORK-006.2
  - AC-AGENTS-BACKGROUND-WORK-006.3
  - AC-PLATFORM-EXPLICIT-STEERING-001.1
  - AC-PLATFORM-EXPLICIT-STEERING-001.2
  - AC-PLATFORM-EXPLICIT-STEERING-002.5
  - AC-PLATFORM-EXPLICIT-STEERING-003.1
system_design:
  - ../../specs/agents/system-design/background-work.md
  - ../../specs/platform/system-design/explicit-turn-steering.md
---

# Task 06: Executor conformance fixtures and public guidance

## Summary

Extend executor fixtures to prove normalized controls and observations travel through the owning runtime across local, Docker, SSH and Kind. Document the capability-based product and its verified limitations, with separate live-provider evidence.

## In scope

- Extend the native Codex fake-server executor fixture for background list/terminate, child runs, controlled delayed events, output and disconnect recovery. Add backend-to-browser ownership assertions, not just UI snapshots.
- Extend the same fake-server executor fixtures with native turn/steer: exact expected turn, sequential messages in one turn, nonempty queue preserved, stale rejection and uncertain response without retry. Record optional live TestNativeCodexSameTurnSteering separately.
- Add remote tests in each executor directory using the containers project; a remote process ID must never cause a host PID operation. Test feature-off and capability-off fallback.
- Extend provider-neutral conformance fixtures and retain ACP observation-only E2E; do not claim new ACP or Claude-native control support.
- Complete protocol-evidence.md with version/head, per-scenario results, exact commands and fake/live labels. Run the opt-in live harness only in a disposable authorized environment; record missing credentials or unobserved cases as incomplete evidence, not pass.
- Update user-facing how-to/reference docs for shared entry points, independent feature gates, Send now versus Queue for later, same-turn versus provider-managed semantics, delivery outcomes, capabilities, supported stop versus stdin limitations, retained/unknown state and estimated usage. Update coverage metadata only if required by its validator; do not publish draft implementation intent as shipped behavior.
- Reconcile affected original/native follow-up plan cross-links without overwriting historical results. This task produces fixtures and documentation; it is not a generic extra QA/review pass.

## Out of scope

New native providers, generic scheduling, host PID control, and production changes owned by later work orders. Preserve the design's explicit exclusions.

## Acceptance

1. Desktop remote fixtures pass through real Docker/SSH/Kind routing with a fake provider, and a normalized observation-only adapter still works in the same UI.
2. Exact pinned-version live results or explicit unverified limitations are recorded separately; no non-triggered approval/child/output scenario is counted as proof.
3. Public docs, runtime-gate descriptions and internal contract references agree with the final implementation and targeted validation evidence.

## TDD and verification

Write the named behavioral tests first and observe the relevant failure before implementation. Proposed file/test names below are implementation targets, not existing coverage. Run from repository root; fresh worktrees first install with `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project containers tests/docker/background-work.spec.ts tests/ssh/background-work.spec.ts tests/kubernetes/background-work.spec.ts)
(cd apps/backend && go test -race ./pkg/codexappserver ./internal/agentctl/server/adapter/transport/codexappserver)
make -C apps/backend lint
make -C apps/backend build
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Optional authenticated evidence, separate from fake-server acceptance (requires disposable workspace and configured credentials):

```bash
(cd apps/backend && KANDEV_CODEX_APP_SERVER_E2E=1 go test -tags=e2e ./internal/agentctl/server/adapter/e2e -run '^TestNativeCodex(BackgroundWork|SameTurnSteering)' -count=1 -timeout=15m)
```

If live evidence is unavailable, the gated feature may retain only its tested
capabilities and the evidence document must clearly state the limitation; do not
mark a live scenario passed. Do not overlap complete E2E suites or override the
runner's worker budget.

## Files likely touched

- Existing: `apps/web/e2e/helpers/native-codex-app-server-executor.ts` and remote executor helpers; proposed `tests/{docker,ssh,kubernetes}/background-work.spec.ts` under `apps/web/e2e/`.
- Existing/proposed: `apps/backend/internal/agentctl/server/adapter/e2e/native_codex_background_test.go`, proposed `native_codex_steer_test.go`, and conformance fixtures.
- Docs: `docs/public/agents-and-profiles.md`, `docs/public/agent-communication.md`, runtime-toggle reference found through docs catalog; `protocol-evidence.md` and implementation Results sections.
- Inspect `README.md` and `docs/screenshots.md` for affected claims; update only when changed product behavior makes them inaccurate.

## Dependencies

Task 08: Explicit composer delivery on desktop and phone (including Tasks 05 and 07).

## Risks

Fake executors prove routing, not live provider behavior. Docker/SSH/Kind prerequisites and authenticated Codex credentials are separate evidence prerequisites.

## Parallelism

`sequential`

## Inputs

- [Explicit steering requirements](../../specs/platform/requirements/explicit-turn-steering.md)
  and [design](../../specs/platform/system-design/explicit-turn-steering.md).

- [Requirements](../../specs/agents/requirements/background-work.md), criteria listed in frontmatter.
- [System design](../../specs/agents/system-design/background-work.md), corresponding mapped sections.
- [ADR](../../decisions/2026-09-27-provider-neutral-background-work.md).
- [Plan baseline and existing test patterns](plan.md#baseline-and-integration-points).

## Results

Pending.
