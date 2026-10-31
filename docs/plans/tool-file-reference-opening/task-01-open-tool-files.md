---
id: "01-open-tool-files"
title: "Open tool file references"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TOOL-FILE-OPEN-001
acceptance_criteria:
  - AC-UI-TOOL-FILE-OPEN-001.1
  - AC-UI-TOOL-FILE-OPEN-001.2
  - AC-UI-TOOL-FILE-OPEN-001.3
  - AC-UI-TOOL-FILE-OPEN-001.4
system_design:
  - ../../specs/ui/system-design/tool-file-reference-opening.md
---

# Task 01: Open tool file references

## Summary

Remove the unsupported extension heuristic for the existing read/search file
reference control and retain its explicit inactive cases. Prove the behavior
through real consumers and through both existing editor destinations.

## In scope

- Apply the [design](../../specs/ui/system-design/tool-file-reference-opening.md)
  to `FilePathButton`; preserve all caller paths and ownership.
- Independently author causal RED/GREEN component coverage using actual read,
  expanded search, shared button and real read hook. Check exact original button
  title/path, opener invocation and read cursor position keyed by session; use
  the existing pending-cursor API as an observable, not a mock of the hook.
- Include `/workspace/Dockerfile`, `/workspace/Makefile`, `/workspace/LICENSE`,
  `/workspace/script`, dotted and hidden controls, `.` and trailing slash,
  whitespace, absent opener, and inert search scope. Assert one invocation per
  click, workspace alias normalization and a positive read line.
- Seed real extensionless workspace files and read/search messages in focused
  desktop and mobile Playwright tests. Expand search and open both references;
  assert file content on both destinations and the visible read line on phone.
  Use an isolated task/session and fixture cleanup.

## Out of scope

Filesystem probing, protocol/provider changes, selector parsing, generalized
classification, new copy, unrelated editor and layout changes, and public
feature documentation.

## Acceptance

1. Eligible extensionless and dotted references activate once; blanks, explicit
   directory markers, search scope and missing opener remain inactive.
2. Existing path, cursor line and session/workspace selection behavior remain
   intact, with no new file-service or error policy.
3. Desktop click and phone tap open the intended actual files. The phone read
   reaches its visible line; the component test proves the session-scoped request.

## ASCII UI preview

`UI-01: Tool references`, task chat, expanded search (see the
[full desktop and phone preview](plan.md#ascii-ui-preview); criteria 001.1-001.4):

```text
Read [Dockerfile] :20
Found files (expand)
  [LICENSE]
  folder/
```

Desktop click opens the existing editor; phone tap opens the existing viewer.
The current chat and result scroll owners remain. Brackets denote actions.

## Verification

After separate implementation and exclusive HEAVY grants, install first using
the pinned toolchain. Use `/tdd`, `/e2e`, and `/mobile-parity`. Create the
three test files below before production change; run each focused command and
record causal failures (not zero collection, setup or timeout), then implement
and rerun to green. The managed desktop command builds fresh artifacts; the
following mobile command reuses that same unchanged build. Run serially.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH"
export GOTOOLCHAIN=local
export GOFLAGS='-trimpath -tags=fts5 -p=1'
export GOMAXPROCS=2
export GOMEMLIMIT=512MiB
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/chat/messages/tool-file-reference-opening.test.tsx)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/task/chat-tool-file-opening.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --host --shards 1 --no-build --project mobile-chrome tests/task/mobile-chat-tool-file-opening.spec.ts -- --retries=0)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/chat/messages/file-path-button.tsx components/task/chat/messages/tool-file-reference-opening.test.tsx e2e/helpers/tool-file-opening.ts e2e/tests/task/chat-tool-file-opening.spec.ts e2e/tests/task/mobile-chat-tool-file-opening.spec.ts)
(cd apps/web && pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Do not force the Vitest `browser` project: the new suite defaults to
`browser-locales`. Verify collected test names and counts. If the desktop or
mobile rendered RED needs an independent pre-fix build, run the managed desktop
build before the first desktop run and use `--no-build` only while its source
is unchanged; rebuild after the fix. Run scoped lint and typecheck once after
green. Record each exit and actual join, including failures. Use a unique
`KANDEV_RUN_QUIET_DIR` for each runquiet invocation if runquiet is used.
Never overlap the commands or install with another heavy job.

## Files likely touched

- `apps/web/components/task/chat/messages/file-path-button.tsx`
- `apps/web/components/task/chat/messages/tool-file-reference-opening.test.tsx`
- `apps/web/e2e/helpers/tool-file-opening.ts`
- `apps/web/e2e/tests/task/chat-tool-file-opening.spec.ts`
- `apps/web/e2e/tests/task/mobile-chat-tool-file-opening.spec.ts`
- This plan and owning specification pair for status/results.

## Dependencies

None. Implementation and the HEAVY lease were separately released.

## Risks

Unmarked directories and stale results cannot be classified from the payload;
they follow existing file-open failure handling. Keep the selector's existing
comma and colon semantics. Actual environment or browser setup failures are
not valid RED evidence.

## Parallelism

Sequential.

## Inputs

Owning requirement and system design; `ToolReadMessage`, `ToolSearchMessage`,
`useOpenFileAtLine`, `chat-read-multi-file.spec.ts`, and
`mobile-file-viewer.spec.ts`.

## Results

The component RED had four intended extensionless failures and eight passing
controls; GREEN passed 12 tests. Desktop and phone RED each showed both paths
as inert text. After the fix, desktop search passed and desktop read opened the
correct file but did not visibly reach line 300. The matched dotted `control.txt`
opened yet showed lines 1-34 instead of 300 as well. Its deeper cause is unknown;
this existing desktop cursor visibility limitation is deferred without changing
editor or cursor production code. Phone GREEN passed both flows, including the
visible line 300. ROOT reviewed this scope precision in
`child014/desktop-cursor-scope-adjudication.json`. Desktop read after scope
precision passed one focused run (handle 87882); search passed in the earlier
post-fix run (handle 69851). Mobile passed both (handle 96739). The component
run passed 12 tests (handle 15691). Scoped ESLint passed (handles 95435 and
10999). The frozen install passed (handle 13673). The first typecheck exceeded
a self-imposed 3 GiB V8 heap (handle 64067, exit 134); after the resource
checkpoint the 6 GiB bounded canonical typecheck passed (handle 44657).
All listed handles were actually joined. Catalog, specification lint and actual
changed-path documentation coverage passed; public documentation is unchanged
because no new public feature, terminology or workflow was added. Full command
logs and receipts remain in the task evidence directory.
