---
id: "02-authorized-image-reads"
title: "Read authorized question images"
status: complete
wave: 2
depends_on:
  - "01-question-image-contract"
plan: "plan.md"
requirements:
  - REQ-TASKS-QUESTION-IMAGES-002
  - REQ-TASKS-QUESTION-IMAGES-004
acceptance_criteria:
  - AC-TASKS-QUESTION-IMAGES-002.1
  - AC-TASKS-QUESTION-IMAGES-002.2
  - AC-TASKS-QUESTION-IMAGES-002.3
  - AC-TASKS-QUESTION-IMAGES-002.4
  - AC-TASKS-QUESTION-IMAGES-002.5
  - AC-TASKS-QUESTION-IMAGES-004.1
  - AC-TASKS-QUESTION-IMAGES-004.2
system_design:
  - ../../specs/tasks/system-design/question-image-previews.md
---

# Task 02: Read authorized question images

## Summary

Provide a read-only image route that resolves saved references from an authorized
bundle. Deliver bytes through the source executor with strict containment and limits.

## In scope

- Add the backend route and injected preview-reader interface from the design.
  Use durable bundle identities, `AuthorizeBundleAccess`, and lifecycle permissions.
- Add the authenticated agentctl image endpoint, bounded runtime client method,
  and backend adapter wiring. Reuse current workspace/source root ownership.
- Implement PNG/JPEG signature, dimension, and decode validation. Bound reads
  to 10 MiB plus one probe byte and images to 16 million pixels. Limit decoding
  concurrency to two. Preserve request cancellation and the 15-second deadline.
- Use root handles to prevent escaping links and replacement races. Reject
  non-regular files without blocking. Reuse supported-platform file-open patterns.
- Map errors to the documented closed code set. Return safe content headers.
  Treat unsupported older runtimes as unsupported, without a broad read fallback.

## Out of scope

Changes to generic editor reads, rich-output file blocks, arbitrary URLs, new
servers, uploads, archives, task lifecycle mutation, and image transforms.

## Acceptance

1. Only the authorized saved image in its source session can produce bytes.
   Cross-task/workspace IDs, mismatched sessions, parent bundles, and removed sources fail closed.
2. Local and tunneled runtime clients return validated bounded raster content.
   Unsupported runtimes, malformed images, large files, and deadlines yield typed errors.
3. Reading pending or terminal images never claims a clarification, sends a prompt,
   changes an answer, or falls back to host paths or generic file reads.

## TDD cases

Add `image_handlers_test.go::TestQuestionImageAccess` for authorized, unauthorized,
unknown, inconsistent, and parent bundles. Exercise the real route registration.
Assert no runtime method runs after an authorization failure. Cover revoked access
and readers who can see a question but lack `session.exec`.

Add `adapters_question_images_test.go::TestQuestionImageSourceSession`. Use two
sessions with identical filenames and distinct contents. Assert source ownership,
not selected-session or current-primary fallback. Exercise restart reconstruction.

Add process and agentctl API tests for valid PNG/JPEG, uppercase suffixes, mismatched
magic, corrupt bodies, zero/huge dimensions, exact byte boundary, growth during
read, traversal, Windows drive/UNC paths, home shortcuts, URL forms, and encoded
traversal treated as a literal path. Cover symlink escapes, safe contained links,
root/repository replacement during open, unknown sources, FIFO/device rejection,
read cancellation, and concurrency. Assert no bytes escape on any failure.

Add client tests for non-2xx responses, unsupported endpoint, malformed MIME,
oversized HTTP body, timeout, and cancellation. Assert zero calls to `file/content`
or shell/proxy fallback. Cover source roots outside the worktree only when attached.

## Verification

Run from the repository root. New test names use the `QuestionImage` prefix.

