---
created: 2026-11-01
status: implemented
requirements:
  - REQ-UI-COMMENT-MARKDOWN-001
system_design:
  - ../../specs/ui/system-design/comment-markdown.md
legacy_specs: []
---

# Local Markdown file resolution correction

## Scope and approach

Remove the unrequested publication implementation without rewriting history. Reuse
ADR 0016, workspace.file.get, current file actions and image viewers. Keep repository
aliases, malformed-path rejection and read/write boundaries.

## UI preview

Desktop and phone share the existing transcript entry point:

```text
[Local report] -> existing file viewer -> Markdown preview
[Local image]  -> existing image inspection
[Unavailable] for missing or undecodable images
```

Session images use the current-file reader. Shared Markdown keeps browser image
rendering when no session context exists. Repeated concurrent reads share one request.
Phone uses the existing full-height file/image viewer; no new navigation.

## Work orders

- [x] [Task 01](task-01-local-links.md)

## Initial implementation verification

No backend, MCP, attachment/storage or public file-return guidance changes relative
to edebfa524. The previous publication flow is removed by a new correcting commit.

- Resolver, shared Markdown, image hook and context/cache regressions: 73 passed.
- Desktop Chromium direct local Markdown/PNG flow: passed.
- Mobile Chrome direct local Markdown/PNG flow: passed, including full-height viewer.
- Missing and corrupt images show unavailable states; original links need no publishing.
- make fmt and final make typecheck: passed.
- web/harness/spec/architecture lint, catalog validation and diff checks: passed.
- make typecheck test lint was run. The full backend suite hit the previously
  documented npm configuration/macOS cache baseline failures and was stopped after
  recording them. Backend code is unchanged from the pre-repair baseline. These
  failures are not reported as passed, and the branch is not pushed or deployed.

## Review remediation verification

- `cd apps && pnpm --filter @kandev/web test -- --run hooks/use-markdown-image.test.tsx components/shared/markdown-components.test.tsx lib/markdown/file-link-target.test.ts` passed: 67 tests.
- `cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck` passed.
- `cd apps/web && pnpm e2e:run e2e/tests/chat/local-markdown-links.spec.ts` passed: 1 test.
- `cd apps/web && pnpm e2e:run --no-build --project mobile-chrome e2e/tests/chat/mobile-local-markdown-links.spec.ts` passed: 1 test using the desktop run's production build at the same source revision.
- `python3 scripts/list-docs.py validate` passed.
- `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` passed.
