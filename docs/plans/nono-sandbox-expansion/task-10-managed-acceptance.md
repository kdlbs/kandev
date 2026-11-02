---
id: "10-managed-acceptance"
title: "Prove the upgraded host and packaged plugin together"
status: pending
wave: 10
depends_on: 
  - "09-typed-learning"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SANDBOX-012
  - REQ-AGENTS-SANDBOX-003
  - REQ-AGENTS-SANDBOX-005
  - REQ-AGENTS-SANDBOX-006
  - REQ-PLUGINS-SANDBOX-002
acceptance_criteria:
  - AC-AGENTS-SANDBOX-012.1
  - AC-AGENTS-SANDBOX-012.2
  - AC-AGENTS-SANDBOX-012.3
  - AC-AGENTS-SANDBOX-012.4
  - AC-AGENTS-SANDBOX-003.1
  - AC-AGENTS-SANDBOX-003.2
  - AC-AGENTS-SANDBOX-005.2
  - AC-AGENTS-SANDBOX-005.3
  - AC-AGENTS-SANDBOX-006.3
  - AC-AGENTS-SANDBOX-006.4
  - AC-PLUGINS-SANDBOX-002.4
  - AC-PLUGINS-SANDBOX-002.5
system_design:
  - ../../specs/agents/system-design/sandbox-operation-approvals.md
  - ../../specs/agents/system-design/sandbox-policies.md
  - ../../specs/plugins/system-design/sandbox-provider-v2.md
---

# Task 10: Prove the upgraded host and packaged plugin together

## Summary

Run actual managed acceptance for the final pair and reconcile public documentation, runtime locks, and evidence with the demonstrated support matrix.

## In scope

- Disposable source-built instance, exact package/runtime/agent versions, actual discovery/ACP/native/MCP/restart/cleanup, typed approvals and negative enforcement.
- Update source receipts, plugin README, host walkthrough, and public docs; keep feature default-off. Separate host and plugin PR delivery.

## Out of scope

- No merge, production enablement, retained-instance shutdown, remote executor expansion, or silent downgrade.

## Acceptance

- The final pair passes actual managed flows, including same-session recovery and all three added approval classes with forbidden alternatives.
- Fresh receipts identify artifact hashes and limitations; no skipped or historical test is presented as current acceptance.
- The fixture stops through verified ownership and cleanup; main :9998 and retained instances remain unchanged.

## Verification

Run from the host repository root. New test files/scripts named below are implementation deliverables. A command that runs zero relevant tests or skips real-runtime coverage does not satisfy this order.

```bash
set -e
: "${NONO_ACCEPTANCE_BINARY:?Set the verified candidate binary path before runtime tests}"
(cd ../kandev-plugin-nono-main && make validate verify-package)
: "${NONO_ACCEPTANCE_BINARY:?Set the verified packaged runtime path}"
: "${NONO_ACCEPTANCE_SEED_MANIFEST:?Set an owned disposable acceptance manifest}"
: "${NONO_ACCEPTANCE_PI_AUTH:?Set a private explicit fixture login path}"
python3 scripts/check-nono-real.py --check-prerequisites
python3 scripts/check-nono-real.py --conformance --record /tmp/nono-expansion-final-runtime.json
python3 scripts/check-nono-real.py --run --record /tmp/nono-expansion-final-managed.json
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `scripts/check-nono-real.py`
- `scripts/nono_managed.py`
- `docs/plans/nono-sandbox-expansion/acceptance.md (new during implementation)`
- `docs/plans/nono-sandbox-integration/walkthrough.md`
- `docs/public/`
- `../kandev-plugin-nono-main/README.md`
- `../kandev-plugin-nono-main/runtime-lock.json`

## Dependencies

09-typed-learning.

## Risks

Real credentials and delegated kernel containment must belong to the disposable fixture. Missing prerequisites are blockers, not passing skips.

## Parallelism

`sequential`. Keep work in the primary session.

## Inputs

- Linked requirement IDs and system designs above.
- Existing sandbox policy, provider, and execution UI tests; upstream source evidence in [the plan](plan.md#source-evidence).

## Results

Pending.
