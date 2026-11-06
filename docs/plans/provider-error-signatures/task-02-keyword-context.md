---
id: "02-keyword-context"
title: "Classify unmatched provider diagnostics using context"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROVIDER-ERROR-SIGNATURES-002
acceptance_criteria:
  - AC-AGENTS-PROVIDER-ERROR-SIGNATURES-002.1
  - AC-AGENTS-PROVIDER-ERROR-SIGNATURES-002.2
  - AC-AGENTS-PROVIDER-ERROR-SIGNATURES-002.3
  - AC-AGENTS-PROVIDER-ERROR-SIGNATURES-002.4
  - AC-AGENTS-PROVIDER-ERROR-SIGNATURES-002.5
  - AC-AGENTS-PROVIDER-ERROR-SIGNATURES-002.6
  - AC-AGENTS-PROVIDER-ERROR-SIGNATURES-002.7
system_design:
  - ../../specs/agents/system-design/provider-error-signatures.md
---

# Task 02: Classify unmatched provider diagnostics using context

## Outcome and scope

Add a default-off structural/keyword/context fallback after existing rules and
before phase defaults. Own keyword/context files, typed flag wiring,
diagnostic producer/consumer consistency and dynamic circuit integration.
Preserve explicit candidate policy, all safety guards and model order.

The classifier is prepared independently on the current main base. Preserve its
known rules and empty-turn behavior; do not import the pending literal catalog,
reimplement either subsystem, add an external loader, change installed account
attribution, activate an operator flag or publish from this worktree.

## Acceptance

- OFF equals the prior classifier; ON preserves typed and known-rule results
  and distinguishes structural, keyword, ambiguity, quote and context cases.
- Current diagnostic evidence reaches guarded routing/policy and scope/reset/
  probe consumers. Saved policies and unclassified thresholds win.
- The exact changed snapshot passes focused checks and normal hooks, receives
  independent review, then reaches the upstream publication owner.

## Verification

The initial implementation used excluded disposable Go overlays. The approved
PR review follow-up adds minimal regressions to existing package test files;
run only their relevant entrypoints:

```text
go test -trimpath -tags fts5,e2e ./internal/agent/runtime/routingerr ./internal/agent/runtime/dynamic ./internal/orchestrator -run '^(TestClassifyKeyword|TestKeyword|TestConsecutiveKeyword)' -count=1
```

Run affected signature, diagnostic, unclassified and circuit controls. Use race
checks for changed evidence ownership. Verify existing runtime flag/profile/
frontend feature contracts, changed-Go lint and normal hooks. No whole-backend
build or unchanged whole-review repetition is required.

## Results

The initial excluded classifier check failed on the old phase default, then
passed after adding the prefilter. Classifier boundaries, scoped circuits,
structured HTTP projection and guarded next-candidate selection passed focused
checks. A reproduced cross-candidate count leak was corrected; additional
orchestrator race checks passed. Conflicting cross-model observations now keep
path/model scope. Existing flag contracts, frontend feature data (8 tests),
typecheck and specification checks passed. Normal changed-Go lint, all other
applicable commit hooks and commit-message validation passed for the local
implementation.

The review follow-up covers bounded envelopes, distinct hits, ties,
normalization, structural statuses, disabled/known-rule precedence, scoped
circuits, reset deadlines, probe rollback, saved policies and bounded
consecutive observations. Reproduced mixed ACCOUNT/GONE and generic LIMIT
diagnostics retain ambiguity instead of granting automatic quota recovery;
existing known rules and single-term confidence remain unchanged.

Actual session launch context supplies both optional account context and the
candidate's separate fallback account identity. Existing billing detection is
applicable only to a proven local launch using the backend's default credential
home; remote, overridden or missing context remains unknown. Focused checks
confirm that OFF does not consult it, stored profiles remain unchanged, known
same-account candidates share limits, and unknown bindings stay isolated.
The ordinary known-rule binding remains unchanged. Missing models stay
candidate-scoped, and an OFF consumer retains its legacy circuit duration even
when an input carries keyword metadata. The terminal
stream type/identity correction is incorporated from the preceding change.
Additional real-entrypoint checks preserve known-rule priority over newly
projected HTTP metadata and correlate repeated failures by semantic group.

No OpenCode Go paid-tier detector exists in the current registered agent; its
default remains API key. Post-output mid-turn recovery remains refused by the
existing safety contract. The classifier returns recommendations without implementing generic post-output
continuation, which is outside this work order. The original local candidate
passed independent formal review and was received by the publication owner.
It is now reconciled as a separate delta on current main; changed-context
verification, exact delta review and publication remain distinct milestones.

Current-base focused classifier, circuit, billing, diagnostic projection and
guarded-routing overlays passed, including the current continuation controls.
Real stream checks preserve base attribution when OFF and transport-type/legacy
selection when ON. Pending-tool admission still requires the retained runtime.
Flag/profile/config feature contracts and frontend feature data (9 tests) passed;
the backend composition package compiles. No pending literal catalog is imported.
Broad Windows package attempts also encountered unchanged home-directory and
escaped-path assertions outside this delta; those runs are not all-green results.

The startup fallback separates only the matching wrapper of a typed process-start
or ACP-initialization failure after existing classification remains unknown.
Both wrapped and inner diagnostic bounds, envelope/quote/negation controls,
known-rule priority, OFF behavior and internal-failure rejection remain intact.
