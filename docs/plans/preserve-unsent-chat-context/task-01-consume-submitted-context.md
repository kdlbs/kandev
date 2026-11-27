---
id: 01-consume-submitted-context
title: Consume only submitted context
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-UI-FILE-TREE-CHAT-CONTEXT-001
acceptance_criteria:
  - AC-UI-FILE-TREE-CHAT-CONTEXT-001.5
  - AC-UI-FILE-TREE-CHAT-CONTEXT-001.6
  - AC-UI-FILE-TREE-CHAT-CONTEXT-001.11
  - AC-UI-FILE-TREE-CHAT-CONTEXT-001.12
  - AC-UI-FILE-TREE-CHAT-CONTEXT-001.13
  - AC-UI-FILE-TREE-CHAT-CONTEXT-001.14
system_design:
  - ../../specs/ui/system-design/file-tree-chat-context.md
---

# Task 01: Consume Only Submitted Context

## Summary and gate

An accepted older send must not silently remove the next message's selected
context. Implement the submitted-selection boundary and its permanent
regressions as one sequential slice, using the paired design. ROOT reviewed the four-artifact package and explicitly released implementation
and the exclusive serial local-heavy lease on 2026-10-07. A separate merge
grant remains required. Do not replay or copy the protected ROOT proof.

## In scope

- Add `consumeSubmittedEphemeral` to `context-files-store.ts`. Clone a newly
  inserted descriptor to give each selection canonical object identity;
  keep no-op duplicate identity stable. Persist only the live survivor list.
- Expose the action through `useContextFiles`; capture the unpinned submitted
  objects/session before await in `submitChatPayload`; pass them to accepted
  `completeChatSubmission`. Keep existing admission branching/results and
  plan-mode re-add intact.
- Write new tests independently using real providers/store/storage/default
  caller and actual FileBrowser action. Update affected existing mocked panel
  fixtures to include the new action and verify it is not called on rejection.
  Do not hide missing wiring with optional chaining or a fallback global clear.
- Audit actual diff, docs traceability, i18n and normal active hooks.

## Out of scope

No passthrough send-lifecycle migration, navigation/remount redesign, same-text
draft replacement, comment/feedback concurrency, new settings, backend, transport
policy, new payload fields, store framework or persistence schema. Preserve
intentional clearing and successful text/upload cleanup. No rendered UI change;
mobile-parity data-only exception and absence of preview/E2E are justified in
the plan. Expand only for a proved requirement and ROOT checkpoint.

## Acceptance

1. AC-001.5 and .11-.14 hold in live state, sessionStorage and hydration with
   submitted/current/new/pinned/replacement mixtures; selection identity survives
   unrelated updates and distinguishes equal-value remove/re-add.
2. AC-001.6 and current admission/payload behavior hold through the real default
   caller and file-selection action with only external transport mocked;
   callback acceptance/rejection remains compatible.
3. The exact serial checks below and active hooks pass; results record actual
   coverage/counts and limitations, with production changes restricted to the
   reviewed boundary and no claim of browser/backend/agent proof.

## TDD cases and evidence

Author `components/task/chat/chat-input-area-context-submission.test.tsx` with
real `StateProvider`, `ToastProvider`, context-files store and sessionStorage.
Mount real `FileBrowser` alongside a submit harness consuming real
`useContextFiles` and `useSubmitHandler`; use default `useMessageHandler`,
`sendMessageRequest`, real queue hook/API and normal session input-mode derivation.
Mock only external HTTP/WS transport (including file-tree replies); do not mock
providers, selector hooks, storage, FileBrowser handlers, message/queue hooks,
formatting or admission functions. Use causally controlled deferred promises,
not sleeps. Seed an ordinary available task session and a valid queue identity
for the busy/clarification case. Actual FileBrowser touch menu action
`file-tree-touch-add-to-chat` must call its production session-bound handler.

| Named case | Boundary and independently checked outcome | AC |
| --- | --- | --- |
| `preserves a file selected through FileBrowser while default direct admission is pending` | Capture actual message.add request; select new file via row action; accept old request; outgoing context contains submitted path only; new chip/selection, storage and rehydrated selection remain | .6, .11 |
| `consumes unchanged submitted context and preserves pinned selections` | Ordinary direct success consumes old ephemeral, retains pinned; metadata/hidden context correct; stored/hydrated survivors match | .5, .6 |
| `preserves current old and new selections after default rejection` | Real default transport returns a definite admission error; old/new/pinned remain; manually removed item stays absent | .13 |
| `preserves a same-path replacement after older acceptance` | Remove/re-add same path during deferred admission, including equal values and same caller descriptor; replacement survives | .12 |
| `retains in-flight pin and unpin edits` | Submitted ephemeral pinned later, and submitted pinned unpinned later, both survive | .14 |
| `preserves later selections after real queued admission` | Busy or clarification routing uses real queue hook/API; deferred external queue admission; submitted metadata unchanged; new selection survives accepted cleanup | .5, .6, .11 |
| `preserves callback admission compatibility` | Exposed onSend true/void consumes snapshot, false/throw preserves current state; payload API unchanged; no mocked internal hook | .5, .13 |

