---
id: "09-diff-finding-annotations"
title: "Render findings as anchored diff annotations, with stale handling"
status: done
wave: 6
depends_on: ["08-frontend-state-and-types"]
plan: "plan.md"
spec: "../../specs/agents/requirements/native-code-review.md"
---

# Task 09: Render findings as anchored diff annotations

Findings appear where the code is, in the same annotation surface as anchored comments.

## Inputs

- Spec **What** (findings render at their anchored line; advisory actions), the stale scenario, and the "not in current changes" failure row.
- `components/diff/use-diff-annotation-renderer.tsx` — the `AnnotationMetadata` union to extend.
- `components/diff/use-diff-viewer-state.ts` — `useWalkthroughSelection` is the exact shape for a store-driven annotation source; `buildAnnotations` shows how annotations anchor to `endLine`.
- `components/diff/comment-display.tsx` — card layout and action-button conventions.
- `apps/web/AGENTS.md` component limits (files ≤600 lines, components <200 lines) and `cursor-pointer` requirement.

## Work

1. `components/diff/review-finding-card.tsx` — severity chip (`blocker`/`major` destructive-toned, `minor`/`nit` muted), category badge, title, Markdown body via the existing comment Markdown renderer, optional `suggestion` in a read-only code block with an explicit "not applied automatically" affordance, and the three actions. `resolved` / `dismissed` render collapsed with an Undo action.
2. `components/diff/use-diff-annotation-renderer.tsx` — add `"review-finding"` to the metadata type union with a `finding` field, and render the card.
3. `components/diff/use-diff-viewer-state.ts` — `useReviewFindingAnnotations({ filePath, repo, diffHash })` reading the review slice for the active task, filtering to this file via `findingFileKey`, dropping stale findings that cannot be re-anchored, using `reanchorFinding` for the rest, and anchoring at `end_line` on the finding's `side`. Append to the annotation list next to the walkthrough annotation. Pass the file's current `diffHash` down from the review file list so the hook does not recompute it per render.
4. `components/review/review-file-findings-banner.tsx` — collapsible group rendered directly above a file's diff listing that file's stale findings and its findings whose anchor no longer exists, each with the same action set. This is the surface that guarantees a stale finding is neither dropped nor mis-anchored.
5. `components/review/review-diff-list.tsx` — thread `diffHash` and the banner into each file section; keep the file component under the size limit by extracting the banner rather than growing the section component.
6. `hooks/domains/review/use-send-finding-to-agent.ts` — builds the agent context payload from a finding (repository, path, line range, severity, title, body) and sends it through the same path `use-run-comment.ts` uses for a diff comment; the finding stays `open`.
7. `hooks/domains/review/use-finding-actions.ts` — resolve / dismiss / undo wrappers over `updateReviewFindingStatus` with optimistic slice updates and rollback on failure.

## Acceptance

- An open non-stale finding renders at its `end_line` in the correct file, in a multi-repository task only inside its own repository's file.
- A stale finding does not render inline at its stored line; it appears in the file's stale banner.
- A stale finding whose `anchor_text` still exists renders inline at the relocated line.
- Resolve collapses the card and decrements the open count; failure rolls the optimistic update back.
- Send-to-agent produces the expected context payload and leaves the finding `open`.

## Verification

```
cd apps/web && pnpm vitest run components/diff/use-diff-viewer-state.test.ts components/diff/review-finding-card.test.tsx hooks/domains/review
cd apps/web && pnpm run typecheck && pnpm lint
```

## Files likely touched

`components/diff/{review-finding-card.tsx,use-diff-annotation-renderer.tsx,use-diff-viewer-state.ts}`, `components/review/{review-file-findings-banner.tsx,review-diff-list.tsx}`, `hooks/domains/review/{use-send-finding-to-agent.ts,use-finding-actions.ts}`, plus tests.

## Output contract

Summary, files changed, tests run with results, blockers, risks, `status: done`, plan checkbox.
