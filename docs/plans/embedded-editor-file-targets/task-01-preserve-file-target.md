---
id: "01-preserve-file-target"
title: "Preserve the resolved embedded editor file target"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-EMBEDDED-EDITOR-TARGET-001
acceptance_criteria:
  - AC-UI-EMBEDDED-EDITOR-TARGET-001.1
  - AC-UI-EMBEDDED-EDITOR-TARGET-001.2
  - AC-UI-EMBEDDED-EDITOR-TARGET-001.3
  - AC-UI-EMBEDDED-EDITOR-TARGET-001.4
  - AC-UI-EMBEDDED-EDITOR-TARGET-001.5
  - AC-UI-EMBEDDED-EDITOR-TARGET-001.6
system_design:
  - ../../specs/ui/system-design/embedded-editor-file-targets.md
---

# Task 01: Preserve the file target

## Summary and admission

Implement the private producer/consumer correction as one vertical slice with
real service and hook-to-wire regression evidence. ROOT explicitly released this order after actual design END and concrete
four-file review; implementation and scoped behavioral conformance are complete. A design handoff is not an implementation or heavy grant.
Read the [manifest](plan.md), owning pair and scoped guidance first; mark this
order in progress only after that release. No delegation or extra sessions.

## Owned scope

- `apps/backend/internal/editors/service/service.go`: encode path-only `goto`
  and independent positive coordinates after existing canonical resolution.
- New `embedded_file_targets_test.go` in that package: actual `OpenEditor`
  service fixtures and controls; do not grow the already-large legacy test file
  beyond its lint ceiling. Update only affected expectations in `service_test.go`.
- `apps/backend/internal/editors/service/testdata/embedded-editor-file-targets.json`:
  one shared expected input/resolved-tuple/response oracle.
- `apps/web/hooks/use-open-session-in-editor.ts`: parse query once, no colon
  splitting; use the returned canonical file. No API/options changes.
- New `apps/web/hooks/use-open-session-in-editor.wire.test.tsx`: real
  hook/request/provider/API/socket protocol; import the shared JSON using
  `../../backend/internal/editors/service/testdata/embedded-editor-file-targets.json`.
  `resolveJsonModule` is already enabled. Update existing hook test responses
  to the new producer format and run both suites together.
- Update this order and manifest results/status, and the paired spec lifecycle
  after all exact checks pass and conformance is established.

The primary is not alone in the codebase. Preserve others' edits and adapt to
them; never revert foreign changes or mutate shared dependencies/caches/refs.

## Exclusions

