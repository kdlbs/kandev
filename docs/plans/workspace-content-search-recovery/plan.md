---
created: 2026-10-21
status: done
requirements:
  - REQ-UI-TASK-WORKSPACE-CONTENT-SEARCH-001
system_design:
  - ../../specs/ui/system-design/task-workspace-content-search.md
legacy_specs: []
---

# Implementation Plan: Workspace Content Search Recovery

## Overview

Retire an earlier attempt's error after a successful retry so a recovered empty
search completes as no-match. The parent source diagnostic and real-hook
diagnostic reproduce the stale error; existing match results mask the defect.
The explicit bounded-fix authorization waives a separate planning pause.

## Scope

Own the content-search hook, faithful transport-boundary regression tests,
existing owning specifications and this delivery package. Preserve retries,
timing, incremental results, deduplication, cancellation, limits and raw identity.
Exclude backend changes, caching, the separate committed-render hypothesis,
plugin settings, delegation and publication before parent validation.

## Technical approach

Clear `lastError` after a successful `searchWorkspaceContent` response in
`apps/web/hooks/domains/session/use-workspace-content-search.ts`. Mock only
`getWebSocketClient().request` in the hook tests so the actual WS wrapper runs.
Terminal errors without matches still fail; collected matches still survive.
No unresolved material assumption or new architectural boundary is introduced.

## ASCII UI preview

### UI-01: Existing result row

Entry: active task, command palette Contents tab, nonblank query, polling done.
Desktop and phone share this state-only correction; no geometry is changed.

```text
Earlier failure + later empty success:
Before: [Search failed. Edit the query or reopen search to retry.]
After:  [No content matches found.]
Final failure without matches: [Search failed. ...] (unchanged)
```

The translated state selection is required by AC-UI-TASK-WORKSPACE-CONTENT-SEARCH-001.10.
Spacing is illustrative. The existing palette remains the mobile exemplar.
The mobile-parity state-normalization exception applies: no navigation, touch,
scroll, layout or breakpoint behavior changes. Use focused hook/consumer tests.

## Tests and E2E evidence

`use-workspace-content-search.test.ts` owns AC .10 recovery and failure controls
with the actual hook and WS wrapper. Include initial/middle recovery, healthy
empty, all-failed, late terminal failure, mixed matches and cancellation.
Existing `workspace-content-search.test.tsx` owns translated consumer outcomes;
exercise the real hook-to-consumer chain in a focused integration suite as well.
No new mobile Playwright test or actual-backend claim: shared state-only logic
has no viewport-dependent branch. Existing browser flows live in
`e2e/tests/search/task-workspace-content-search.spec.ts` and are not claimed run.

## Work orders

- [x] [Task 01: Correct recovered empty searches](task-01-recover-empty-search.md)

## Verification results

See the [work-order receipts](task-01-recover-empty-search.md#results).
Causal RED produced five expected failures with 36 controls passing. The final
three-suite run passes all 41 tests after the one-line correction. Focused lint,
bounded typecheck, specifications and public-doc checks pass. Implementation is
complete; publication remains gated on parent validation and separate authorization.

## Risks

Clearing an error unconditionally at completion would hide terminal failure.
Preserve result accumulation even when another repository attempt fails.
