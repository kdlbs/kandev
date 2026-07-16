# Product Story Recipes

Each story needs: stable opening, one understandable action chain, visible payoff, settled ending. Keep copy natural and domain-specific.

## Plan

Show planning as collaboration, not a static artifact:

1. Open real Plan mode.
2. Produce an authored plan through the real Kandev MCP plan path.
3. Select a plan step and add a natural comment.
4. Run the comment so a queued mock-agent turn updates the plan through `update_task_plan_kandev`.
5. Show the revised plan, then the real Implement action.

Do not paint a fake plan card or claim a plan exists before the product creates it. Desktop and mobile need separate native paths.

## Coordinate

Seed a legible board with 3-4 named stages and enough cards to communicate parallel work. Use distinct task titles and believable state distribution. Show one move, stage switch, or task handoff.

Desktop can show the board overview. Mobile should navigate native stage tabs or swipe behavior; keep selected stage label and complete cards inside frame.

## Prepare

Open real task intake. Show agent/model and executor choice using profiles created through the API. Keep the complete popover/sheet visible. Do not force aliases or fabricate unavailable executor types.

If demonstrating setup scripts, repositories, branches, or worktrees, seed valid values and verify the chosen profile persists.

## Run And Inspect

For a controlled clean story:

1. Create the task without `start_agent`.
2. Seed one session as `RUNNING`.
3. Add two distinct progress phases using separate thinking/tool/result boundaries.
4. Show readable implementation and test evidence tied to the disposable repository.
5. Move the same session to `IDLE` or completed state.
6. Inspect the resulting file/change through real Files, editor, or Changes UI.

This avoids generic fixture-agent output while preserving truthful product state. Do not seed a claim that the repository cannot support.

## Review And Repair

Seed a real changed-file set plus mock PR/check/review state. Show:

1. changed-file navigation;
2. readable diff;
3. anchored or line-level feedback;
4. the real fix/repair action;
5. agent Read/Edit/Bash or equivalent evidence;
6. passing focused tests or a coherent repaired diff.

Desktop should use the current main Changes/review surface. Mobile should use the native Changes panel and `MobileDiffSheet`, then native Chat for repair when that is the current responsive route. Never open a known overflowing desktop modal on mobile merely to mirror desktop choreography.

## Integrations

Prefer the real Settings integration index or real task-import surface. Mock provider health/data behind it. Famous logos may carry the visual story, but they must come from actual product assets and current supported entries.

## Editor And Terminal

Use actual files and shell commands in the disposable repository. Initialize a fictional shell identity and working directory before recording. Ensure canvas/WebGL terminal pixels are visible in the chosen capture path; verify the raw frame rather than assuming the browser recorder captured them.

## Static Screenshots

Use the same seeding and truthfulness rules. Capture a settled pointer-free frame at native resolution. Crop only after preserving a full raw source. Keep dense UI text readable and avoid `cover` when it removes controls needed to understand the state.

## Alternate Versions

When only framing, zoom, poster, or pacing changes, reuse the approved clean raw master. Re-seed and recapture when visible data, UI behavior, viewport, story route, or feature state changes.