Keep storage assertions in separate cases or first assertions so the live-state
failure cannot prevent causal persisted-survivor evidence. Rehydrate after
clearing only the test's in-memory entries, not its browser storage. Verify
submission stays pending until explicitly settled and no second transport
submission occurs. Clean test-owned providers/store/storage after each case.

Extend `lib/state/context-files-store.test.ts` for the new action: new path,
same-path replacement with exact reused descriptor object, duplicate no-op,
unpinned/pinned mixtures, pin/unpin changes, file/directory/legacy shapes,
`prompt:` and `plan:context` compatibility, latest metadata, empty/idempotent
snapshot and other-session isolation. Retain existing unconditional
`clearEphemeral` and explicit remove/session-clear semantics; test no resurrection.
Do not invent a new persisted identity. Existing
`use-chat-input-state-accepted-payload.test.ts` protects successful text/upload
clearing and rejected payload retention. Include it and existing routing,
transform, handler and affected fixture suites in GREEN.

## Execution and resource protocol

Read `/tdd`, scoped web AGENTS, requirement/design and this plan again at later
authorization. Mark this work order `in_progress`. Verify exact checkout/source
identity without rebasing or proof replay. ROOT must grant the local-heavy lease
before install, RED, GREEN, lint, typecheck, i18n or hooks. Run one operation at
a time; record original UTC/argv/cwd/log, native start+terminal chunks, PID/PGID,
cutoff, exit and actual joined/gone evidence. Retain/poll every returned handle
to completion; no duplicate invocation or overlap. Checkpoint setup/resource,
timeout/transport/unknown/out-of-scope failures to ROOT, without auto retries,
cache deletion or test weakening.

If workspace dependencies are absent, allow exactly one conditional pinned
installation from `apps/`; preserve existing dependencies and managed worktree:

```bash
(cd apps && timeout --kill-after=10s 600s env NODE_OPTIONS=--max-old-space-size=2048 corepack pnpm@9.15.9 install --frozen-lockfile)
```

Inspect dependency presence first; do not run installation as a routine gate.
Record pnpm 9.15.9 and Node 24 environment before package checks. Do not run any
of these commands in the design turn.

## Exact verification commands

Run RED once after independently authoring permanent regressions and before
production changes. Require expected live/persistence causal failures, not a
setup/import failure; report the classification and actually join/gone before GREEN.

```bash
(cd apps/web && timeout --kill-after=10s 120s env NODE_OPTIONS=--max-old-space-size=2048 pnpm exec vitest run --maxWorkers=1 --no-file-parallelism --testTimeout=30000 lib/state/context-files-store.test.ts components/task/chat/chat-input-area-context-submission.test.tsx)
```

After implementing the minimum correction, run GREEN once, serially:

```bash
(cd apps/web && timeout --kill-after=10s 300s env NODE_OPTIONS=--max-old-space-size=2048 pnpm exec vitest run --maxWorkers=1 --no-file-parallelism --testTimeout=30000 lib/state/context-files-store.test.ts components/task/chat/chat-input-area-context-submission.test.tsx components/task/chat/chat-input-area.test.tsx components/task/chat/chat-input-area.test.ts components/task/chat/chat-input-area-transform-outgoing.test.tsx components/task/chat/chat-input-area-preview-feedback.test.ts components/task/chat/chat-submit-plugin-decoration.test.tsx components/task/chat/use-chat-input-state-accepted-payload.test.ts hooks/use-message-handler.test.ts)
(cd apps/web && timeout --kill-after=10s 300s env NODE_OPTIONS=--max-old-space-size=2048 pnpm run typecheck)
(cd apps/web && timeout --kill-after=10s 180s env NODE_OPTIONS=--max-old-space-size=2048 pnpm exec eslint --max-warnings 0 lib/state/context-files-store.ts lib/state/context-files-store.test.ts components/task/chat/use-chat-panel-state.ts components/task/chat/chat-input-area.tsx components/task/chat/chat-input-area-context-submission.test.tsx components/task/chat/chat-input-area.test.tsx components/task/chat/chat-input-area-transform-outgoing.test.tsx)
(cd apps/web && timeout --kill-after=10s 180s env NODE_OPTIONS=--max-old-space-size=2048 pnpm run i18n:check)
(cd apps/web && timeout --kill-after=10s 180s env NODE_OPTIONS=--max-old-space-size=2048 pnpm run i18n:ratchet)
```