```bash
(cd apps/backend && go test ./internal/agentctl/server/process ./internal/agentctl/server/api -run 'QuestionImage|WorkspacePreview')
(cd apps/backend && go test ./internal/agent/runtime/agentctl -run 'QuestionImage|FileContent')
(cd apps/backend && go test ./internal/clarification ./internal/backendapp -run 'QuestionImage|AuthorizeBundle')
(cd apps/backend && go test -race ./internal/agentctl/server/process ./internal/clarification -run 'QuestionImage')
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the changed-Go lint helper on concrete changed files. If platform-specific
open code is necessary, add compile checks and platform tests for each affected
OS to Results. Do not mark platform safety proven by a Linux-only test.

## Files likely touched

- New `apps/backend/internal/clarification/image_handlers.go` and tests
- `apps/backend/internal/clarification/handlers.go`, route registration
- New `apps/backend/internal/backendapp/adapters_question_images.go` and tests, service wiring
- New `apps/backend/internal/agent/runtime/agentctl/client_question_images.go` and tests
- New `apps/backend/internal/agentctl/server/api/question_images.go` and tests, `server.go`
- New `apps/backend/internal/agentctl/server/process/question_images.go` and tests
- `apps/backend/internal/agentctl/server/process/manager_submodules.go`
- `apps/backend/internal/agentctl/server/process/manager_rescan.go`
- Shared internal image request/response types only if needed by those boundaries

## Dependencies

Task 01. Read the backend and agentctl scoped AGENTS files before editing.

## Risks

`resolveWorkspacePreviewRoot` provides a source-selection precedent, not a guarantee
against subsequent root replacement. Test actual handle containment. Source mounts,
Windows links, and FIFO opens need explicit implementation evidence.

## Parallelism

Sequential. This work owns the read boundary consumed by Task 03.

## Inputs

Design sections Authorized preview transport, Executor reader, Failures, and
Lifecycle and compatibility. Existing workspace preview and attachment validation tests.

## Results

Implemented and verified on Linux on 2026-10-24. Real PNG/JPEG, exact byte limit,
pixel bound, FIFO, symlink escape, source revocation, replaced-root,
missing-workspace, cancellation, source-session, authorization, and client error
cases pass. Redirect refusal prevents an agentctl response from fetching another
URL. No generic file-reader fallback exists.

All Go test commands in Verification passed. Source-session tests use different
image bytes at the same relative path and verify `session.exec` denial. Process
and clarification race checks passed. The route uses a `question_id` query value:
a RED test proved that existing slash-containing question IDs fail in a Gin path
segment. The design records this compatible correction.

Passed cross-compilation commands from `apps/backend`:

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/question-images-process-windows.test.exe ./internal/agentctl/server/process
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go test -c -o /tmp/question-images-process-darwin.test ./internal/agentctl/server/process
```

These binaries have not executed on Windows or macOS. They prove compilation,
not native filesystem behavior. Real remote executor evidence belongs to Task 04.

Changed-file lint used `PATH=/tmp/kandev-merge-tools:$PATH bash scripts/lint-go-changed`
with the concrete changed Go paths under clarification, MCP, backendapp, process,
agentctl API, runtime client, and common/imagepreview. It passed with zero issues.
The matching tool was golangci-lint 2.9.0 built with Go 1.26.6. Final test-only lint
also passed; log: `/tmp/question-images-final-go-lint.log`.

PR preparation on 2026-10-25 found a runtime import boundary violation in the
normal architecture hook. The adapter and its tests now use the existing
`internal/agent/runtime.AgentExecution` alias. The behavior is unchanged.
`go test ./internal/backendapp -run 'TestQuestionImage|TestClarification.*Image' -count=1 -timeout=3m`
passed after that correction.


PR review correction: workspace changes now update the image root with the tracker
root under the existing lock. Rebind, failed rebind, rollback, and rescan tests use
different image bytes at the same relative path. Both new tests failed before the
fix by returning bytes from the old root. After the fix, this command passed:

```bash
go test -trimpath -race ./internal/agentctl/server/process -run 'TestQuestionImage|TestRebindWorkspace|TestRescanRepositories' -count=1 -timeout=3m
```

Merge validation preserved upstream answer labels, process-manager state, and all
translation keys. The focused Go image tests passed across seven packages. The
five focused UI files passed (nine tests), as did typecheck, localization, catalog,
specification, and harness checks. Desktop browser validation passed (four tests); phone validation passed (two tests).
Remote CI and review results remain pending.

```bash
source /tmp/question-images-browser-env.sh
E2E_PORT_OFFSET=23 pnpm --dir apps/web e2e:run --host --no-build --project chromium tests/chat/question-image-previews.spec.ts tests/chat/clarification-choice-only.spec.ts
E2E_PORT_OFFSET=23 pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/chat/mobile-question-image-previews.spec.ts
```


CodeRabbit review correction: release the workspace lock after selecting and
opening the authorized root handle. File I/O and decode keep root-handle confinement
without blocking workspace updates. A gated decoder regression failed while the
lock covered decoding and passed after the change. The focused image race suite
passed with `go test -trimpath -race ./internal/agentctl/server/process -run TestQuestionImage -count=1 -timeout=2m`.
