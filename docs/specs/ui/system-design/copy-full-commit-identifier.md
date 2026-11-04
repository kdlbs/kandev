---
status: current
system: ui
requirements:
  - REQ-UI-COPY-COMMIT-ID-001
---

# Copy Full Commit Identifier System Design

## Boundaries and mapping

This is a UI-owned, source-neutral local action in the existing commit-detail presentation. [Commit file navigation](commit-file-navigation.md) retains surrounding composition, and [clipboard feedback](clipboard-feedback.md) owns the shared helper and feedback lifecycle. The source's existing target remains authoritative.

| Criteria | Design section |
| --- | --- |
| AC-UI-COPY-COMMIT-ID-001.1, .2, .5 | Header integration |
| AC-UI-COPY-COMMIT-ID-001.3 | Clipboard and feedback |
| AC-UI-COPY-COMMIT-ID-001.4 | Responsive interaction |

## Header integration

`CommitDetailContent` in `apps/web/components/task/commit-detail-content.tsx` passes `target.sha` to `CommitHeaderMeta`, which currently displays its first seven characters. Add a small separate button component and make a minimal consumer edit at this existing nonempty identifier location. Pass the current full string directly at activation; do not fetch, validate, transform, or persist it. The same path handles local and GitHub targets. Preserve existing header rendering conditions and every surrounding metadata, index, diff and navigation path. No backend, Git mutation, API, session, or workspace operation is introduced.

## Clipboard and feedback

Use `useCopyToClipboard` from `apps/web/hooks/use-copy-to-clipboard.ts`, which delegates to `copyToClipboard` and reports success only after its final boolean result. Reuse localized `task:copyIdentifier` and `task:copied`; expose an accessible name and an accessible success announcement. Keep the action usable while feedback is displayed. A failure does not create success or change an earlier success deadline, per the existing feedback contract. Do not modify the shared hook or utility. The active identifier supplied on each activation must reflect the current rendered target. Key the local feedback instance by identifier so a target change, including an in-flight completion from the previous target, cannot claim success for the new identifier; do not change the shared hook lifecycle.

## Responsive interaction

Keep the short identifier and explicit action inline in the existing detail header, wrapping safely when space is constrained. The existing commit detail phone sheet and its index action are the nearest mobile exemplar: the sheet retains its current scroll owner and visible touch controls. Use the shared responsive breakpoint/control sizing conventions: 28px desktop and at least 44px square for phones (including fine-pointer widths below 768px) and coarse pointers. The control stays visible without hover and does not overlap neighbors, clip, or create document overflow. Existing sheet navigation, scroll and safe-area behavior are unchanged.

## Verification boundary

Mounted `CommitDetailContent` coverage must exercise the real consumer with distinct full and seven-character values, both target sources, target updates, failure/retry, and accessible feedback. Dedicated desktop and configured mobile-chrome E2E use isolated real commit details, actual clipboard contents, keyboard/touch activation, measured geometry and synthetic screenshots; a narrow fine-pointer phone regression verifies the width-based condition. No production or permanent test changes occur before the later execution release.

## Implementation plan

- [Copy full commit identifier](../../../plans/copy-full-commit-identifier/plan.md)