Append any actually changed existing fixture/test files to changed-file ESLint,
and ensure each changed test is in GREEN's file list. Check fixture references
with `rg -n 'clearEphemeral|consumeSubmittedEphemeral' apps/web/components/task/chat`.
Update the recorded exact commands before execution if the final owned fixture
set differs; do not silently omit a changed suite. No additional suite replay
after passing unless a real change/failure justifies it. No broad suite, browser,
build or E2E absent a proved need and ROOT lease. No backend checks for this
frontend-only repair; a real backend-code finding requires ROOT checkpoint and
its full CHANGED exact-PR-base serial resource protocol.

Lightweight documentation checks from repository root (also allowed in design):

```bash
timeout --kill-after=10s 60s python3 scripts/list-docs.py validate
timeout --kill-after=10s 60s python3 scripts/lint-spec-files.test.py
timeout --kill-after=10s 60s python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/preserve-unsent-chat-context
git status --short -- docs/plans/preserve-unsent-chat-context
```

Run the repository's pure coverage preflight from root with the four documents
and actual changed paths. The fixed comparison base is the reviewed task base;
if the actual PR base differs, checkpoint ROOT and record its exact SHA before
using it. Do not invoke the network/status publication runner.

```bash
timeout --kill-after=10s 60s node <<'JS'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const docs = [
  'docs/specs/ui/requirements/file-tree-chat-context.md',
  'docs/specs/ui/system-design/file-tree-chat-context.md',
  'docs/plans/preserve-unsent-chat-context/plan.md',
  'docs/plans/preserve-unsent-chat-context/task-01-consume-submitted-context.md',
];
const rawPaths = [
  execFileSync('git', ['diff', '--name-only', '1f40b1a4ef72251c7ec1e8d8c9c1dedfeaa70dd9'], { encoding: 'utf8' }),
  execFileSync('git', ['ls-files', '--others', '--exclude-standard'], { encoding: 'utf8' }),
].join('\n').split('\n').filter(Boolean);
const changedFiles = [...new Set(rawPaths)].map(filename => ({ filename, status: 'modified' }));
const fileContents = Object.fromEntries(docs.map(path => [path, fs.readFileSync(path, 'utf8')]));
const result = validateCoverage({ changedFiles, fileContents });
process.stdout.write(JSON.stringify(result, null, 2) + '\n');
if (!result.ok || result.status === 'exempt') process.exitCode = 1;
JS
```

The design attempt included a clearly labelled prospective production path to
exercise non-exempt coverage, but could not start without Node. Implementation
uses actual paths in the command above and must record its actual result.
Audit `docs/public`, README and screenshots; no public change currently required.
No new copy is planned; if implementation introduces copy, translate all seven
languages under existing i18n rules and checkpoint scope change.

After checks pass, synchronize work order `done` and plan `implemented` with
actual commands/results. The existing owning specs retain active/current status.
Normal commit/push/ready PR is already authorized for the later implementation;
load those skills and run all configured hooks without bypass under ROOT lease.
Inspect hook-generated changes, restage and create a fresh commit when required.
Keep actual hook logs/results; do not schedule an extra duplicate generic hook
or product-suite replay. Follow the manifest's current-head review/CI and
separate merge-grant gates; task is incomplete until independently verified merge.

## Files likely touched

Production: `apps/web/lib/state/context-files-store.ts`,
`apps/web/components/task/chat/use-chat-panel-state.ts`,
`apps/web/components/task/chat/chat-input-area.tsx`.
Tests: `apps/web/lib/state/context-files-store.test.ts`, new
`apps/web/components/task/chat/chat-input-area-context-submission.test.tsx`,
and existing chat fixture suites actually referencing the replaced cleanup action.
Evidence-only consumers: FileBrowser and parts, FileContextMenu,
`apps/web/components/task/task-chat-panel.tsx`, `apps/web/hooks/use-message-handler.ts`,
`message-request.ts`, real queue hooks/API, and passthrough composer. No production
edits to those consumers are planned.

## Dependencies, parallelism and risks

Dependencies: none. Parallelism: `sequential`; no delegates, new tasks/sessions,
tabs or model switch. Identity/cloning and captured-context consistency are the
main risks. Separate passthrough and comment cleanup remain bounded residuals;
no browser/backend/agent execution is claimed. Preserve user edits and system
marker/IDs in the external task plan; ROOT watches primary messages directly.

## Results

Implementation is in progress under ROOT lease62. Design checks on 2026-10-07:

- Catalog validate exit 0, chunk a38bc1: 357 decisions and 1413 specifications.
- Spec-linter tests exit 0, chunk 1a7f3a: 36 tests.
- All-spec lint exit 0, chunk dc7bda: all specification files passed.
- Diff/status exit 0, chunk 540baa: two modified specs plus the two-file untracked
  plan directory; no staged files. All four artifacts fit their applicable limits.
