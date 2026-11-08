---
created: 2026-11-07
status: complete
requirements:
  - REQ-UI-CONTEXT-COMPACTION-COUNT-001
system_design:
  - ../../specs/ui/system-design/context-compaction-count.md
legacy_specs: []
---

# Implementation Plan: Generic Compaction Help

## Overview

Remove the Copilot sentence from the generic inferred-count explanation shown for Codex and other agents.
One work order updates the catalogs and their existing component, desktop, and phone assertions.
The count, general accuracy explanation, and native Copilot messages retain their current behavior.

UI owns this change because the existing cross-agent count presentation contract already belongs to UI.
Agents retains provider-event interpretation and counting policy ownership.
The [requirements](../../specs/ui/requirements/context-compaction-count.md) and [system design](../../specs/ui/system-design/context-compaction-count.md#generic-help-and-provider-details) remain canonical.
This package covers criteria `.1`, `.6`, `.7`, `.14`, and `.15` under the requirement in frontmatter.

## Scope

### In scope

- Remove the provider-specific sentence from `common:contextCompactionCountHelp` in all seven supported locales.
- Regenerate the Traditional Chinese and pseudo catalogs through the existing scripts.
- Revise existing assertions to require the generic explanation without Copilot text.
- Preserve zero/non-zero counts, native coverage states, desktop access, and phone tap behavior.

### Out of scope

- Backend inference, native events, persisted totals, or reporting metadata changes.
- New provider checks, agent-profile queries, locale keys, or feature flags.
- Changes to native Copilot messages, coverage badges, controls, or tooltip composition.
- Repairing historical counts, changing provider accounting, or redesigning the phone surface.

## Technical approach

### Existing dispatch

At the verified remote `main` commit `5264536ba9845f966966c2a371c7e8136ccdf119`, `compactionHelpKey` already selects the correct message family.
`apps/web/components/task/chat/token-usage-display.tsx` needs no production change.
The defect is the Copilot sentence inside the generic locale value.

| Reporting input | Selected help | Planned behavior | Evidence |
| --- | --- | --- | --- |
| Missing metadata, including legacy sessions | Generic inferred help | Inference and accuracy explanation without Copilot text | Existing inferred-count component cases and both source-help E2E scenarios |
| `source: "usage_drop"` | Generic inferred help | Same provider-neutral explanation | Parameterized inferred-count component case |
| Native Copilot, available and observed | Native help | Preserve successful-parent and retained-history explanation | Existing native-coverage component case |
| Native Copilot, available and partial | Partial help | Preserve incomplete-history explanation and badge | Existing partial-coverage component case |
| Native Copilot, unavailable | Unavailable help | Preserve retained-history explanation and badge | Existing unavailable-coverage component case |

Missing metadata continues to select generic help.
The existing parser and dispatch retain their handling of unsupported metadata.
The browser does not infer provider identity from the context-window source or exclusion marker.

### Localized content

Edit only `contextCompactionCountHelp` in `apps/web/src/locales/{en,pt-pt,zh-cn,ja,ko}/common.json`.
Remove the equivalent Copilot sentence from each value while preserving the inference and accuracy sentences.
Generate `zh-tw` and `zh-hk` with `pnpm run i18n:zh-hant`.
Generate `pseudo` with `pnpm run i18n:pseudo`.
Review generated changes against the intended key.

The English result is:

> Kandev infers this count from context usage drops. Missing samples or provider resets can affect accuracy.

The native, partial, and unavailable help keys remain unchanged.
No data model, transport, persistence, or observability work is required.

### Existing delivery records

The original [count package](../context-compaction-count/plan.md) is complete.
The [Copilot inference correction](../copilot-compaction-inference/plan.md) is implemented.
Their results remain historical evidence for the earlier wording and counting behavior.

The [native Copilot package](../copilot-native-compaction-count/plan.md) has completed its presentation work order.
Its durable-projection work order remains blocked on separate PostgreSQL parity evidence.
This copy correction has no dependency on that remaining work and does not change its status or results.
The [native-evidence decision](../../decisions/2026-11-05-copilot-native-compaction-evidence.md) remains authoritative for counting policy.
No new ADR is required for this local copy correction.

### Documentation impact

The context-window section of `docs/public/sessions-and-review.md` accurately describes provider counting policies.
It does not quote the generic tooltip.
The public guide, root README, and screenshot catalog need no change for this correction.
Internal requirements and design now specify the revised generic explanation.

## UI preview

**UI-01: Expanded inferred-count help**, shared desktop/phone composition.
Entry point: the context-window ring in the chat-input toolbar, followed by the compaction info control.

| Region | Proposed result |
| --- | --- |
| Compaction row | Existing Compactions label, retained count, and info control |
| Expanded explanation | Kandev infers this count from context usage drops. Missing samples or provider resets can affect accuracy. |
| Native Copilot states | Existing source and coverage explanations |
| Phone interaction | Tap the ring, tap info, then tap info again to close the explanation |

This content-only preview uses the current layout and control order.
The current generic explanation includes an additional Copilot exclusion sentence, which the proposed explanation removes.
The phone needs no distinct composition for this shorter text.
Its closest shipped exemplar is `apps/web/e2e/tests/chat/mobile-context-window-source.spec.ts`.
The count, explanation content, and access paths are requirements. Line wrapping and spacing are illustrative.
Criteria: `.1`, `.6`, `.7`, `.14`, and `.15`.

## Tests

Use the existing `apps/web/components/task/chat/token-usage-display.test.tsx` cases.

| Criteria | Existing or revised case | Required evidence |
| --- | --- | --- |
| `.1`, `.6`, `.14` | `shows the inferred compaction count and its accuracy disclosure` | Parameterize missing reporting and explicit `usage_drop`. Retain inference and accuracy text and reject Copilot text |
| `.1`, `.14` | `shows zero compactions for a fresh mixed-context session` | Preserve the zero count and require generic help without Copilot text |
| `.6` | Three cases in `TokenUsageDisplay compaction coverage` | Preserve native, partial, and unavailable explanations and badge behavior |
| `.7` | Existing tooltip interaction and Escape cases | Preserve keyboard and pinned-help behavior |
| `.15` | `pnpm run i18n:check` and catalog review | All shipped locales and pseudo use the revised key with valid catalogs |

The revised component assertion must fail on the Copilot sentence before the locale change.
Reuse the existing coverage cases instead of adding a second suite or a catalog snapshot test.

## E2E tests

| Criteria | Project and file | Flow |
| --- | --- | --- |
| `.1`, `.6`, `.7`, `.14` | `chromium`: `e2e/tests/chat/context-window-source.spec.ts` | In `context source help stays open when hovered`, open the count explanation and require inference/accuracy text without Copilot text |
| `.1`, `.6`, `.7`, `.14` | `mobile-chrome`: `e2e/tests/chat/mobile-context-window-source.spec.ts` | In `context source help is reachable by touch without overflow`, require the same text and preserve tap dismissal and overflow assertions |

Keep the existing zero-count help fixtures and non-zero count coverage in both files.
The existing mock-agent/store fixture proves presentation without a live provider account.
Run the full two focused files sequentially through the managed runner, with a fresh production build.
The work order lists the exact component, i18n, browser, and documentation commands.

## Work orders

- [x] [Task 01: Make generic compaction help provider-neutral](task-01-update-generic-help.md) (`done`, wave 1, no dependencies).

Execution is sequential. Implementation is complete.

## Verification results

Design-package validation passed on 2026-11-07:

- `python3 scripts/list-docs.py validate`: passed (436 decisions and 1,931 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: actual documentation-only changes are exempt.
- The same API validated the work-order reference chain as covered, with the existing consumer component as a synthetic coverage trigger.
- `git diff --check -- docs/specs/ui docs/plans/generic-compaction-help`: passed.
- `git status --short`: only the two specification files and this new plan directory changed.

The synthetic trigger validates references without changing the component.
No product code, locale values, or permanent tests changed during planning.

Implementation passed on 2026-11-07:

- RED: the revised component cases failed on the current copy; all three generic-help cases reported the Copilot sentence in the selected explanation.
- GREEN: `cd apps/web && pnpm exec vitest run components/task/chat/token-usage-display.test.tsx` passed (20 tests).
- GREEN: `cd apps/web && pnpm run i18n:check` passed; the generated catalogs kept one changed key each and no unrelated churn.
- GREEN: `cd apps/web && pnpm run i18n:ratchet` passed.
- GREEN: `cd apps/web && pnpm run typecheck` passed.
- GREEN: focused Oxlint over the three changed frontend files passed.
- GREEN: `pnpm e2e:run --host --shards 1 --project chromium tests/chat/context-window-source.spec.ts` passed (3 tests).
- GREEN: `pnpm e2e:run --host --shards 1 --project mobile-chrome tests/chat/mobile-context-window-source.spec.ts` passed (3 tests).
- GREEN: `python3 scripts/list-docs.py validate` passed (436 decisions, 1,931 specifications).
- GREEN: `python3 scripts/lint-spec-files.test.py` passed (36 tests).
- GREEN: `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check -- docs/specs/ui docs/plans/generic-compaction-help apps/web` passed.

Rendered comparison: the desktop hover and the phone tap surface show the same count row and the shorter provider-neutral explanation.
Native Copilot, partial, and unavailable states keep their existing explanations and badges.
No layout, control, or interaction change was observed in either viewport.

## Risks

- Catalog generation can produce unrelated changes. Keep only changes attributable to this help correction.
- The locale key is shared by legacy sessions. Its general inference and accuracy explanations must remain present.
- A broad `Copilot` assertion can reject valid native help. Scope assertions to the selected explanation.
- Native projection still has an unrelated parity blocker. This package must not imply that it resolves that blocker.
