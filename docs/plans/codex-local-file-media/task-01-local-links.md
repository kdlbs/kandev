---
id: "01-local-links"
title: "Restore local Markdown file resolution"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-COMMENT-MARKDOWN-001
acceptance_criteria:
  - AC-UI-COMMENT-MARKDOWN-001.10
  - AC-UI-COMMENT-MARKDOWN-001.11
system_design:
  - ../../specs/ui/system-design/comment-markdown.md
---

# Restore local Markdown file resolution

## Scope

Undo publication flow; fix absolute-path handling and session-local image reads.
Preserve aliases, external URLs, invalid-path rejection and existing write policy.

## UI preview

Same composition as [the plan](plan.md#ui-preview): link opens the existing file
viewer; image opens existing inspection. Phone retains full-height navigation.

## Acceptance and tests

Resolver/component regression tests prove original absolute paths reach openFile.
Hook tests prove workspace.file.get uses the rendered session, including stale/error
responses. Desktop/mobile local-markdown-links.spec.ts exercise original temporary
Markdown and PNG links directly, without publication or synthetic file-serving routes.

## Verification

```bash
make fmt
make typecheck test lint
(cd apps/web && pnpm test lib/markdown/file-link-target.test.ts components/shared/markdown-components.test.tsx hooks/use-markdown-image.test.tsx components/shared/memoized-markdown.test.tsx components/shared/task-markdown-file-link-provider.test.tsx)
(cd apps/web && pnpm e2e:run e2e/tests/chat/local-markdown-links.spec.ts --project=chromium)
(cd apps/web && pnpm e2e:run --no-build e2e/tests/chat/mobile-local-markdown-links.spec.ts --project=mobile-chrome)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Results

Implementation complete: 73 focused frontend regressions, desktop and mobile direct
local-link E2E, typecheck and static checks passed. No publication call is involved.
Full repository checks retain existing backend baseline failures, separately recorded
in the parent plan; no push/deployment. No subagents or new worktrees.