- Independent static traceability/whitespace exit 0, chunk 8dc46a: requirement,
  acceptance, design, manifest and one-work-order references valid.
- Pure repository JS coverage preflight did not start, chunk 15c05a: `node` absent
  from PATH, exit 127. Checkpointed to ROOT; no retry/install. Still required under
  the authorized Node 24 environment before delivery; static checks do not replace it.

Every design command returned terminal output with no running native handle.
At DESIGN END there were no permanent tests, production changes, install, product
checks, hooks, commit, push or PR. Protected archive verified
read-only 0400 with its original SHA256; no contents replayed or copied.


Implementation evidence on 2026-10-07 (original logs and joined/gone receipts
retained as `/tmp/kandev-child62-<label>.{json,log}`):

- One conditional frozen pnpm 9.15.9 install succeeded, using existing Node 24.21.0.
- First RED (`red`) contained one causal store identity failure and twelve
  fixture-invalid component failures (connection subscription fixture).
- Corrected RED (`red-corrected`) had five causal failures and five fixture
  failures (four desktop viewport, one initial queue refresh readiness).
- ROOT authorized the five affected executions only (`red-fixture-final`): four
  causal failures established real FileBrowser/default direct live, storage and
  hydration loss plus real queue storage loss; rejected admission control passed.
  Native16227, chunks5d2573 to ba0d68, exit1, actually joined/group gone.
- Minimum repair changes only the three reviewed production files. Nine affected
  suites (`green`) passed 110/110, native4042/d49895 to1ea5e5, exit0, joined/gone.
- Original typecheck (`typecheck`) failed134 at the configured 2 GiB heap limit,
  native44644/919515 to be6754, joined/gone. No type-check verdict; preserved
  resource failure, not a source failure. ROOT authorized exactly one recovery
  with 4 GiB on the same Node/pnpm runtime and cutoff:

```bash
(cd apps/web && timeout --kill-after=10s 300s env NODE_OPTIONS=--max-old-space-size=4096 pnpm run typecheck)
```

This explicit resource exception does not increase other caps, reinstall,
change configuration, clear caches, or replay passing product tests.

The authorized 4 GiB recovery reached three concrete type diagnostics in the new
fixture (explicit partial panel assertion and branded session/task IDs), exit2,
native 51394/a50e5d to e58c8a, actually joined/gone. Corrected only fixture type
declarations; rerun its component suite and affected typecheck under the same
4096MiB cap, as authorized for concrete diagnostics.

- Corrected fixture suite passed 12/12 (`green-fixture-types`); typecheck passed
  (`typecheck-fixture-types`) at the authorized 4096MiB cap.
- Changed-file ESLint first reported five warnings (`lint`): fixture nested
  ternary/repeated literal/group length, store test group length and chat module
  one line over its limit. Split test groups, named the transport action, adjusted
  request URL conversion and combined duplicate type imports; no assertion or
  behavior changes. ESLint then passed (`lint-fix`).
- Only the two edited test suites reran (`green-lint-fix`), 21/21 passed. Final
  typecheck is rerun for the changed declarations/imports, using the same
  authorized command; the unchanged 110-test suite was not replayed.

```bash
(cd apps/web && timeout --kill-after=10s 120s env NODE_OPTIONS=--max-old-space-size=2048 pnpm exec vitest run --maxWorkers=1 --no-file-parallelism --testTimeout=30000 components/task/chat/chat-input-area-context-submission.test.tsx)
(cd apps/web && timeout --kill-after=10s 120s env NODE_OPTIONS=--max-old-space-size=2048 pnpm exec vitest run --maxWorkers=1 --no-file-parallelism --testTimeout=30000 lib/state/context-files-store.test.ts components/task/chat/chat-input-area-context-submission.test.tsx)
```

Final typecheck (`typecheck-final`, native13436/ad09c5 to811c81) passed at
4096MiB, actually joined/gone. Full i18n check passed (`i18n-check`), including
9246 referenced keys and 3753 guarded files; no new copy/catalog changes.

I18n ratchet passed (`i18n-ratchet`, native64033/e06d90 to f276ff); no new
copy/catalog changes. Catalog validation passed357 decisions/1413 specifications
(`docs-catalog`); all-spec lint passed (`docs-spec-lint`). Actual changed-file
coverage (`docs-coverage`) passed with `status: covered`, `requiresCoverage: true`,
`errors: []`, all three production paths triggering this single work order.
Every operation above actually joined with its process group gone. Normal
active hook/commit, published-head and hosted review/CI receipts will be retained
in the external task plan; no hook bypass or merge authorization is implied.
