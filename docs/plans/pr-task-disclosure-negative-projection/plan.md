---
created: 2026-10-01
status: complete
requirements:
  - REQ-UI-PR-TASK-STATUS-SUMMARY-001
  - REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001
  - REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003
system_design:
  - ../../specs/ui/system-design/pr-task-status-summary.md
  - ../../specs/integrations/system-design/github-workflow-attention.md
legacy_specs: []
---

# Implementation Plan: Preserve PR details after approval clears

## Overview

A newer compact task summary can replace full PR details with an empty list.
Merged PR disclosures then show a blank card. Open PR disclosures can show only automation details.
One sequential work order repairs negative workflow projection selection and proves desktop and phone recovery.

Integration owns the approval evidence and freshness rules.
The existing UI requirement owns complete, readable PR entries.
This repair restores those active contracts without a new requirement or ADR.

## Evidence and root cause

`compactWorkflowApprovalIsNewerThanFullPRs` accepts an explicit false approval flag.
It selects compact evidence when the summary timestamp is newer than every cached PR observation.
`getCompactWorkflowStatusSummaries` creates entries only for positive approval or conflict signals.
A negative summary without conflicts therefore returns `[]`.

`getTaskPRIconViewModel` selects that array with `compactDisclosureSummaries ?? presentation.summaries`.
An empty array does not activate the nullish fallback.
Full records still exist, so `hasFullData` prevents the loading or unavailable branch.
The summary component then receives zero PR entries.
Merged and closed PRs also have no open-PR automation details.

A temporary component regression used PR #42 with title `Test PR` and author `alice`.
Its full record had `last_synced_at=2026-10-01T11:00:00Z`.
The compact summary had `statusSummaryUpdatedAt=2026-10-01T12:00:00Z` and `workflowApprovalRequired=false`.
Title assertions failed for merged, closed, and open states because the scroll body text was exactly empty.
A fourth diagnostic case reproduced automation-only content with zero PR entries for an open PR.
The temporary tests were removed after diagnosis. No production change exists.

The earlier asynchronous sizing experiment found a separate placement problem.
It did not reproduce these empty-content states and is outside this package.

## Scope

### In scope

- Preserve all cached linked PR entries after a newer explicit negative approval projection.
- Retain PR numbers, titles, authors, independent status rows, and available terminal state.
- Clear older approval rows and their stale-evidence notes across all linked PRs.
- Preserve compact conflict attribution, disclosure counts, and phone drawer identity.
- Add focused projection, component, desktop, and phone regression evidence.

### Out of scope

- Tooltip height, collision placement, scroll mechanics, and shared primitive changes.
- Provider collection, refresh cadence, APIs, persistence, credentials, or merge permission.
- A new mobile surface, new product copy, runtime flags, or public documentation changes.
- Redesigning the existing positive compact projection path.

## Technical approach

Keep the existing freshness predicate in `pr-task-workflow-projection.ts`.
A valid compact timestamp must remain strictly newer than every full observation.
Missing flags, malformed timestamps, equal timestamps, and newer full records retain the current full-record path.

Add a focused disclosure derivation helper in that file.
Call it from `getTaskPRIconViewModel` in `pr-task-icon.tsx`.
Separate the negative projection case from positive projected attention entries.
Do not use array length as evidence that approval remains active.

For a newer explicit negative, start with one derived summary per cached linked PR.
Remove rows that claim maintainer approval, including stale same-head positives.
Retain independent review, check, queue, and merge rows supported by the detail snapshot.
An explicit negative approval flag does not clear ambiguous action-required or unavailable workflow evidence by itself.
Keep the existing stale-note selection, which omits stale approval notes after the negative projection.

Keep conflict rows consistent with the accepted compact conflict projection.
Discard superseded conflict rows before applying the selected compact conflict entry.
Match that entry by repository plus PR number, never by number alone across repositories.
If its full record is absent, keep the existing attributed compact conflict entry without inventing a title or author.

