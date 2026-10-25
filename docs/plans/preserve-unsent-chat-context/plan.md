---
created: 2026-10-07
status: implemented
requirements:
  - REQ-UI-FILE-TREE-CHAT-CONTEXT-001
system_design:
  - ../../specs/ui/system-design/file-tree-chat-context.md
legacy_specs: []
---

# Implementation Plan: Preserve Unsent Chat Context

## Overview

An older accepted submission currently clears a file reference selected for the
next message while admission was pending. The next message silently loses the
user's chosen context, and the current cleanup also writes the reduced selection
to browser storage. Repair only the shared task-chat submitted-selection
consumption boundary in one sequential work order. ROOT reviewed and released the package for implementation on 2026-10-07;
every local-heavy operation uses its exclusive serial lease. Publication and
current-head CI/review gates precede a separate merge grant.

## Ownership and settled assumptions

Reuse [the existing requirement](../../specs/ui/requirements/file-tree-chat-context.md)
and [design](../../specs/ui/system-design/file-tree-chat-context.md). The former
already owns file-tree context retention in AC-001.5 and payload inclusion in
AC-001.6. Clarify .5 and add .11-.14 rather than duplicate the capability under
Tasks. Tasks continues to own durable message/queue admission. The prior
[completed package](../file-tree-chat-context/plan.md) and its three done work
orders remain historical evidence; their existing responsive/E2E results are not
rerun or represented as proof of this race correction.

Confirmed intent: later unsent additions and same-path replacements survive;
unchanged submitted ephemeral entries are consumed; current pinned and rejected
selections persist. No material product question remains. No meaningful new
architecture alternative, incident specification, or ADR is needed for this
local immutable-selection comparison.

## Source and evidence audit

Design HEAD and remote main read at entry: `1f40b1a4ef72251c7ec1e8d8c9c1dedfeaa70dd9`.
At the final remote read, main had advanced to
`a5f6f7722ee0ebf6ae966f96a7d9c8a17ce6f8fc`; this package remains grounded in
the unchanged design HEAD. No fetch, rebase, or source mutation was performed.
Proof baseline: `62b39941214ffe63ce72d307b6e599a7bd2a7b63`. On 2026-10-07 all seven
source SHA256s in the supplied reachability receipt matched the working files,
proof-baseline blobs, and HEAD blobs exactly:

- `apps/web/components/task/file-browser.tsx`
- `apps/web/components/task/file-browser-parts.tsx`
- `apps/web/components/task/file-context-menu.tsx`
- `apps/web/components/task/chat/use-chat-panel-state.ts`
- `apps/web/components/task/chat/chat-input-area.tsx`
- `apps/web/hooks/use-message-handler.ts`
- `apps/web/lib/state/context-files-store.ts`

The actual source places `clearEphemeral` after awaited callback/default admission
in `submitChatPayload`/`completeChatSubmission`. It filters every current
unpinned entry, not only the selection submitted by that call. The real
FileBrowser touch and desktop actions can add context during the await.

Accepted read-only evidence:
`/tmp/kandev-root-chat-context-send-proof-receipt.json`,
`/tmp/kandev-root-chat-context-send-proof-classification.json`,
`/tmp/kandev-root-chat-context-send-proof-test.log`, and
`/tmp/kandev-root-chat-context-send-source-reachability-20261007.json`.
The original native session 44666, start chunk e0f266, terminal chunk f8ac96,
ran 2026-10-07T01:04:20.605529Z to 01:04:32.909210Z and actually joined exit 1;
PID/PGID 4138700 and wrapper 4138668 were gone and owned scratch removed.
One causal new-reference-loss assertion failed; ordinary accepted and rejected
admission controls passed. No setup, timeout, resource, or race failure was reported.

This is production `useSubmitHandler` with real StateProvider/ToastProvider,
Zustand context store and storage, mocking only exposed `onSend` admission.
The failing live-store assertion prevented its later causal storage assertion:
persistence loss is source-supported, not an executed failing assertion.
FileBrowser/default transport reachability is static evidence, not executed
browser/default-WebSocket/backend-agent proof. Never replay, copy/import, modify,
or remove the protected 0400 archive
`/tmp/kandev-root-chat-context-send-next-candidate.test.tsx`, SHA256
`30a1565e07bb352f2fa363c300c0366440ef9bc8ab4278f37521e2f6653c0ef6`.

## Scope and technical approach

Capture the submitted unpinned canonical entry objects and session before
admission. Add `consumeSubmittedEphemeral` in the context store and expose it
through `useContextFiles`. Accepted shared cleanup consumes only identical
current unpinned objects. Clone incoming descriptors on new insertion so a
remove/re-add is a new selection even if the caller reuses its object. Retain
identity on duplicate no-ops; preserve later pin/unpin replacements. Persist the
survivors with existing serialization and plan-mode re-add behavior.

| Caller / boundary | Shape and behavior | Planned evidence / residual |
| --- | --- | --- |
| Shared task chat, default direct WS | Existing ContextFile + message.add payload; snapshot consumption after acceptance | Real providers/hooks, deferred external request |
| Shared task chat, queue/clarification | Existing queue identity and context metadata; same completion | Real queue hook/API with external transport only |
| Exposed onSend callback | Existing ChatSubmitPayload/ChatSubmitResult unchanged | Deferred true/void, false and throw controls; no invented context composition |
| Files/directories, legacy entries | Optional directory and pin fields remain compatible | Store persistence/hydration and actual file-selection action |
| Prompt and plan sentinels / inline mentions | Existing formatting/filtering; stored sentinels use selection identity; inline mentions remain payload-owned | Focused sentinel/metadata controls |
| Passthrough composer | Existing unconditional clearEphemeral remains | Audited residual; no passthrough race protection claim |

