---
id: "01-asset-delivery"
title: "Asset delivery and import boundaries"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WEB-ASSET-DELIVERY-001
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-004
acceptance_criteria:
  - AC-PLATFORM-WEB-ASSET-DELIVERY-001.1
  - AC-PLATFORM-WEB-ASSET-DELIVERY-001.2
  - AC-PLATFORM-WEB-ASSET-DELIVERY-001.3
  - AC-PLATFORM-WEB-ASSET-DELIVERY-001.4
  - AC-UI-SESSION-REFRESH-EFFICIENCY-004.2
system_design:
  - ../../specs/platform/system-design/web-asset-delivery.md
  - ../../specs/ui/system-design/session-refresh-efficiency.md
---

# Task 01: Asset delivery and import boundaries

## Summary

Serve compressed immutable assets from the native server and preserve original decoded bytes. Defer an editor import only if the baseline trace proves that a closed surface loads it unnecessarily.

## In scope

- Implement and test deterministic smaller-only gzip siblings through the Vite build lifecycle.
- Implement quality-aware representation selection, MIME/cache headers, GET/HEAD, conditional/range behavior, identity fallback, and encoded error handling in the existing native handler.
- Trace packaging paths and smoke-test the production asset tree. Keep the shell private and API/WS handling unchanged.
- Use import initiators and chunk graphs to locate optional editor code. Move only a proven eager boundary to an existing lazy surface with unchanged loading/focus behavior. Record no change if no safe unused boundary exists.

## Out of scope

Live instance mutations, unrelated refactors, new task/session creation, deployment,
and changes outside this package's contracts. No delegation is authorized.

## Acceptance conditions

1. Compression negotiation tests pass and every served gzip asset decodes to the original bytes; all build entry points await completed siblings.
2. Packaged native assets load Threads and task detail on desktop and phone, with correct identity fallback, caching, HEAD, and ranges.
3. Any import deferral has a measured reduction before first use and a real editor-open regression test; report cold byte gains separately from warm CPU.

## Verification commands

Run the listed commands sequentially. New test files are implementation outputs;
they must exist before these commands count as verification. Use the guarded
E2E runner with fresh known artifacts. Capture RED/GREEN evidence for changed
logic and apply scoped lint to touched files.

```bash
cd apps/web
pnpm exec vitest run scripts/precompress-assets.test.mjs
pnpm exec vitest run components/diff/diff-viewer-resolver.test.tsx components/task/file-editor-content.test.tsx
pnpm run build:vite
NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck
cd ../backend
go test ./internal/webapp
cd ../web
pnpm e2e:run --project chromium tests/performance/lazy-monaco.spec.ts
pnpm e2e:run --project mobile-chrome tests/performance/mobile-lazy-monaco.spec.ts
pnpm e2e:run --project chromium tests/git/diff-refresh-continuity.spec.ts
pnpm e2e:run --project mobile-chrome tests/git/mobile-diff-refresh-continuity.spec.ts
```

## Likely files

- `apps/web/vite.config.ts`
- `apps/web/scripts/precompress-assets.mjs` and `.test.mjs` (new)
- `apps/backend/internal/webapp/handler.go` and handler tests
- Production asset-copy/archive paths identified during the packaging audit
- Only the optional editor import boundary demonstrated by work order 01

## Dependencies and parallelism

Independent retained extraction. No new investigation or delegation.

## Inputs

Read the requirements and designs referenced in frontmatter, the
[plan](plan.md), and the original reviewed benchmark protocol. Preserve existing
companion-plan ownership and historical results described by the plan.

## Risks

Wrong variant headers can break caches or execute compressed bytes as JavaScript. Sidecars omitted from packaged assets erase the benefit. Lazy imports must not delay an already-visible composer or lose selection.

## Execution results

Main `d17d84a8084b` fails the ported HTTP negotiation and
non-seekable HEAD/range/conditional regressions. A no-op build helper fails
both compression output and missing-directory error propagation tests.
A fresh browser build with main's eager imports fails the closed-surface
Monaco request assertion. The same run opens the file and accepts keyboard
input. Initial attempts stopped on inherited host configuration, missing
Chromium libraries, and an outdated baseline artifact; they are setup failures,
not behavioral RED evidence. The successful RED uses the isolated managed
runner with the host startup variables removed and the Nix browser libraries.

Node compression tests, Go webapp tests, 8 component tests, scoped Go/web lint,
and web typecheck pass. Typecheck needs the same 8 GB Node heap as CI; the
initial default-heap run exhausted memory. All six candidate browser tests pass: one lazy-loading case and two existing
diff-continuity cases per viewport. Desktop accepts keyboard input after first
open. Phone retains its native CodeMirror viewer and has no page overflow.
Both viewports show compressed script transfer. Desktop and phone screenshots
were inspected; the desktop capture maximizes the editor to exclude the host
terminal prompt. A final capture-only desktop rerun also passes.

The production `build:vite` audit verifies all 549 gzip siblings: 60,333,971
identity bytes and 15,364,427 encoded bytes. Every sibling decodes exactly,
regenerates deterministically, and is smaller than its original. These are
whole-build totals, not a page-load or CPU measurement. Recursive Unix copy,
Windows robocopy, and Go embedding retain sidecars. After `make sync-embedded-web`,
`TestEmbeddedCompressedAsset` passes against the generated asset tree. The full
webapp package also passes after that sync.

Scoped Go/web lint, specification catalog validation, and specification lint
pass. Commit hooks and remote CI/review results remain delivery evidence.

PR review exposed a multipart-range response that retained `Content-Encoding:
gzip` on an unencoded multipart envelope. A two-range regression fails on both
seekable and non-seekable sources before the fix. Gzip multi-range requests now
receive the complete compressed representation without mutating the request.
The webapp package and scoped Go lint pass. Added negotiation cases preserve
implicit identity preference and reject explicitly excluded identity responses
for assets without a compressed representation, including fonts.

CI discovered the compression test with Vitest, but its original Node test API
registered no Vitest suite. The exact Vitest command reproduces that failure.
The test now uses Vitest's test and cleanup APIs; both compression cases pass
through the same project selection used by CI.

A later review reproduced an asset-length header on bodyless 412 responses.
Eight cases cover failing `If-Match` and `If-Unmodified-Since` preconditions for
gzip/identity GET and HEAD. All fail before the fix and pass after clearing the
length on 412. The full webapp package and scoped Go lint pass. The existing
translated lazy-editor fallback now exposes a status region. The phone browser
check observes late imports after the native viewer opens before asserting that
Monaco remains unloaded.
Both lazy-loading browser cases and scoped web lint pass after these changes.

## Documentation impact

Public docs need no change: compression and deferred imports require no new
operator action, settings, navigation, or terminology. The platform contract
and this execution record document the internal delivery behavior.