For a single cached PR with the same representative number, a newer merged or closed compact state supplies its terminal row.
Normalize the compact lifecycle value before comparison because the mapper capitalizes it.
Do not apply one representative state to every PR in a mixed collection.
Otherwise retain the cached PR lifecycle and association identity.

Derive negative-path disclosure count and identity from the resulting entries.
A negative projection must not set a nonempty disclosure count to zero.
Preserve the existing compact icon precedence and accessible approval clearing.
No disclosure helper can grant merge eligibility or change automation state.

Keep the current positive compact projection behavior and its per-PR attribution tests.
A fallback to unfiltered full summaries is insufficient because it restores cleared approval rows.
Both `PRTaskIconTooltip` and `PRTaskIconDrawer` receive the same corrected content.

| Input | Disclosure behavior | Verification |
| --- | --- | --- |
| Newer explicit false, full PRs available | Full PR entries with approval removed | Negative projection unit and component cases |
| Newer false plus compact conflict | Full identities and correctly attributed conflict | Conflict and repository-collision cases |
| Newer true | Existing positive compact behavior | Existing positive/stale approval regressions |
| Missing flag, invalid/equal timestamp, or newer full evidence | Existing full-record behavior | Freshness regressions |
| No full records | Existing loading/unavailable hydration path | Existing hydration component tests |

This package changes only the GitHub task disclosure adapter.
GitLab and registered providers retain their existing shared summary behavior.

## Mobile design contract

Entry: open the phone task picker, then tap its existing PR control.
The nearest exemplar is `PRTaskIconDrawer` and `mobile-pr-sidebar-automation-indicators.spec.ts`.
The drawer provides a temporary read of PR status without task navigation.
Its header precedes the PR number, title, author, terminal or active status, and automation details.
The existing body remains the single scroll owner under its `80dvh` limit.
The shared Drawer retains safe-area handling, dismissal, and focus return.
The existing coarse-pointer hit area remains at least 44 pixels.
Desktop hover/focus and phone tap consume one disclosure derivation.

## ASCII UI preview

UI-01: Desktop sidebar hover after a newer negative approval projection.

```text
Before: merged PR             After: merged PR
+-------------------------+   +-------------------------+
|                         |   | PR #42                  |
+------------v------------+   | Test PR                 |
Task [PR]                     | by alice                |
                              | State   Merged          |
                              +------------v------------+
                              Task [PR]

Before: open PR               After: open PR
+-------------------------+   +-------------------------+
| Automation              |   | PR #42 / Test PR        |
| owner/repo PR #42        |   | by alice                |
+------------v------------+   | Independent status rows |
                              | Automation              |
                              | owner/repo PR #42        |
                              +------------v------------+
```

UI-02: Phone task picker, merged PR control tapped.

```text
Task picker -> existing PR drawer
+-----------------------------+
| Pull request #42 status     | fixed header
|-----------------------------|
| PR #42                      | existing scroll body
| Test PR                     |
| by alice                    |
| State   Merged              |
+-----------------------------+ safe-area clearance
```

A newer negative removes approval text and stale approval notes in both views.
Every cached linked PR retains its own entry in the negative path.
Spacing is illustrative. The existing components and localized labels define the presentation.
The previews map to summary AC 001.2/.3/.6/.17/.24 and workflow AC 003.4/.6/.8.

## Tests

Extend `pr-task-icon.workflow-approval.test.tsx` with a title assertion in the existing newer-negative case.
Add merged, closed, and open variants with authors and terminal rows.
Add an open PR with loaded automation options to prevent automation-only content.
Add mixed open/merged siblings with old approval evidence on more than one open PR.
The explicit negative must clear approval from every open sibling while retaining every cached identity.