Excluded: generic navigation/remount lifetimes, same-text draft replacement,
comment/feedback concurrency, framework/store redesign, settings, backend and
global agent-delivery policy. Do not alter text/upload success behavior or
intentional clear/remove/session actions. No transport API or storage migration.

## Tests and bounded default-path integration

The [single work order](task-01-consume-submitted-context.md) maps AC-001.5,
.6, and .11-.14 to permanent store and real-provider component tests. It names
exact accepted/rejected/current/pinned/new-addition/replacement controls and
requires independent live/storage/hydration assertions. The causal component
case must use a real file-selection action, not only `store.addFile`.

## E2E and mobile decision

The narrow integration boundary mounts real providers, FileBrowser action,
context hook, submit hook, default message handler, queue hook, and storage;
only external transport is mocked. It proves client composition and accepted
cleanup without a browser/build/backend/agent run. No broad suites, browser,
build, or E2E is justified by this data-only change. The mobile-parity exception
applies because geometry, touch behavior, scrolling, navigation and breakpoint
branches do not change. Existing touch action is exercised in the component
regression; no new ASCII preview or mobile Playwright run is needed. Any finding
that requires those broader boundaries checkpoints ROOT before expanding scope.

## Public guide audit

Audited `docs/public/**`, root README and screenshot catalog for file/chat
context retention and reset wording. The public WebSocket guide owns queue
context metadata; the mobile guide mentions deliberate context reset. Neither
contract changes. `use-kandev.md` has no pending-selection lifetime instructions
to correct. Internal specs are updated; no public guide or screenshot change is
needed for this bounded concurrency correction. Re-audit the actual implementation
diff before delivery, without adding speculative user instructions.

## Work orders

- [ ] [Task 01: Consume only submitted context](task-01-consume-submitted-context.md)

One work order, wave 1, no dependencies or delegates. Same primary session,
profile and executor throughout. Production/permanent tests await later ROOT
implementation authorization; installs and checks await the heavy lease.

## Verification results

Design-only lightweight validation passed: catalog (357 decisions, 1413 specs),
36 spec-linter tests, all-spec lint, diff/whitespace and four-file static
REQ/AC/design/manifest traceability. The repository's pure JS documentation
coverage preflight could not start: `node` is absent from PATH (exit 127).
This setup limitation was checkpointed to ROOT in the primary conversation and
external task plan; no retry or install. Static traceability is separate evidence,
not a claim that the JS preflight passed. Run that preflight under the later
authorized Node 24 environment before delivery.

Product tests, typecheck, changed-source lint, i18n and hooks remain deferred to
authorized implementation. No product passing result is inferred from previous
work. Record every exact command and actual result in the work order before
marking it done; no automatic broad verification. Design END: four docs only,
unstaged/uncommitted, zero running handles, protected archive unchanged.

## Delivery and operational constraints

Retain original UTC/argv/cwd/log, PID/PGID, cutoffs and start/terminal tool chunks
for every resource operation. Run serially, keep every handle, actually join it
and establish its process group gone before the next operation. Setup/resource,
timeout/transport/unknown or out-of-scope failure checkpoints ROOT; no automatic
retry, cache wiping, duplicate invocation, overlap, or foreign cleanup.

Later implementation has standing authorization for normal commit/push/ready PR
after task checks and ROOT lease, with active hooks and no bypass. Keep the
published SHA fixed except for a real finding; no moving-main rebase, synthetic
merged tests, weakened checks, or optional polish. Follow ROOT's configured
CodeRabbitApp347564 full current-head/all-changed-file semantic review gate;
ACK is not semantic evidence and request only a proven coverage gap once.
Retain one original 90m all-terminal `scripts/pr-await` under GNU 91m/kill10,
joined/gone before any ROOT-authorized successor. Require actual six required
checks and Backend/Frontend/E2E parents SUCCESS, fresh complete errors[] and
zero actionable visible/hidden/human findings. End at MERGE READY, then await a
separate ROOT serial MERGE grant. Task completion requires actual expected-head
squash merge and independent SHA/tree/blob/remote verification. ROOT owns proof
archive release/refill; preserve managed worktree/dependencies/foreign resources.

## Risks and bounded residuals

- Identity must represent a selection, not path/value equality. Cloning new
  inserts and preserving duplicate no-ops are necessary for same-object re-add.
- Capture from the same panel snapshot the default handler composes. Capturing
  newer store state would consume unsent selections; reading only paths would
  consume replacements.
- The separate passthrough completion path, other context-item cleanup, and
  generic navigation/remount races remain outside this repair. State/storage
  assertions here do not prove backend or real browser delivery.


## Implementation result

The three-file production repair captures the same submitted panel context the
default handler composes, consumes only unchanged unpinned selection objects,
and persists live survivors. New insertions own cloned descriptors, so an
equal-value replacement survives while duplicate no-op identity remains stable.

Independent RED established FileBrowser/default direct loss separately in live
state, storage and hydration, and loss after real queued admission; rejected
admission passed. Earlier fixture-invalid attempts remain classified in the
[work order](task-01-consume-submitted-context.md). Nine affected suites passed
110/110, with only changed fixtures rerun afterward (12/12 then 21/21). Final
typecheck and changed-file lint passed. The original 2 GiB typecheck OOM remains a
failed resource attempt; ROOT authorized the 4096MiB diagnostic recovery.

I18n check/ratchet, documentation catalog/spec lint and actual-path coverage
passed (`covered`, `requiresCoverage: true`, `errors: []`). Normal active hooks
and publication receipts are recorded in the external task plan after delivery. This implementation does not claim real browser, backend-agent
or passthrough-composer race coverage. Hosted CI/review and separate merge grant
remain pending; task completion still requires verified actual merge.
