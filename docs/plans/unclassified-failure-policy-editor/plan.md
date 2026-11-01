---
status: done
created: 2026-10-23
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-002
system_design:
  - ../../specs/agents/system-design/dynamic-unclassified-fallback.md
---

# feat(agents): edit unclassified failure policies per candidate

Edit existing unclassified-failure policy per dynamic candidate with integer thresholds 2 through 10 and a confirmed bulk draft action.

## Delivery order and dependencies

The implementation is one existing PR slice. Preserve its native dependencies and existing feature defaults, then verify the narrow affected paths. No dependent PR is imported into this package.

- [Implementation and verification](task-01-implementation.md)

## Scope and risks

Backend routing policy, candidate reordering, automatic activation, and changes to transient/hard policies. These remain outside this delivery slice. Preserve existing files, configuration and history; report unavailable integration coverage without inventing a successful result.

## Verification strategy

Reuse the completed focused implementation checks recorded below and the existing documentation-coverage evaluator. Only remaining review fixes require their affected checks to run again.

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