Add `pr-task-workflow-projection.test.ts` for the focused helper.
Cover false, true, missing, equal, older, malformed, and newer-full precedence.
Cover negative-plus-conflict attribution with equal PR numbers in different repositories.
Cover a single cached open PR with a newer merged compact state.
Cover a mixed collection where representative lifecycle must not overwrite sibling states.
Retain existing positive compact, stale positive, current-head negative, and changed-head tests.

| Acceptance criteria | Evidence |
| --- | --- |
| UI 001.2/.3/.17 | Terminal and open content/author regressions |
| UI 001.6/.8 | Mixed PR retention through the shared indicator |
| UI 001.15 | Existing hydration/cache tests and passive-read count assertion |
| UI 001.24 | Tooltip description includes retained identity and status |
| Workflow 001.4/.8 | Independent check/review rows survive approval clearing |
| Workflow 003.4/.6/.7 | Negative freshness, every-open-PR clearing, and mixed identity cases |
| Workflow 003.8 | Desktop hover/focus and existing phone drawer |

Each abbreviated ID uses the full prefix declared in the work order.

## E2E tests

Extend `pr-sidebar-hover-hydration.spec.ts` for a merged PR with newer negative compact data.
Seed an unrelated active task and the target association through the existing API fixture.
Use a task-scoped response fixture to deliver an older full PR snapshot after the newer compact summary.
Keep workspace, repository, and association IDs from the real seeded response.
Assert the timestamp ordering before the content assertion so the test proves the defective branch.
Do not rely on elapsed sleeps or incidental provider timing.

Require visible number, title, author, and Merged text in the active tooltip.
Close and reopen with keyboard focus and require the same content without another detail load.
The component regression covers an open PR with loaded automation options and requires PR details above the automation footer.

Extend `mobile-pr-sidebar-automation-indicators.spec.ts` with the same negative terminal scenario.
Require the existing drawer, retained PR content, absence of approval text, focus return, and unchanged task URL.
Check viewport containment and absence of document horizontal overflow.
The work order contains the exact sequential chromium and mobile-chrome commands.

## Work orders

- [x] [Task 01: Preserve negative-projection PR details](task-01-preserve-pr-details.md)

## Verification results

Design validation passed on 2026-10-01:

- `python3 scripts/list-docs.py validate`: 339 decisions and 1,281 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Local `.github/scripts/pr-docs.cjs` coverage preflight: `covered`, with both design references and all declared requirements accepted.
- `git diff --check`: passed.
- A prose review found no sentences over 25 words in the new plan and work order.

The coverage preflight simulated the two planned production paths.
It changed no runtime file and made no GitHub request.
The plan and work order remain unstaged and uncommitted.

Implementation completed on 2026-10-01. The negative path retains cached PR identities and independent rows while clearing stale approval, applies compact conflicts with repository-aware attribution, and applies compact terminal state only to its matching single PR.

- Focused unit/component suite: 6 files and 83 tests passed after review remediation.
- `pnpm run typecheck`, targeted ESLint, and `pnpm run i18n:ratchet`: passed.
- Desktop Chromium and mobile Chrome E2E: 3 tests passed in each project.
- Documentation and specification validation passed; `git diff --check` passed after result updates.

The earlier failed regression assertions are root-cause evidence. The permanent desktop and phone scenarios now pass against deterministic timestamps that prove they exercise the newer-negative branch.

Review remediation on 2026-10-02 keeps terminal PR disclosures free of open-only merge rows and replaces stale `ready`/`mergeable` rows with a newer attributed conflict. Independent queue evidence and unrelated sibling rows remain intact. The pure-helper and rendered component regressions, typecheck, lint, formatting, and both browser specs passed after the change.

## Risks

- Unfiltered fallback restores cleared approval warnings.
- Reusing projected array length can erase the multi-PR count or phone heading.
- PR numbers alone are ambiguous across repositories.
- A representative compact state cannot define every sibling lifecycle.
- An E2E fixture with no timestamp-order assertion can pass through an unaffected branch.
