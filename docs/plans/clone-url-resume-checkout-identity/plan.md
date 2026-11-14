---
created: 2026-10-25
status: implemented
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-005
system_design:
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
  - ../../specs/agents/system-design/session-recovery-failures.md
legacy_specs: []
---

# Clone-URL resume checkout identity

## Outcome

Resume a clone-URL task against the canonical checkout inventory when its
checkout branch differs from its comparison base. Preserve the existing
workspace and all inventory admission checks. This repairs issue #4786.

## Scope and approach

For required workspace reuse, the executor carries the sanitized checkout
identity from one unambiguous task-repository attachment into the single-repository
resume request. Fresh materialization retains its base-branch inventory identity. It
leaves `CheckoutBranch` unset so resume does not request another checkout.
Missing or ambiguous attachments retain the existing behavior. Local executor
requests retain their current branch handling.

The task system owns the canonical workspace identity contract. This repair
implements its existing requirements and design without changing public APIs,
configuration, recovery policy, or user instructions.

## Work orders

- [x] [Task 01: Preserve checkout identity on resume](task-01-preserve-checkout-identity.md)
- [x] [Task 02: Repair CI recovery and readiness races](task-02-repair-ci-recovery-readiness.md)

## Verification

Run focused resume, reuse, and inventory tests. Prove that the regression fails
with the identity assignment disabled. Validate documentation coverage and run
normal commit hooks before pushing.

## Risks

A branch identity must select the existing inventory row without authorizing
new materialization. Tests must retain reuse admission and leave checkout and
clone requests unset. Hosted CI and current-head bot reviews remain delivery
gates after local validation.

## Documentation impact

No public docs change is needed. This repair restores documented workspace
continuity and introduces no new operator action or public contract.

## Results

The focused resume, reuse, and inventory suite passed 372 tests and subtests.
One existing test skipped because it requires a case-insensitive filesystem.
The new regression passed with the race detector for all three executor types.
Disabling the identity assignment reproduced the original mismatch in each case.
Documentation coverage and diff whitespace checks passed. Normal commit hooks
and hosted CI/reviews remain delivery gates for the final commit.

## Review remediation

CodeRabbit identified that a fresh materialization could be labelled with a
checkout identity while preparing the base branch. Restrict the identity
assignment to required workspace reuse. The new non-reuse regression failed
with `feature-y` before this restriction and passed with `release-x` afterward.
The focused race regressions passed after the correction. Final hosted checks
and a fresh Claude review remain pending for the new commit.

## CI remediation results

The workspace and remote-recovery race suite passed 266 tests/subtests with
one existing skip. Six mobile repeats, the desktop Quick Chat regression,
the unchanged SSH interruption test, and the unchanged isolated Kubernetes
stop/restart test passed with retries disabled. Go lint reported zero issues.
The Kubernetes shard sequence replay and fresh hosted checks remain pending.
See Task 02 for the Nix host prerequisite and temporary network rule.

## Final identity sequence correction

The follow-up regression covers fresh materialization and later reuse in one
sequence. A validated canonical base slot now supplies identity when the
configured checkout slot has no match. The expanded focused suite passed 394
tests/subtests with one existing filesystem skip. Focused race checks passed.
This retains strict rejection for foreign branches, inactive or duplicate rows,
foreign environment IDs, wrong placements, and explicit checkout requests.
