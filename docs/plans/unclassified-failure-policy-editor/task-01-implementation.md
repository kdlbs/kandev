---
id: "01-implementation"
title: "feat(agents): edit unclassified failure policies per candidate"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-002
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.1
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.2
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.10
system_design:
  - ../../specs/agents/system-design/dynamic-unclassified-fallback.md
---

# feat(agents): edit unclassified failure policies per candidate

## Outcome and scope

Edit existing unclassified-failure policy per dynamic candidate with integer thresholds 2 through 10 and a confirmed bulk draft action.

Out of scope: Backend routing policy, candidate reordering, automatic activation, and changes to transient/hard policies.

## Implementation acceptance

1. The affected native entrypoint follows the linked requirement and design while preserving unrelated behavior and history.
2. Rejected, stale and failed operations retain their existing refusal or rollback semantics.
3. The focused implementation checks and linked documentation coverage have honest recorded results; unexecuted follow-up checks remain pending.

## Files and dependencies

The source files are the existing PR #4724 diff. This work order adds its delivery record and links existing requirements and system design; it introduces no runtime gate or permanent test.

- [System design](../../specs/agents/system-design/dynamic-unclassified-fallback.md)
- [Plan](plan.md)

## Verification and results

Validation: 16 existing policy/editor/save-concurrency tests and 12 disposable original-source checks passed, covering targeted draft updates, bulk apply/cancel, switch/threshold behavior, fractional rejection and summaries. Full TypeScript, changed-file ESLint, locale validation and all normal commit hooks passed. Temporary fixtures were removed; no standing tests or CI were added.

Documentation-only correction: git diff --check and the existing .github/scripts/pr-docs.cjs validateCoverage entrypoint with the candidate changed paths and the linked candidate documents supplied on stdin. No GitHub status is synthesized locally.

## ASCII UI preview

UI-01: Existing affected surface, after this change.

```text
Dynamic profile > Candidate
  [ ] Unclassified failures
      Consecutive failures: [2..10]
  [Apply threshold 2 to all...]

The bulk confirmation edits the draft only. Existing candidate layout is shared across desktop and phone.
```

This sketch records the affected structure; spacing is illustrative. It does not add a new control or approval requirement.


## Review follow-up verification

The existing parent agent setup Save contributor accepted invalid newly created
candidate thresholds because only the embedded editor checked the policy and
that editor did not own the dirty save contribution. The parent now reads its
current canonical draft through the same existing unclassified-policy predicate,
retaining administrator, name/model, MCP and provider configuration guards.
The specific threshold reason accompanies the blocked shared Save.

- Disposable parent-save probe: thresholds 1, 2.5 and 11 plus a valid-to-invalid
  draft edit failed before the fix; all six checks passed afterward. Thresholds
  2 and 10 and a disabled policy remain accepted.
- Existing policy editor, payload and standalone save concurrency checks passed:
  3 suites, 16 tests.
- Changed-file ESLint and i18n checks passed. Two existing description/help values
  were updated in each shipped human locale; the two pseudo values were generated
  through the existing transformer.
- The bulk action now uses the existing settings action sizing helper. The
  design describes the actual expanded candidate list, preserving its layout.
- The disposable fixture and its explicit-only config are removed after checking;
  no permanent tests or CI workflow changes are added.

Commands run from apps/web:

    pnpm exec vitest run --config node_modules/.cache/p4-review.vitest.config.ts
    pnpm exec vitest run components/settings/dynamic-agent-policy-editor.test.tsx components/settings/dynamic-agent-profile-editor-state.test.ts components/settings/dynamic-agent-profile-editor-save-concurrency.test.tsx
    pnpm exec eslint "app/settings/agents/[agentId]/agent-save-contributor.ts" components/settings/dynamic-agent-unclassified-bulk-apply.tsx
    pnpm i18n:check

Normal commit hooks must pass before the follow-up is published.