All [requirement exclusions](../../specs/ui/requirements/embedded-editor-file-targets.md#exclusions)
apply. Do not change router/DTO/API/WebSocket schema, availability policy,
registry/settings, request/toast lifetime, file-tree entry-point wiring,
native CLI/runtime-root behavior or the unrelated dot-prefix guard. Do not
introduce a generic URI module or legacy-grammar fallback. No native launch,
browser/build/E2E/full suites. Pure data/mobile exception requires no preview.

## Acceptance

1. Real producer output and real hook dispatch agree on exact canonical tuples
   for every supported shared oracle row, including numeric-colon no-coordinate
   paths and single-decoding controls (AC 001.1-001.3).
2. Existing no-file/directory/session/worktree/error/external-editor controls
   retain their observable request/result behavior, and raw caller paths never
   replace returned canonical targets (AC 001.4-001.6).
3. Record causal RED then GREEN and all exact checks with actual joined receipts;
   retain wire/native limitations and publish only under the manifest's later
   delivery gates. Test/setup/transport/resource failures cannot stand in for RED.

All AC suffixes refer to `AC-UI-EMBEDDED-EDITOR-TARGET-001`.

## TDD sequence and oracle

After release, inspect current source against the two audited blobs. If relevant
source differs, checkpoint ROOT; do not reset/rebase/replay to recreate evidence.
Accepted ROOT proof remains immutable and is not the permanent RED run.

Write `TestOpenEditor_EmbeddedFileTargets` in the new Go test file using the
real `Service.OpenEditor`, narrow repository/settings fixtures, and the shared
JSON expected response. Add `TestOpenEditor_EmbeddedTargetControls` for service
admission/root/selection/fallback/external controls. Reuse existing test helpers
only where their methods actually satisfy the exercised interfaces; no embedded
missing-method trick may provide a RED. Record each relevant behavioral mismatch.

The shared oracle includes exact request options, canonical file, effective line
and column, and expected URL. Minimum file rows:

| Input path | Input line/column | Expected canonical tuple |
| --- | --- | --- |
| `src/plain.ts` | 17/5 | `src/plain.ts`,17,5 |
| `src/name:part.ts` | 17/5 | `src/name:part.ts`,17,5 |
| `notes:2026` | 0/0 | `notes:2026`,0,0 |
| `src/sub/../name:part.ts` | 17/5 | `src/name:part.ts`,17,5 |
| `notes:2026` | 17/0 | `notes:2026`,17,0 |
| `notes:2026` | 0/5 | `notes:2026`,0,0 |
| `src/space name.ts` | 0/0 | exact path,0,0 |
| `src/café.ts` | 17/5 | exact Unicode path,17,5 |
| `src/literal%3A%25.ts` | 0/0 | literal percent sequences,0,0 |
| `src/plus+query&=?#name.ts` | 17/5 | exact punctuation,17,5 |

Go checks actual default and explicitly selected worktree/association-ID
resolution and repository fallback with real service calls. Invalid selection
must fail; root path `.` and no-file inputs return a bare sentinel; unsupported
executor, missing session/workspace, disabled editor and escaping path still fail.
Use a hosted editor for service external response controls, never spawn commands.
POSIX legal-colon names are the behavioral scope; keep string/oracle controls
platform-neutral where possible and introduce no blanket skips or native Windows
support claims.

Frontend suite name: `embedded editor target wire`. Mount real
`useOpenSessionInEditor` inside real `ToastProvider`; keep real `useRequest`,
`openSessionInEditor`, `fetchJson`, `openFileInVscode`, `WebSocketClient` and
connection singleton. Defer/substitute only HTTP/socket wire. Assert actual
POST path/body including worktree selection, one exact `vscode.openFile` frame,
and returned response/state. A caller canonicalization row must differ from the
returned target. Ordinary/no-coordinate/line-only cases are positive controls.

Additional real hook controls: missing session means null/no HTTP/no RPC; bare
sentinel and marked directory mean no RPC; rejected HTTP returns null, real
localized toast and error state, no RPC; external URL goes through actual
`window.open` interception and no embedded RPC; empty response remains intact.
Socket failures retain swallowed/logged behavior, not a new toast policy.
Restore the prior client/globals, disconnect and clear/drain owned timers.
Null Dockview is allowed, with explicit no-rendering qualification.

Run the new Go and wire suites before production edits. Their failures must be
actual output/tuple mismatches in the existing path, never source matching,
copied parser, missing method, setup, timeout or weakened oracle. Then implement
producer and consumer together; update old builder/hook expectations to the
chosen grammar. Run GREEN once, including affected existing suites. Later causal
fixture/CLI/lint/oracle correction permits an affected-only rerun; no automatic
retry of resource/timeout/transport/unknown/out-of-scope failures or passing replay.

## Bounded invocation contract

One global heavy lease from ROOT is required for every install/test/lint/typecheck/
i18n invocation; none is granted by this order. Commands run serially. Before
each launch record exact argv, absolute cwd, UTC start, absolute cutoff and unique
log/receipt path under `/tmp/kandev-child48-<check>-<attempt>`. Use an owned
supervised process group (for example a temporary Python `Popen` wrapper with
`start_new_session=True` around GNU timeout), record wrapper PID, child PID,
group and returned native handle. Actually join that original handle; verify
wrapper/child/group gone before the next command or lease return. On lost
response reconcile the original state; NO VERDICT until an actual join.

Bounds below include command runtime, with GNU `--kill-after=10s` cleanup.
No unbounded shell waits. Report progress at least every 60s while polling an
active native handle. No package rerun can substitute for joining an earlier run.
Temporary supervision scripts/logs are not additional permanent repo artifacts.

Pinned runtime PATH prefix for every package command:
`/home/jcfs/.local/share/mise/installs/node/24.21.0/bin`.
Pinned pnpm executable:
`/home/jcfs/.local/share/mise/installs/pnpm/9.15.9/pnpm`.
Pinned Go executable:
`/home/jcfs/.local/share/mise/installs/go/1.26.0/bin/go`.
Verify these existing binaries read-only before use; do not install toolchains.
For Node set `NODE_OPTIONS=--max-old-space-size=4096`; Vitest one worker and no
file parallelism. For scoped Go set `GOMAXPROCS=2`, `GOMEMLIMIT=512MiB`.

## Exact later verification payloads

All commands below are FUTURE and require release plus heavy admission. Cwds
are rooted independently against the repository in `plan.md`; actual launch
receipts must expand them to absolute paths. Prefix the pinned Node directory
to existing PATH rather than replace the remainder. Logs and absolute cutoffs
are chosen upfront, never reconstructed after execution.

Conditional once, cwd `apps`, only if worktree dependencies are still absent;
300s, Node 4GiB:

```bash
timeout --signal=TERM --kill-after=10s 300s /home/jcfs/.local/share/mise/installs/pnpm/9.15.9/pnpm install --frozen-lockfile
```

RED Go, cwd `apps/backend`; 180s, Go memory settings above:

```bash
timeout --signal=TERM --kill-after=10s 180s /home/jcfs/.local/share/mise/installs/go/1.26.0/bin/go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=150s ./internal/editors/service -run '^TestOpenEditor_EmbeddedFileTargets$'
```

RED frontend, cwd `apps/web`; 120s, Node 4GiB:

```bash
timeout --signal=TERM --kill-after=10s 120s /home/jcfs/.local/share/mise/installs/pnpm/9.15.9/pnpm exec vitest run --project browser-locales hooks/use-open-session-in-editor.wire.test.tsx -t '^embedded editor target wire ' --maxWorkers=1 --no-file-parallelism
```

GREEN Go, cwd `apps/backend`; 180s, Go memory settings above. This selector
includes both new suites, existing service admission/resolution and external
URL/args controls, and affected builder expectations:

```bash
timeout --signal=TERM --kill-after=10s 180s /home/jcfs/.local/share/mise/installs/go/1.26.0/bin/go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=150s ./internal/editors/service -run '^(TestOpenEditor_|TestResolveSessionPath_|TestResolveFilePath$|TestBuildInternalVscodeURL$|TestBuildHostedURL$|TestBuildRemoteSSHURL$|TestBuildLocalArgs$)'
```

GREEN frontend, cwd `apps/web`; 120s, Node 4GiB:

```bash
timeout --signal=TERM --kill-after=10s 120s /home/jcfs/.local/share/mise/installs/pnpm/9.15.9/pnpm exec vitest run --project browser-locales hooks/use-open-session-in-editor.test.ts hooks/use-open-session-in-editor.wire.test.tsx --maxWorkers=1 --no-file-parallelism
```

Scoped Go lint, cwd `apps/backend`; 180s, `GOMAXPROCS=2`, `GOMEMLIMIT=512MiB`.
Replace `EXACT_BASE_SHA` with the verified task comparison base (PR base after
publication), recording it in the receipt. This is a placeholder, not a ref to run:

```bash
timeout --signal=TERM --kill-after=10s 180s /home/jcfs/.local/bin/golangci-lint run ./internal/editors/service --new-from-rev=EXACT_BASE_SHA --timeout=150s --concurrency=2 --allow-serial-runners
```

Frontend lint, cwd `apps/web`; 120s, Node 4GiB:

```bash
timeout --signal=TERM --kill-after=10s 120s /home/jcfs/.local/share/mise/installs/pnpm/9.15.9/pnpm exec eslint --max-warnings 0 hooks/use-open-session-in-editor.ts hooks/use-open-session-in-editor.test.ts hooks/use-open-session-in-editor.wire.test.tsx
```

Typecheck, cwd `apps/web`; 180s, Node 4GiB. The actual package command includes
its existing pretypecheck generators; retain them:

```bash
timeout --signal=TERM --kill-after=10s 180s /home/jcfs/.local/share/mise/installs/pnpm/9.15.9/pnpm run typecheck
```

I18n, cwd `apps/web`; 120s, Node 4GiB, no new product copy:

```bash
timeout --signal=TERM --kill-after=10s 120s /home/jcfs/.local/share/mise/installs/pnpm/9.15.9/pnpm run i18n:check
```

Affected i18n ratchet, cwd `apps/web`; 120s, Node 4GiB:

```bash
timeout --signal=TERM --kill-after=10s 120s /home/jcfs/.local/share/mise/installs/pnpm/9.15.9/pnpm run i18n:ratchet
```

Only on an actual backend-code PR fixup, one full CHANGED lint, cwd
`apps/backend`: `GOMAXPROCS=2`, `GOMEMLIMIT=1GiB`, GNU 6m, CLI 5m, kill-after
10s, concurrency 2/allow-serial. `EXACT_PR_BASE_SHA` must be the resolved PR base:

```bash
timeout --signal=TERM --kill-after=10s 6m /home/jcfs/.local/bin/golangci-lint run ./... --new-from-rev=EXACT_PR_BASE_SHA --timeout=5m --concurrency=2 --allow-serial-runners
```

Reconcile typecheck-generated changes, never discard foreign files. Add causal
test/lint path changes to the affected selectors before running, not silently
after marking done. Do not broaden to full tests/build/browser checks.

Design/spec/whitespace/reference gates, cwd repository root, 60s each:

```bash
timeout --signal=TERM --kill-after=10s 60s python3 scripts/list-docs.py validate
timeout --signal=TERM --kill-after=10s 60s python3 scripts/lint-spec-files.test.py
timeout --signal=TERM --kill-after=10s 60s python3 scripts/lint-spec-files.py --all
timeout --signal=TERM --kill-after=10s 60s git diff --check -- docs/specs/ui docs/plans/embedded-editor-file-targets
timeout --signal=TERM --kill-after=10s 60s python3 scripts/list-docs.py specs --system ui --format paths
```

Use the actual repository `.github/scripts/pr-docs.cjs:validateCoverage` on
the actual changed paths, including untracked files. In DESIGN expect docs-only
`exempt`, no triggering paths; independently parse/check package REQ/AC/design/
manifest links and source/test/package command references. After implementation
expect `covered` for the real code/test/fixture and package paths, no fake input
or synthetic triggering path. Keep actual changed-path enumeration and module
result in the receipt. Coverage exemption is not a traceability verdict.

## Dependencies and risks

None. Sequential only. Mixed-version private sentinels require refresh; no old
grammar guessing. Native CLI numeric-colon behavior and secondary-worktree
runtime-root correspondence remain explicit residuals. Tests dispatch to socket
wire only and cannot assert native rendering or panel opening with null Dockview.
Existing unavailable paths stay unavailable. Scope changes require a checkpoint.

## Inputs and results

Inputs: owning pair, manifest proof attribution, actual source/router/client/
callers/Remote CLI audit, scoped AGENTS, context-engineering/fix/planner/spec/
plan/mobile-parity workflows. Later use TDD and normal commit/push/PR skills.

Implemented after the reviewed release. Shared real-service Go RED and real
frontend wire RED failed causally before production edits; affected Go GREEN
and both frontend suites (25 tests) passed. Scoped Go/ESLint, typecheck including
generators, i18n check and affected ratchet passed with original handles actually
joined and all owned processes gone. The [manifest](plan.md#implemented-behavior-and-local-conformance)
records receipt names, handles, AC conformance and residuals. The owning
requirement is active and design current. Remaining docs checks, normal hooks,
publication and hosted delivery evidence are recorded in the task checkpoint;
MERGE NONE until a separate ROOT expected-head serial squash grant.
