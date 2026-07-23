---
id: "01-link-foundation"
title: "External VCS link foundation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/external-vcs-file-links.md"
---

# Task 01: External VCS link foundation

## Acceptance

- GitHub, self-hosted GitLab, and Azure DevOps file URLs are built from credential-free repository metadata with correct provider routing and URL encoding.
- Published review branches, base fallback, added/deleted/renamed rules, multi-repo identity, and repeated-repository ambiguity match the approved spec.
- A reusable provider-branded semantic link control is available in compact desktop and 44px touch sizes, while applicable linked-review metadata hydrates once per task rather than once per file.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- lib/utils/external-vcs-file-url.test.ts hooks/domains/workspace/use-external-vcs-file-link.test.tsx components/editors/external-vcs-file-link.test.tsx
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/types/http.ts`
- `apps/web/lib/utils/external-vcs-file-url.ts`
- `apps/web/lib/utils/external-vcs-file-url.test.ts`
- `apps/web/hooks/domains/workspace/use-external-vcs-file-link.ts`
- `apps/web/hooks/domains/workspace/use-external-vcs-file-link.test.tsx`
- `apps/web/components/editors/external-vcs-file-link.tsx`
- `apps/web/components/editors/external-vcs-file-link.test.tsx`
- `apps/web/components/task/task-page-content.tsx`

## Dependencies

None.

## Inputs

- Spec: `What`, `Failure modes`, and all provider/revision scenarios.
- Plan: `External URL contract and task metadata` and `Mobile design contract`.
- Architecture: ADR `2026-07-20-provider-neutral-remote-repositories`, ADR `2026-07-20-repository-provider-origin-identity`, and ADR `0013-multi-branch-tasks`.
- Patterns: `useRepository`, `useTaskPR`, `useWorkspaceMRs`, `useAzureDevOpsTaskPullRequests`, and existing external anchors with `noopener noreferrer`.

## Output contract

Report summary, files changed, tests/commands with results, blockers, security risks, and any divergence. Update only this task file's `status` to `in_progress` at start and `done` after acceptance and verification pass; do not edit `plan.md`.
