# Transcript navigation E2E consolidation

Scope: `chat/last-prompt-scroll.spec.ts`. The shared helper starts a real agent,
sends two distinct user prompts and seeds intervening/trailing agent messages.
Retain those real prompts, their content, the filler counts and both navigation
entry points. No production or shared helper change is proposed.

## Candidates

| Disposition | Existing case | Keeper and preserved proof |
| --- | --- | --- |
| Consolidate | `scroll-to-start button jumps back to the first prompt` | Add a named step to `scroll-to-last-prompt button jumps back to the top of the last prompt`. Return to the same final filler before the moved assertions. Preserve the first prompt initially outside the viewport, the visible start button, its actual click, the first prompt entering the viewport and the button hiding. Keep the anchored bar disabled for both actions. |
| Consolidate | `scroll-to-start drains older pages until it finds the true first prompt` | Add a named step to `keeps task opening bounded before last-prompt navigation`. Both histories use 80 trailing fillers. Install a new request recorder and reload before the moved scenario, preserving the missing first row, visible button, real click, more than one older-page request, true first-prompt viewport assertion and hidden button. Earlier around-window loads must not satisfy this fresh observation. |
| Retain | Other eight cases | Disabled preferences, unloaded prompt navigation through both desktop controls, anchored bar expansion, Markdown/mention rendering, partial visibility, directional controls, mid-scroll streaming and proportional height each exercise a distinct state or rendering contract. All mobile cases remain separate. |

The owner is `task-chat-panel.tsx`, with `TranscriptNavGroup`, the two
scroll-button components, `useDrainOlderMessages` and `usePendingScrollToStart`.
The last-prompt path chooses direct scrolling or a bounded around-window load;
the start path drains older history. These are distinct behaviors even when
their setup is shared. Relevant contracts are the UI last-prompt pinning,
pinned-prompt availability and task-prompt transcript visibility documents.

Navigation originated in `3eb8e4a59c` (#1999). Pagination-to-first-prompt was
repaired in `3ca388ebe4` (#3002), and unloaded last-prompt availability in
`44e431b7c6` (#3987). The 120-filler unloaded-prompt case remains independent
and is not replaced by the 80-filler consolidation.

Risks: the first click changes scroll position and possibly the loaded window.
Return to the original final filler for the basic journey; reload and use a new
request recorder for the paginated journey. Keep all original assertions and
timeouts. No obsolete helper or production seam is removed.

## Verification

Complete one-worker, retries-disabled baseline and candidate runs are required.
Compare original case titles against retained cases plus named steps. Hosted
runtime acceptance remains separate from this local sample.

The complete baseline passed 12/12 in 197.071 seconds; the candidate passed
10/10 in 166.209 seconds. Both used the same host and builds, one worker and
zero retries/skips. All twelve original case names appear as retained titles
or the two executed named steps. The single pair saves 30.863 seconds (15.66%)
for this file, not the whole suite. Changed-file formatting and ESLint pass.
Raw reports and the identity/timing receipt are retained under ignored
`apps/backend/.build/ci-half/e2e-focus/prompt-scroll-*`.
