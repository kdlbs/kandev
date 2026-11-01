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

Remove the unrequested publication implementation in f527dff67 without rewriting
history. Reuse ADR 0016, workspace.file.get, current file actions and image viewers.
Keep repository aliases, malformed-path rejection and read/write boundaries.

## UI preview

Desktop and phone share the existing transcript entry point:

```text
[Local report] -> existing file viewer -> Markdown preview
[Local image]  -> existing image inspection
[Unavailable] for missing or undecodable images
```

Phone uses the existing full-height file/image viewer; no new navigation.

## Work orders

- [x] [Task 01](task-01-local-links.md)

## Verification results

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
