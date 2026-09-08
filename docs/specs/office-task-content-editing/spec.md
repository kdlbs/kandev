---
status: specified
created: 2026-08-19
owner: kandev
---

# Office Task Content Editing

## Why

The Office task detail page (`/office/tasks/[id]`, simple pane) renders the task **title** and
**description** as static text. Every other property on the page (status, priority, assignee,
project, parent, blockers, reviewers, approvers, labels) is editable inline. The two fields that
carry the most meaning are the two a user cannot change.

The description is not decoration: it is the brief the assigned agent works from. A typo in the
title, a missing constraint in the brief, or a scope note that arrived after the task was filed
currently has exactly one remedy: leave the Office UI, open `/t/<id>`, and edit from the Kanban
surface. Users read the absence as a bug. It is not: `docs/specs/office/tasks.md:58` enumerates the
editable properties and omits both fields, and the shipped code implements that list faithfully.
This is a spec gap, and this document closes it.

Nothing in the backend blocks it. `PATCH /api/v1/tasks/:id` has accepted `title` and `description`
since before Office existed, both surfaces read the same `tasks` row, and the resulting
`task.updated` event already drives the Office detail page's live refresh.

## What

### A. Scope of the affordance

- The title and description of a task are editable **in place** on the Office task detail page's
  simple pane. No dialog, no navigation, no separate edit mode for the page as a whole.
- The affordance is available on every task the detail page can render, in every task status
  (including `done` and `cancelled`) and in every session state (including `RUNNING`). No new
  permission, ownership, or lifecycle gate is introduced. This matches the sidebar property
  pickers, which are likewise ungated.
- **Archived tasks are not a special case here.** The Kanban title editor
  (`apps/web/components/task/task-top-bar-title.tsx`) gates renaming on an `isArchived` prop, and
  parts of that component are reused for the title editor. **That gate is deliberately NOT ported.**
  The Office detail page does not render an archived state, does not receive an equivalent flag, and
  `Service.UpdateTask` enforces no archived precondition of its own, so porting the gate would
  introduce a condition that can never be true and would read as dead code. AC-30 and AC-47 govern.
- Only the simple pane is in scope. The advanced / dockview mode is unchanged.

### B. Title editing (inline, Enter to commit)

- The title renders as a text element that the user activates by **double-click**, or by focusing
  it and pressing **Enter**. Activation replaces it with a single-line text input seeded with the
  current title, focused, with its contents selected.
- **Enter** commits. **Escape** cancels and discards the draft. **Blur** cancels and discards the
  draft.
- **The title baseline.** When the title editor opens it captures the **displayed** title at that
  moment as its **frozen baseline**, frozen for the lifetime of the editor. Wherever this document
  says "the stored title" while a title editor is open (AC-4's restore target, AC-8's equality
  test) it means that frozen baseline, never a canonical value that arrived afterwards. This is
  the same construct section C defines for the description and it exists for the same reason:
  while the editor is open the displayed title is guarded (section E), so a remote edit must not
  silently change what Escape restores or what counts as "unchanged". The canonical value that
  arrived meanwhile is not lost. It is applied when the editor closes, under the deferred-apply
  rule in section E. AC-51 governs.
- Commit is **optimistic**: the new title renders immediately, the request is issued, and on
  failure the previous title is restored and the error surfaces as a toast. Which value "the
  previous title" means when commits overlap is defined in section F under "Rollback ordering".
- The value that is **sent** and the value that is **optimistically rendered** are both the
  whitespace-trimmed draft, so the visible title always equals the value the server will persist.
  The server trims too; the client trims first so the two never disagree, even briefly.
- The input is hard-clamped to **60 characters** as the user types, counted the same way the
  backend counts (Unicode code points, `TaskTitleMaxLength = 60` in
  `apps/backend/internal/task/service/task_title.go`). The caret must not jump to the end of the
  field when a keystroke is clamped away.
- **Paste is clamped identically to typing**: the pasted text is inserted at the caret, the result
  is clamped to the first 60 code points, and the caret stays at the end of the inserted run (or at
  the clamp boundary when the insertion is cut short). Paste is not a separate code path with
  separate truncation rules.
- A commit whose trimmed draft is **empty** is refused client-side: the editor stays open, the
  field is marked invalid, and no request is issued. The draft is never silently discarded.
- A commit whose trimmed draft **equals the current title** issues no request and closes the editor.
- IME composition must not commit. An `Enter` that the IME is using to accept a candidate is
  consumed by the IME, not by the editor.

### C. Description editing (explicit Save and Cancel)

- The description renders as static text that the user activates by **clicking** it. Activation
  replaces it with a multi-line textarea seeded with the current description, focused, caret at the
  end.
- When the task has **no description**, the page renders a persistent placeholder affordance in the
  description's position that activates the same editor. An empty description must never leave the
  user with nothing to click.
- **"No description" is defined as: the stored description, after trimming, is the empty string.**
  This single definition covers `null`, `""`, and a field omitted from the DTO (the Office task DTO
  serialises `description` with `omitempty`, so all three reach the client as the same absence).
  There is no fourth state and no visible difference between them.
- **The editor baseline.** When the editor opens it captures the current stored description as its
  **baseline**, and that baseline is **frozen for the lifetime of the editor**. Every
  draft-versus-stored comparison in this document (Save enabled/disabled, Cancel confirm/no-confirm,
  Escape inert/closing) compares the trimmed draft against the **frozen baseline**, never against a
  value that may have changed underneath. A successful save replaces the baseline with the value
  that was saved. Nothing else replaces it. The rationale is in section F under "Late-arriving
  refresh": without a frozen baseline, a remote edit by another client silently flips Save,
  Cancel, and Escape behaviour while the user is typing, with no action of their own.
- Comparisons use the **trimmed** draft, and the value sent is the trimmed draft. A whitespace-only
  draft is therefore equal to an empty one, so a whitespace-only edit on an empty description leaves
  Save disabled rather than issuing a save that the server would silently reduce to a no-op.
- The editor commits only on an explicit **Save**. **Cancel** discards. **Blur does not commit and
  does not close the editor**: the draft survives clicking elsewhere on the page.
- **Escape** discards the draft and closes the editor when the draft is unchanged from the baseline.
  When the draft *is* changed, Escape does nothing; the user must choose Save or Cancel.
- **Cancel with a changed draft** requires a confirmation before the draft is discarded. Cancel with
  an unchanged draft closes immediately with no prompt.
- Save is disabled while the trimmed draft equals the baseline, and while a save is in flight.
- Save is **not** optimistic: the editor stays open and shows a saving state until the request
  resolves. The non-optimism is end to end, not just on this page: the description's Office task
  store patch, which drives card previews in list and board views, is also applied only on success
  (section E, AC-56). A card preview that updated before the save resolved would contradict this
  bullet on a different surface. On failure the editor stays open with the draft intact and the
  error surfaces as a toast, so a rejected save never destroys typing.
- **While a save is in flight** the textarea **stays editable**; Save and Cancel are both disabled
  and Escape does nothing. Disabling the textarea instead would silently swallow keystrokes the user
  has already begun typing.
- **On a successful save**, if the current trimmed draft still equals the text that was submitted,
  the editor closes and the saved value renders. If the user **edited the draft while the save was
  in flight**, the editor **stays open with the newer draft intact** and the baseline becomes the
  value that was just saved, so Save re-enables for the newer text. A successful save must never
  discard typing that the user did after pressing it. This is the same principle as the failure
  path, applied to the success path.
- The description is **plain text**, rendered with preserved whitespace. This feature introduces no
  Markdown rendering and no rich-text editing.
- There is no length limit on the description, client-side or server-side.

**Why the two fields differ.** The title is one line, bounded at 60 characters, and cheap to retype
if a commit is lost; optimistic-with-discard matches the pickers and the existing Kanban rename. The
description is unbounded prose that is also the agent's brief: Kandev stores no description version
history, so a save that fires by accident, or a draft dropped on blur, is unrecoverable. The
industry split follows exactly that line. Linear and Notion autosave and have undo; Jira and GitHub
use explicit buttons and do not. Kandev is in the second group. `project-header.tsx` already gates a
Save affordance on a dirty long-form Office textarea, which is the precedent for requiring an
explicit commit; it is **not** a precedent for this feature's control layout, which section H
specifies directly.

### D. What an edit does NOT do

- A title or description edit **does not wake, interrupt, or re-prompt the assigned agent**, and
  does not queue a run. Verified: `Service.UpdateTask` publishes `task.updated`
  (`service_tasks.go:1607`); the only Office subscriber for that event is `handleTaskUpdated`
  (`internal/office/service/event_subscribers.go:749`), which reads
  `assignee_agent_profile_id` off the payload; `publishTaskEventNow`
  (`service_events.go:370-436`) never puts that field on a `task.updated` payload. With an empty
  profile id and `fallbackToStoredRunner = false`, `queueTaskAssignedRun` returns without queuing.
  Note that `TaskUpdatedData` **declares** that field even though this publisher never sets it, so
  the invariant is one added line away from breaking. AC-31 is therefore a tested contract, not a
  standing code-reading.
- Consequently, a running agent continues on the brief it was launched with. An edited description
  is picked up at the agent's **next wakeup**, produced by the existing triggers (a comment, an
  assignment change, a status change), not by the edit.
- Because this surprises users, the description editor shows a **non-blocking inline note** while at
  least one session on the task is in state `RUNNING`, stating that the change applies at the
  agent's next wakeup. The note never blocks Save.
- An edit does not change task status, does not move the task between workflow steps, and records no
  approval decision.

### E. Persistence and live update

- Both fields are written through the existing `PATCH /api/v1/tasks/:id` endpoint (frontend:
  `updateTask()` in `apps/web/lib/api/domains/kanban-api.ts`). The Office endpoint
  `PATCH /api/v1/office/tasks/:id` is **not** extended, and its documented field list in
  `docs/specs/office/tasks.md:229` remains correct.
- The request carries **only the field being edited**. A title commit sends `{ title }`; a
  description save sends `{ description }`.
- The server **trims leading and trailing whitespace** from both fields before persisting
  (`task_http_handlers.go`, `httpUpdateTask`). The client trims before sending as well (sections B
  and C), so the rendered value equals the stored value at every moment, not merely after a
  round-trip.
- After a successful write the backend publishes `task.updated`, which the Office WS broadcaster
  re-emits as `office.task.updated`, which triggers `triggerRefetch("task:<id>")` and re-fetches the
  canonical task DTO. Other open clients converge without polling. No new event, subject, or
  subscriber is introduced.
- **Canonical value versus displayed value.** These are two different things and the distinction
  carries the whole of this section. The **canonical value** of a field is the value this client
  last learned the server holds. The **displayed value** is what renders on the page, what seeds an
  editor, and what the Office task store entry carries for list and board cards. They are usually
  equal. They diverge exactly while a local edit is in flight or an editor is open, and the rules
  below say how they re-converge. Nothing in this feature ever discards a canonical value in order
  to protect a draft; it defers applying it.
- **What records a canonical value: the complete list.** Exactly three events record a canonical
  value for a field, and nothing else does.
  1. **The initial task load** for the page records both fields (AC-59).
  2. **A refetch resolving**, unless the response is discarded by one of the two ordering rules
     below (AC-49).
  3. **A successful write resolving**, which records the value that request persisted for the field
     it wrote, read from the response body. `updateTask()` already returns the updated task
     (`apps/web/lib/api/domains/kanban-api.ts`), so this costs no extra request. The recording
     happens **before** the write releases that field's guard (AC-58).
  **Every recording of `description` is normalised to the empty string first.** `undefined`, `null`,
  and a field omitted from the payload all record as `""`. This is not a tidiness rule and it is not
  optional: the two sources genuinely disagree in their types. The Office task DTO declares
  `description` **optional** (`OfficeTask` in `apps/web/lib/state/slices/office/types.ts`, and
  the Go side serialises it with `omitempty`), so a load or refetch recording can carry
  `undefined`; a write response carries `Task.description`, which is a **required** string
  (`apps/web/lib/types/http.ts`), so it records `""`. Without normalisation those two are unequal,
  AC-50 applies a canonical value "if the two differ", and every guard release on a task with no
  description would fire a spurious apply forever. Normalising at the recording site makes the
  canonical value, the displayed value and the store entry use the one definition section C already
  gives for "no description", so all three agree. **`title` needs no equivalent rule** and must not
  be given one by symmetry: both DTOs declare it a required string. AC-71 governs.
  Recording is otherwise **unconditional**: it is never suppressed by an open editor or an in-flight
  write. Only the **application** of a canonical value to the displayed value is guarded (AC-49).
  The third trigger is not bookkeeping. Without it the client's own successful edit is never
  recorded, so the deferred apply defined below would land the pre-write value the instant the guard
  released and visibly revert a save that had just succeeded.
- **Stale responses are discarded, not recorded.** Each fetch records, **per field**, the client's
  write generation for that field at the moment the request is issued. A response is **stale for a
  field** if this client committed a write to that field after the request was issued, and a stale
  response is discarded **for that field only**: it is neither recorded as canonical nor applied.
  Other fields of the same response are unaffected. This is what stops a refetch begun before a
  save from resolving afterwards and reinstating the pre-save text (AC-52). Staleness is judged by
  request-issue order, not by comparing values, so a response that happens to carry a third party's
  newer value but was requested before this client's write is discarded too. That is deliberate and
  costs nothing durable: the third party's write published its own `task.updated`, so a fresh
  refetch is already on its way.
- **Recordings are ordered by `tasks.updated_at`, and a strictly older recording is discarded.**
  A candidate recording is accepted only if the `updated_at` its payload carries for the task is
  **not older** than the `updated_at` carried by the field's currently recorded canonical value. A
  strictly older candidate is discarded for that field: neither recorded nor applied (AC-60).
  `updated_at` is the right column precisely because section F already establishes that it is
  stamped **inside** the write transaction, so its order matches true serialization order.
  - **This applies to all three record triggers**, a write's own response (AC-58) included, not only
    to refetches. That is load-bearing rather than tidiness: when two title commits overlap and the
    server commits them in the opposite order to the one the client issued them in, the two write
    responses carry the row's two successive `updated_at` values, so ordering by that column records
    whichever write the row actually kept. Recording by arrival order instead would leave the
    canonical value at the losing write. This is a second, independent reason AC-54 converges.
  - **Tiebreak, named.** When two instants are exactly equal, the later-arriving recording wins,
    ordered by this client's request-issue sequence for the task (AC-63). That sequence is a single
    per-task counter, incremented on **every** request this client issues for the task, fetch or
    write, so every candidate recording carries one. Equal `updated_at` means
    the same row version, so the field values agree and the choice is not observable; the tiebreak
    exists so the rule is total instead of leaving Build to invent one, and it does real work only
    where a second-resolution timestamp format is in play.
  - **Compare parsed instants, never raw strings, and note that the two formats are further apart
    than they look.** The three record triggers do not agree on serialization, and the disagreement
    is not a fractional-second detail. It is two different date syntaxes.
    - **The Office task DTO** feeds two of the three triggers, the initial load (AC-59) and every
      refetch (AC-49). It passes the SQLite `updated_at` column straight through as a raw string
      (`internal/office/repository/sqlite/tasks.go`, `TaskSearchResult.UpdatedAt string`). What is
      in that column is written by `updateTaskTx`, which binds `task.UpdatedAt` as a Go `time.Time`
      to `mattn/go-sqlite3`; that driver's default bind layout is
      `2006-01-02 15:04:05.999999999-07:00`: **space-separated, with a numeric offset (`+00:00`),
      never a `T`, never a `Z`, and not ISO-8601**. This is established in this repository, not
      inferred: `internal/analytics/repository/sqlite/stats.go` carries a comment recording the bug
      it already caused ("RFC3339 put a 'T' where every stored value has a space, and 'T' sorts
      above ' ', so rows silently lost their first day"); the same layout is referenced in
      `internal/office/repository/sqlite/failure.go` and
      `internal/task/repository/sqlite/worktree_ownership_normalize.go`; and
      `internal/office/dashboard/service_inbox.go`'s `parsedTaskUpdatedAt` needs **four** fallback
      layouts and still falls back to `time.Now()` when all four miss.
    - **A write's own response** (AC-58) carries `TaskDTO.UpdatedAt time.Time`
      (`internal/task/dto/dto.go`), marshalled by `encoding/json`, which is RFC3339Nano: the
      `T`/`Z` form. The `task.updated` WS payload uses the same form
      (`internal/task/service/service_events.go`), though that payload is **not** a record trigger;
      it is named here only so a builder does not add it as a fourth one.
    So a comparison between a refetch recording and a write recording is a comparison between
    `2026-08-20 14:30:05.123456789+00:00` and `2026-08-20T14:30:05.123456789Z`. As text they do not
    order at all; the space sorts below `T`, so the raw-string comparison is not merely
    imprecise, it is systematically wrong in one direction. The trailing-zero hazard is real too,
    and survives within the RFC3339Nano form alone, because RFC3339Nano strips trailing zeros
    from the fractional second, so `...:05.1Z` and `...:05.05Z` do not order correctly as text
    either.
    **The requirement:** parse every candidate's `updated_at` to an absolute instant before
    comparing, accepting **both** layouts above. The space-separated form must be parsed
    **explicitly**, treating its trailing offset as authoritative; it must not be handed to
    `Date.parse` / `new Date(...)` and assumed, because that string is not an ISO-8601 date-time and
    browsers do not agree on how or whether to parse it, with some treating it as local time. AC-66
    governs.
  - **A candidate whose `updated_at` cannot be parsed is discarded, and is never treated as
    newest.** If parsing fails, that candidate is dropped for that field: neither
    recorded as canonical nor applied to the displayed value. It must **not** fall back to the
    current time, and must not be accepted on the grounds that it arrived most recently. The backend
    precedent in `parsedTaskUpdatedAt` does fall back to `time.Now()`, and copying it here would be
    actively wrong rather than merely lax: this rule discards anything strictly **older** than the
    recorded value, so a "now" fallback always compares as newest, always wins, and silently defeats
    the entire ordering rule for exactly the malformed inputs it exists to protect against. Dropping
    the candidate is safe because no canonical value is lost: the field keeps the value it
    already had, and the next load, refetch or successful write records again. AC-67 governs.
  - **The first recording for a field is a seed and is never discarded.** The two rules above are
    comparisons, and until a field has a recorded canonical value there is nothing to compare
    against, so the first recording is made unconditionally: neither the staleness rule, nor the
    `updated_at` ordering rule, nor the parse rule can reject it (AC-72). This matters because
    section F's rollback target is the recorded canonical value (AC-64), so a field that reached an
    editable state with no canonical value at all would leave a failed commit with nothing to
    restore. AC-59 makes the initial page load the seed, and it necessarily precedes any edit,
    so in practice the seed is always in place before a commit can be issued.
  - **An unknown instant loses to a known one.** If the seed's `updated_at` could not be parsed,
    the field has a canonical value but no comparable instant. Any later candidate whose
    `updated_at` does parse supersedes it (AC-73), rather than being discarded for failing a
    comparison against something uncomparable. Without this the field would be pinned to the seed
    for the lifetime of the page.
  This rule closes two races the write-generation rule above does not reach: two refetches resolving
  out of order (AC-60), and a refetch issued *after* a commit that nonetheless read a pre-write
  snapshot and resolves after the write's own response (AC-62).
  **Which rule applies to which trigger, stated rather than left to be inferred.** A **refetch**
  response must survive **both** the staleness rule and this ordering rule to be recorded. A
  **write's own** response is subject to this ordering rule **only**; the staleness rule above is
  written against a refetch response and must not be extended to write responses. Extending it
  would be actively wrong rather than merely redundant: when two title commits overlap, the second
  is by definition committed after the first was issued, so a staleness test would discard the first
  commit's own response, and if the server happened to commit that first write last, the client
  would record the losing value and disagree with the row permanently.
- **The in-flight guard (applies to the displayed value only).** A canonical value is applied to a
  field's displayed value **only when that field has no unresolved write of this client's own and
  no open editor for it.** The guard is per field and covers three windows that would otherwise
  each lose a user's work:
  1. an **open editor** (AC-28): the draft is never touched;
  2. an **in-flight optimistic title**, where the editor has already closed at commit time (AC-3),
     so an editor-open test alone does not protect it (AC-38);
  3. a refetch triggered by an **unrelated concurrent mutation on the same task**, for example
     another property picker firing while a title PATCH is still in flight; that refetch can read a
     snapshot taken before this client's write and would otherwise revert it (AC-39).
  A field with no unresolved write and no open editor is unguarded, and a canonical value lands on
  it immediately, so an ordinary remote edit still appears promptly.
- **Deferred apply on release.** A field is **unguarded** when both conditions hold at once: it has
  no open editor and no unresolved write of this client's own. At the moment a field becomes
  unguarded, whether because its editor closed by any route (Save, Cancel, Escape, discard-confirm)
  or because its last in-flight write resolved, the client **applies the field's most recently
  recorded canonical value** to the displayed value if the two differ. If several canonical values
  were recorded while the field was guarded, only the most recent one is applied; the earlier ones
  are superseded, not queued. On the successful-save path this runs **after** the editor closes and
  AC-17 renders the saved value, and it can only ever move the display forward onto something newer,
  never back onto the pre-save text. Three rules together are what make that true, and all three are
  load-bearing: the save's own success has already recorded the persisted value as this field's
  canonical value (AC-58), so the canonical value is not stale to begin with; a response whose
  request predates the save is discarded as stale (AC-52); and any surviving response carrying an
  older `updated_at` is discarded by the ordering rule (AC-60). Earlier drafts of this document
  rested the same claim on AC-52 alone, which was wrong: a canonical value recorded legitimately
  **before** the save was issued is not stale under AC-52 and would have been applied over the saved
  text.
  This is the half of the convention the guard would otherwise drop. Without it, a canonical value
  that arrived while the field was guarded would be recorded and never shown: a remote edit landing
  during an open editor that the user then cancels would leave stale text on the page forever,
  because no further event is coming. AC-50 governs, and it is what keeps AC-27
  true in the guarded case. `apps/web/AGENTS.md` names this exact shape as the established
  convention for this class of race: "guard responses with per-scope revision and request/workspace
  generation; discard or refresh stale responses **and cover deferred responses**".
- **Guard lifetime: the guard is DERIVED, never independently set.** A field is guarded exactly when
  at least one of two underlying facts holds: an editor is open for it, or a write of this
  client's own to it is unresolved. The guard is computed from those two facts; it is **not** a
  third piece of state that one code path sets and another must clear. That shape is
  mandatory rather than stylistic, because a separately-mutated flag is precisely what can be left
  set by a path that never runs its clear, and a permanently-set guard is invisible: it produces no
  error, it silently freezes the field's Office task store entry for that task, and AC-61 makes the
  WebSocket handler honour it, so the task's cards on the list and board stop updating for the rest
  of the session with nothing to indicate why. The lifetime rules follow from the derivation and are
  stated so Build does not have to derive them:
  - **Creation.** No explicit creation step exists. A (task, field) pair is guarded from the moment
    one of the two facts becomes true. The guard is keyed by task and field, not by component, and
    there is at most one open-editor contribution per field because the Office simple pane is the
    only surface this feature gives an editor to. A second view of the same task does not add a
    second contribution.
  - **Editor closing, including unmount and route change.** When the detail page unmounts, or the
    route changes away from it, its open-editor contribution for both fields ends unconditionally,
    exactly as if the editor had been closed (AC-68). An unmount is not a special case that skips
    cleanup; it is the ordinary close path.
  - **A write resolving.** A write's contribution ends when it resolves, **whether it succeeded or
    failed** (AC-70). A failed write is a resolved write. This is called out because the failure
    path is the one a builder is most likely to leave without a release, and because it also
    records no canonical value, so nothing later would correct a guard left set by it.
  - **A write that resolves after the page is gone.** It still records its canonical value (AC-58
    is unconditional and does not depend on a mounted page) and it still ends its own
    contribution. The deferred apply then has no displayed value to write to, so it applies to the
    Office task store entry only (AC-69). The task's card therefore ends on the persisted value
    even though the user navigated away mid-write.
  - **Garbage collection.** No guard entry may outlive both of its underlying facts. Once a field
    has no open editor and no unresolved write it is unguarded and holds no state, and an
    implementation
    that keeps a per-task map must not leave a set entry behind for a task the user has left
    (AC-69).
- **Why the guard is needed at all here.** `refetchTask` in
  `apps/web/app/office/tasks/[id]/page.tsx` calls `setTask(mapOfficeTaskToTask(res.task))`,
  replacing the whole task object rather than
  merging fields, and `patchTaskInStore` writes the store entry the same way. Applying a response
  wholesale is therefore the default behaviour that has to be constrained.
- **The Office task store follows the same rule, with per-field timing.** A title edited on the
  detail page must update the task's card in list and board views. The **title** store patch is
  applied at **commit-issue** time, in lockstep with the optimistic render (AC-3), which is what
  makes AC-39's protection meaningful: the store already carries the unconfirmed value while the
  request is in flight. The **description** store patch is applied **only on success**, because
  section C makes description Save non-optimistic and an optimistic card preview would contradict
  it (AC-11, AC-56). Both require `title` and `description` to be carried by the store patch
  mapping in `apps/web/hooks/use-optimistic-task-mutation.ts`, which currently drops both.
- **There are TWO writers of these fields into the Office task store, and both consult the guard.**
  The guard is a property of the task and the field, not of the detail-page component. Naming only
  the first writer is what previously made AC-39 and AC-56 unsatisfiable:
  1. `patchTaskInStore`, reached from the optimistic-mutation hook and from the page's refetch path.
  2. **`apps/web/lib/ws/handlers/office.ts`.** Its `office.task.updated` handler calls
     `updateTaskStatus(taskId, normalizeIssueFields(p))`. `normalizeIssueFields` maps `p.title` and
     `p.description` off the event payload, and `updateTaskStatus` merges them into
     `state.office.tasks.items` with a raw `store.setState`, **synchronously on every event**,
     bypassing `patchTaskInStore` and everything this section constrains.
  For `title` and `description` both writers are subject to the per-field guard (AC-61). The
  practical consequence, which Build must plan for rather than discover: the guard state has to be
  reachable from a module-level store subscriber, so component-local state alone cannot satisfy
  AC-61. **Where** that state lives is Build's choice; that both writers read the same guard is not.
- **Why the WS handler is routed through the guard rather than stripped.** Two alternatives were
  considered and rejected, and this is recorded because the choice was left open rather than
  directed. **Stripping** `title` and `description` from `normalizeIssueFields`, so the guarded
  refetch path became the sole store writer, is the smaller diff and would also satisfy AC-39 and
  AC-56. It was rejected because that handler is shared by the Office list and board, where no
  editor is ever open and no write is ever in flight: stripping the two fields would make every
  remote title change on those surfaces wait for a refetch instead of landing synchronously, which
  is a regression for users on surfaces this feature otherwise does not touch. **Accepting** the
  handler as an unguarded writer, and weakening AC-39 and AC-56 to match, was rejected because
  AC-39 exists precisely for the routine case of an unrelated picker firing mid-write. Routing keeps
  the synchronous fast path for the common unguarded case, keeps both ACs intact, and puts the rule
  in one place, so a third writer added later inherits it instead of silently defeating it the way
  this one did.
- **The store's unguarded path is deliberately not ordered, and that is stated rather than left
  silent.** While a field is unguarded the WS handler writes the payload value into the store
  directly, and a refetch response may write a different value moments later. Both carry server
  state, WS delivery on a single connection is ordered, and any disagreement is corrected by the
  next event or refetch, so the window is bounded and self-correcting. No ordering guarantee is
  claimed for the store entry while the field is unguarded and **no test should assert one**. The
  `updated_at` ordering rule governs canonical recording, which is what the displayed value is
  derived from; it is not extended to the store's unguarded fast path.
- **Store rollback is field-scoped.** A failed title commit rolls back the store entry's `title`
  under the same sequence rule **and to the same target** as the displayed title (section F,
  "Rollback ordering": the currently recorded canonical value, AC-64), and rolls back nothing
  else. Without this the task's card keeps showing a rejected, unpersisted
  title permanently, because a failed write is never persisted and no `task.updated` is ever
  published to correct it. Note that `useOptimisticTaskMutation` today restores the **whole**
  `storeSnapshot` on failure, which would revert unrelated fields another writer changed in the
  meantime; it must not be reused unchanged for this. AC-55 governs.
- A manual title edit **clears any pending agent-generated title** for the task: `Service.UpdateTask`
  deletes the `agent_title_pending` and `agent_title_owner_session_id` metadata keys whenever a
  title is supplied, and the row is then written unconditionally. A human title always wins over an
  agent title that has not landed yet. A description-only edit leaves the pending marker intact, and
  the backend's conditional title write then applies; see section F.

### F. Ordering, concurrency, and retries

- **Ordering.** The two fields are independent single-column writes to one row; this feature
  introduces no list, no sort, and no tiebreak. When several edits are committed in sequence, the
  effective order is commit order, and the persisted `tasks.updated_at` is stamped **inside** the
  write transaction (`repository/sqlite/task.go`, `updateTaskTx`), so its ordering matches true
  serialization order.
- **Concurrency, general case.** `Service.UpdateTask` is a read-modify-write: it loads the row,
  applies the patch, and issues a full-row `UPDATE`. For every column **except** `title` and the
  title-related metadata keys there is no version or `updated_at` precondition, so two writers
  resolve **last-commit-wins**, and a write can carry a stale value for a field it did not intend to
  change if a different writer committed inside the window between its read and its write. This is
  pre-existing behaviour of the shared endpoint, it applies equally to every current caller, and
  this feature does not change it. Adding general optimistic concurrency control is named out of
  scope below.
- **Concurrency, the title exception (do not assume last-commit-wins here).** `updateTaskTx` is not
  a single statement. When the caller's in-memory snapshot still carries `agent_title_pending`, it
  issues a **conditional** update whose title assignment is
  `title = CASE WHEN <live pending marker> THEN ? ELSE title END`, together with a JSON merge of the
  title metadata. That is a genuine compare-and-swap against live database state, it exists so an
  agent-generated title cannot overwrite a human one that landed first, and it is covered by
  dedicated tests in `apps/backend/internal/task/repository/sqlite/task_title_cas_test.go`
  (`TestSetTaskTitleIfPendingRequiresTrueMarker`, `TestSetTaskTitleIfPendingRejectsNonOwner`,
  `TestUpdateTaskPreservesWinningTitleAgainstStaleUpdate`, `TestClaimTaskTitleSessionIsSingleOwner`).
  `Service.UpdateTask` acknowledges this in a comment: the post-write snapshot may be stale precisely
  because a conditional title patch was applied. **This feature exercises that path directly**: a
  description-only save on a task whose agent title is still pending goes through the conditional
  statement, and AC-48 requires it to be tested rather than assumed. Do not "simplify" this branch
  and do not treat that window as an unguarded race already excluded from scope.
- **Rollback ordering (overlapping title commits).** Title commits are sequenced. Each commit takes
  a monotonically increasing sequence number when it is issued. A failed commit restores the title
  **only if it is the last word standing**: only if there exists no later-sequenced commit that has
  already resolved successfully **and** no later-sequenced commit that is still unresolved.
  Otherwise the restore is skipped and the displayed title is left alone. The error is surfaced
  as a toast either way, and the same condition governs the store rollback (section E).
- **What a rollback restores: the field's currently recorded canonical value, read at the moment the
  restore runs.** It is **not** a per-commit snapshot of whatever happened to be displayed when that
  commit was issued. This definition is corrected from an earlier draft and the reasoning is
  recorded here so it is not rediscovered the hard way. A value captured at issue time can be the
  **optimistic value of an earlier commit that later failed**, a title the server never persisted;
  restoring it puts the page into precisely the state AC-44's rationale says must never occur. A
  canonical value cannot carry that defect by construction: section E records one only from the
  initial load, a surviving refetch, or a **successful** write, so every canonical value is one the
  server confirmed it holds. AC-64 governs. Two consequences, both load-bearing:
  - **Restoring is idempotent.** Two failed commits that both restore land on the same value, so the
    order in which they resolve cannot change the end state.
  - **The restore and section E's deferred apply agree by construction, not by sequencing.** Both
    write the same canonical value, so when a guard release and a rollback coincide it does not
    matter which runs first. No ordering between them is specified, and none is needed.
  The exclusions are load-bearing and none is redundant. The partition is over the state of
  **later-sequenced** commits, and all four of its cases are named:
  - **A later commit that already succeeded** (AC-44). Restoring here would silently revert a
    newer successful commit, leaving a title that is neither what the user typed nor what is
    stored.
  - **A later commit still in flight** (AC-53). The displayed title currently shows that newer
    commit's optimistic value. Restoring the older snapshot would overwrite it, and a commit
    *succeeding* re-renders nothing on its own (AC-3 renders at issue time only), so the newer
    commit's success would not correct the display. It would be corrected only later and only
    indirectly, when a canonical value arrives and the deferred apply runs, which is at least an
    event round-trip away and shows the user a title they did not type in the meantime. The
    narrower rule that named only the already-resolved case therefore mandated a mismatch it was
    written to prevent.
  - **A later commit that has already resolved with FAILURE is not an exclusion**, and does not
    block this commit's restore (AC-65). This is the fourth case and it is named explicitly rather
    than left to be inferred from "neither succeeded nor still unresolved", because leaving it
    implicit is what previously let a chained double failure land on a never-persisted title: under
    the old snapshot-based restore target, the later commit's snapshot was the earlier commit's
    optimistic value, so whichever of the two resolved last restored a title that had never been
    stored, and the end state depended on network ordering alone. It is safe now because of the
    corrected restore target above: both commits restore to the same canonical value, so the
    second restore is a no-op rather than a revert, and the end state is the same whichever order
    they resolve in.
  - **No later-sequenced commit at all** is the ordinary single-commit case and restores (AC-9).
- **Convergence after overlapping commits (both succeed).** Sequence numbers order the client's
  rollbacks; they do not reorder the network. If commit 1 sends `B` and commit 2 sends `C` and the
  requests reach the server out of order, the row keeps whichever write committed last, which may be
  `B`, while the display shows `C`. This feature does **not** add ordering guarantees to the wire
  and does not re-send to force a winner. It converges instead, and by a shorter path than an event
  round-trip: **each write's own response records the row's value for that field as canonical**,
  ordered by `updated_at` (section E), so by the time the later-resolving commit's response has
  landed, the canonical value is already whichever write the row actually kept. Once the last commit
  resolves the title is unguarded and the deferred-apply rule lands that value on the display. The
  refetches driven by each `task.updated` are a second, redundant path to the same end state rather
  than the mechanism convergence depends on. The end state is therefore the row's value, reached
  without a page reload, and the window in which the display disagrees with the row is bounded by
  the in-flight commits alone. AC-54 observes this. Users who care about which of two rapid renames
  wins are asking for general optimistic concurrency control, which is named out of scope below.
- **Late-arriving refresh.** A WS-driven refetch that lands while an editor is open must **not**
  overwrite the user's draft, and must not overwrite an in-flight optimistic value either; the
  per-field guard in section E states the full rule. The field's **canonical value** behind an open
  editor may change, and is recorded when it does; what the guard suppresses is only its
  **application to the displayed value**, and that application is deferred to the moment the editor
  closes rather than dropped (section E, "Deferred apply on release"). Meanwhile the editor's
  **baseline is frozen at open time** (section C for the description, section B for the title), so
  Save, Cancel, and Escape behaviour never changes underneath the user as a result of somebody
  else's edit. Those two statements are consistent because they are about different values: the
  canonical value moves, the displayed value and the baseline do not.
- **Idempotency.** A commit whose trimmed value equals the baseline or the current title issues no
  request. Committing the same value twice is therefore a single write followed by a no-op, and
  re-sending an identical `PATCH` converges on the same value: `title` and `description` are
  assigned, not accumulated, so N identical requests leave the same row as one. It is idempotent in
  that sense only. Each request is still a real write that re-stamps `updated_at` inside the
  transaction and republishes `task.updated`, so duplicates are observable as extra events and
  extra refetches even though the field value never moves. This feature adds no idempotency key and
  no duplicate-request suppression, and needs neither: it issues no automatic retries, and a commit
  equal to the baseline issues no request at all, so this client never sends a duplicate of its own.
- **Retries.** No automatic retry on failure. A failed title commit rolls back subject to the
  rollback-ordering rule above and toasts; a failed description save keeps the editor open with the
  draft and toasts. In both cases the next attempt is a fresh user action.

### G. Errors

- Network failure, `5xx`, or a task that no longer exists (`404`) means the edit does not apply, the
  prior value is restored (title) or the editor stays open with the draft (description), and the
  message surfaces as a toast.
- `400` from title validation is unreachable through the UI, because the input clamps at the same
  60-character limit the server enforces. If it is returned anyway, it is surfaced as a toast like
  any other failure; no special-cased inline error is required.
- The failure path never leaves an invisible unsaved draft. Either the editor is open and the draft
  is visible, or the editor is closed and the visible text is the stored text.

### H. Interaction details a builder would otherwise have to invent

- **Keyboard parity.** Both affordances are reachable and operable without a pointer. The title
  element and the description element (or its empty-state placeholder) are focusable, and Enter on
  the focused element opens the corresponding editor. Pointer activation is double-click for the
  title and single click for the description; the asymmetry is deliberate: a single click on a
  paragraph of prose is how every comparable product opens a description, while a single click on a
  page heading is not, and the title's double-click matches the existing Kanban rename.
- **Click versus text selection.** A pointer interaction on the description that ends with a
  non-empty text selection does not open the editor. Selecting and copying description text must
  remain possible.
- **The two editors are independent in state, but not symmetric about focus.** Neither editor ever
  **commits** the other, and a dirty description draft never blocks a title commit (AC-35). They are
  not, however, both freely open at once, and the asymmetry follows from blur:
  - The description editor survives blur (AC-21), so it stays open with its draft while the user
    opens, edits, and commits the title editor. That order is safe and AC-35 covers it.
  - The title editor **cancels on blur** (AC-5). Opening the description editor necessarily moves
    focus off the title input, whether by pointer or by keyboard, so the title editor closes and
    its draft is discarded. AC-57 states this explicitly rather than leaving it to be discovered.
  This is deliberate, not an oversight: the title editor is transient by design, matching the
  existing Kanban rename, and section C's rationale explains why a lost title draft is cheap (one
  line, at most 60 characters) while a lost description draft is not. A user who wants to keep a
  title draft commits it with Enter first. If a future card wants both editors genuinely co-open,
  that is a change to AC-5, not to this bullet.
- **Overlapping title commits.** A second title commit issued while the first is still in flight is
  allowed. Sequencing and the rollback rule are specified in section F under "Rollback ordering";
  that rule, not a per-commit snapshot alone, is what guarantees a failure never reverts a newer
  successful commit.
- **Cancel confirmation shape.** The AC-19 confirmation is a modal with two choices: discard the
  draft and close the editor, or return to the editor with the draft intact. Dismissing the modal by
  any other means (Escape, backdrop, close button) returns to the editor with the draft intact. The
  draft is only ever destroyed by the explicit discard choice.
- **Save and Cancel placement.** Save and Cancel render below the textarea, Save first and styled as
  the primary action, Cancel secondary. This is a fresh product decision for this feature, not a
  port: `project-header.tsx` has a single Save button rendered **above** its textarea and **no
  Cancel control at all**, so it is a precedent for gating Save on dirty state and for nothing else.
- **Invalid-title feedback.** AC-7's invalid state is both programmatic and visible: `aria-invalid`
  on the input plus a short visible message adjacent to it. It clears as soon as the draft becomes
  non-empty.
- **Stable test hooks.** The implementation exposes these `data-testid` values, and the E2E spec
  addresses the feature through them rather than through copy:
  `office-task-title`, `office-task-title-input`, `office-task-title-error`,
  `office-task-description`, `office-task-description-empty`, `office-task-description-textarea`,
  `office-task-description-save`, `office-task-description-cancel`,
  `office-task-description-discard-confirm`, `office-task-description-running-note`.

## Acceptance criteria

Written in EARS form. Each is observable from the Office task detail page or from the persisted row.

**Title**

- AC-1: WHEN the user double-clicks the task title on the Office simple pane, THE SYSTEM SHALL
  replace it with a focused single-line input seeded with the current title and its content
  selected.
- AC-2: WHEN the title element has keyboard focus AND the user presses Enter, THE SYSTEM SHALL
  enter the same edit state as AC-1.
- AC-3: WHILE the title editor is open AND the trimmed draft is non-empty AND differs from the
  stored title, WHEN the user presses Enter, THE SYSTEM SHALL render the **trimmed** draft
  immediately, send `PATCH /api/v1/tasks/:id` with a body containing only `title` set to that same
  trimmed value, and close the editor.
- AC-4: WHILE the title editor is open, WHEN the user presses Escape, THE SYSTEM SHALL discard the
  draft, close the editor, restore the stored title, and send no request.
- AC-5: WHILE the title editor is open, WHEN the input loses focus, THE SYSTEM SHALL behave exactly
  as in AC-4.
- AC-6: WHILE the title editor is open, WHEN the user enters or pastes text that would take the
  field beyond 60 code points, THE SYSTEM SHALL retain only the first 60 and SHALL leave the caret
  at the position the user was typing or pasting at, not at the end of the field.
- AC-7: WHILE the title editor is open AND the trimmed draft is empty, WHEN the user presses Enter,
  THE SYSTEM SHALL keep the editor open, mark the field invalid to assistive technology, and send no
  request.
- AC-8: WHILE the title editor is open AND the trimmed draft equals the stored title, WHEN the user
  presses Enter, THE SYSTEM SHALL close the editor and send no request.
- AC-9: IF the title `PATCH` fails AND no later-sequenced title commit has already resolved
  successfully AND no later-sequenced title commit is still unresolved, THEN THE SYSTEM SHALL
  restore the title's currently recorded canonical value (AC-64) and surface the error as a toast.
  (Both exclusions are required: this is the "last word standing" condition from section F, and
  AC-44 and AC-53 are the two cases it excludes. An earlier version of this criterion carried only
  the first exclusion, which made it mandate a restore in exactly the situation AC-53 forbids. A
  second earlier version restored a per-commit snapshot of the displayed title, which could be a
  never-persisted value; AC-64 replaced that target and AC-65 observes the case that exposed it.)
- AC-10: WHILE an IME composition is active, WHEN the user presses Enter, THE SYSTEM SHALL NOT
  commit the title and SHALL leave the editor open.
- AC-11: WHEN a title commit is **issued**, THE SYSTEM SHALL patch the task's entry in the Office
  task store with the trimmed draft at that moment, before the request resolves, so list and board
  surfaces show the new title without waiting for a WebSocket message. (Issue time, not success
  time: AC-39's protection of that store entry during the in-flight window is only meaningful if
  the entry already carries the unconfirmed value.)
- AC-44: IF a title `PATCH` fails AND a later-sequenced title commit has already resolved
  successfully, THEN THE SYSTEM SHALL leave the displayed title unchanged, SHALL NOT perform a
  restore, and SHALL still surface the error as a toast.
- AC-51: WHILE the title editor is open, THE SYSTEM SHALL use the title displayed at editor-open
  time as the frozen baseline for AC-4's restore target and AC-8's equality test, AND SHALL NOT
  change that baseline when a newer canonical title is recorded for the task.
- AC-53: IF a title `PATCH` fails AND a later-sequenced title commit is still unresolved, THEN THE
  SYSTEM SHALL leave the displayed title and the Office task store entry unchanged, SHALL NOT
  perform a restore, and SHALL still surface the error as a toast.
- AC-54: WHEN two overlapping title commits both succeed, THE SYSTEM SHALL converge the displayed
  title on the task's stored value once no title write is in flight and no title editor is open,
  without a page reload, regardless of the order in which the two requests reached the server.
- AC-55: IF a title commit fails AND the rollback condition in section F is met, THEN THE SYSTEM
  SHALL set the Office task store entry's `title` to the title's currently recorded canonical value
  and SHALL leave every other field of that store entry at its current value.
- AC-64: WHEN a failed title commit performs a restore, THE SYSTEM SHALL restore the title's
  currently recorded canonical value read at the moment the restore runs, and SHALL NOT restore any
  value captured from the display at commit-issue time.
- AC-65: WHEN two overlapping title commits both fail, THE SYSTEM SHALL leave the displayed title
  and the Office task store entry at the title's currently recorded canonical value, and SHALL reach
  the same end state regardless of which of the two commits resolves first.

**Description**

- AC-12: WHEN the user clicks the task description, THE SYSTEM SHALL replace it with a focused
  textarea seeded with the current description.
- AC-13: WHERE the task has no description, THE SYSTEM SHALL render a persistent placeholder
  affordance in the description's position, and WHEN the user clicks it, THE SYSTEM SHALL enter the
  same edit state as AC-12 with an empty draft.
- AC-14: WHILE the description editor is open AND the trimmed draft differs from the frozen
  baseline, THE SYSTEM SHALL display an enabled Save control and a Cancel control.
- AC-15: WHILE the description editor is open AND the trimmed draft equals the frozen baseline, THE
  SYSTEM SHALL display Save in a disabled state.
- AC-16: WHEN the user activates Save, THE SYSTEM SHALL send `PATCH /api/v1/tasks/:id` with a body
  containing only `description` set to the trimmed draft, SHALL keep the editor open in a saving
  state until the request resolves, and SHALL disable Save for the duration.
- AC-17: WHEN a description save succeeds AND the current trimmed draft still equals the submitted
  value, THE SYSTEM SHALL close the editor and render the saved value.
- AC-18: IF a description save fails, THEN THE SYSTEM SHALL keep the editor open with the draft
  unchanged and surface the error as a toast.
- AC-19: WHILE the description editor is open AND the trimmed draft differs from the frozen
  baseline, WHEN the user activates Cancel, THE SYSTEM SHALL require an explicit confirmation before
  discarding the draft.
- AC-20: WHILE the description editor is open AND the trimmed draft equals the frozen baseline, WHEN
  the user activates Cancel, THE SYSTEM SHALL close the editor immediately without a confirmation.
- AC-21: WHILE the description editor is open, WHEN the textarea loses focus, THE SYSTEM SHALL keep
  the editor open, keep the draft, and send no request.
- AC-22: WHILE the description editor is open AND the trimmed draft differs from the frozen
  baseline, WHEN the user presses Escape, THE SYSTEM SHALL take no action.
- AC-23: WHILE the description editor is open AND the trimmed draft equals the frozen baseline, WHEN
  the user presses Escape, THE SYSTEM SHALL close the editor.
- AC-24: WHEN a description save commits an empty trimmed draft on a task that previously had a
  description, THE SYSTEM SHALL clear the stored description and SHALL then render the placeholder
  affordance from AC-13.
- AC-25: WHILE at least one session on the task is in state `RUNNING`, WHEN the description editor
  is open, THE SYSTEM SHALL display a non-blocking note stating the change applies at the agent's
  next wakeup, and SHALL NOT disable Save.
- AC-40: WHILE the description editor is open, WHEN an `office.task.updated` refetch changes the
  task's stored description, THE SYSTEM SHALL leave the editor's frozen baseline unchanged, so that
  the enabled state of Save and the behaviour of Cancel and Escape do not change as a result.
- AC-41: WHILE a description save is in flight, THE SYSTEM SHALL keep the textarea editable, SHALL
  disable both Save and Cancel, and SHALL take no action on Escape.
- AC-42: WHEN a description save succeeds AND the user changed the draft while the save was in
  flight, THE SYSTEM SHALL keep the editor open with the newer draft intact, SHALL set the baseline
  to the value that was saved, and SHALL re-enable Save while the newer trimmed draft differs from
  it.
- AC-43: WHERE the stored description is empty, WHILE the description editor is open AND the draft
  contains only whitespace, THE SYSTEM SHALL treat the draft as equal to the baseline, SHALL keep
  Save disabled, and SHALL send no request.
- AC-45: WHEN the user saves a description of any length, THE SYSTEM SHALL send and persist it
  unmodified apart from whitespace trimming, applying no client-side or server-side length limit.
- AC-46: WHEN a description containing Markdown syntax is rendered, THE SYSTEM SHALL display it as
  literal plain text with whitespace preserved, and SHALL NOT render it as formatted Markdown.
- AC-56: WHEN a description save is issued, THE SYSTEM SHALL NOT patch the Office task store entry,
  and WHEN that save succeeds, THE SYSTEM SHALL patch the store entry's `description` at that
  moment, so card previews never show a description that has not been persisted.

**Shared**

- AC-26: WHEN either field is committed AND no other writer commits to the same task inside the
  request window, THE SYSTEM SHALL send only the edited field in the request body and SHALL leave
  every other task property unchanged in the response. This feature asserts no outcome when a
  concurrent writer does commit inside that window: `Service.UpdateTask` is last-commit-wins for
  every column except the existing title compare-and-swap (section F), this feature neither adds
  nor removes that behaviour, and no test should assert a required winner. Section F describes what
  the backend does today; it does not impose a new requirement here.
- AC-27: WHEN another client edits the same task, THE SYSTEM SHALL update the displayed title and
  description without a page reload: immediately on receipt of `office.task.updated` WHERE the
  field is unguarded, and otherwise at the moment that field becomes unguarded, per the deferred
  apply in AC-50. This criterion promises convergence without a reload; it does not promise
  application during a guarded window. AC-28 and AC-38 govern the guarded window and take
  precedence there. (An earlier version said only "on receipt", which contradicted the guard and
  would have failed a correct implementation.)
- AC-28: WHILE either editor is open, WHEN an `office.task.updated` refetch resolves for this task,
  THE SYSTEM SHALL leave the open draft unmodified.
- AC-38: WHILE a title commit is in flight AND the title editor has already closed, WHEN an
  `office.task.updated` refetch for this task resolves, THE SYSTEM SHALL leave the optimistically
  rendered title unchanged.
- AC-39: WHILE a title commit is in flight, WHEN a refetch triggered by an unrelated mutation on the
  same task resolves and carries a title predating that commit, THE SYSTEM SHALL leave both the
  displayed title and the Office task store entry unchanged.
- AC-49: WHEN a non-stale refetch for this task resolves, THE SYSTEM SHALL record its value for each
  field as that field's canonical value, whether or not an editor is open for that field and
  whether or not a write to that field is in flight.
- AC-50: WHEN a field becomes unguarded, because its editor closed by any route or because its last
  in-flight write resolved, THE SYSTEM SHALL apply that field's currently recorded canonical value
  to the displayed value if the two differ, so that a remote edit received while the field was
  guarded becomes visible without a page reload.
- AC-52: IF a refetch response's request was issued before this client's most recent committed write
  to a field, THEN THE SYSTEM SHALL discard that response for that field, neither recording it as
  canonical nor applying it to the displayed value.
- AC-58: WHEN a write to a field resolves successfully, THE SYSTEM SHALL record the value that
  request persisted for that field, read from the response body, as that field's canonical value,
  AND SHALL do so before releasing that field's guard.
- AC-59: WHEN the task detail page completes its initial load, THE SYSTEM SHALL record the loaded
  title and description as those fields' canonical values.
- AC-60: IF a candidate canonical recording carries an `updated_at` strictly older than the
  `updated_at` carried by the field's currently recorded canonical value, THEN THE SYSTEM SHALL
  discard the candidate for that field, neither recording nor applying it, so that two refetches
  resolving out of order leave the newer value in place.
- AC-61: WHILE a field is guarded, WHEN an `office.task.updated` event is received, THE SYSTEM SHALL
  leave that field's Office task store entry unchanged, INCLUDING when the store write originates in
  the WebSocket handler rather than in the page's refetch path.
- AC-62: IF a refetch was issued after a commit to a field AND its response carries an `updated_at`
  older than the one recorded by that commit's own successful response, THEN THE SYSTEM SHALL
  discard that response for that field.
- AC-63: WHEN two candidate recordings for a field carry exactly equal `updated_at` values, THE
  SYSTEM SHALL accept the later-arriving one, ordered by this client's request-issue sequence for
  the task.
- AC-66: WHEN comparing two candidate recordings' `updated_at` values, THE SYSTEM SHALL parse each
  to an absolute instant before comparing, SHALL accept both the space-separated offset-bearing
  layout emitted by the Office task DTO and the RFC3339Nano layout emitted by a write response, and
  SHALL NOT compare the values as raw strings.
- AC-67: IF a candidate recording's `updated_at` cannot be parsed to an absolute instant, THEN THE
  SYSTEM SHALL discard that candidate for that field, neither recording it as canonical nor applying
  it to the displayed value, and SHALL NOT treat it as newer than the recorded value.
- AC-68: WHEN the task detail page unmounts or the route changes away from it, THE SYSTEM SHALL end
  the open-editor contribution to the guard for both fields, so that a field with no unresolved
  write becomes unguarded.
- AC-69: WHEN a write resolves after the task detail page has unmounted, THE SYSTEM SHALL record its
  canonical value per AC-58, SHALL end that write's contribution to the guard, and SHALL leave no
  guard entry in place for a field that has no open editor and no unresolved write.
- AC-70: WHEN a write to a field fails, THE SYSTEM SHALL end that write's contribution to the
  field's guard exactly as it does for a write that succeeds.
- AC-71: WHEN a canonical value is recorded for `description`, THE SYSTEM SHALL normalise an absent
  value (`undefined`, `null`, or an omitted field) to the empty string before recording it, so that
  a guard release on a task with no description applies nothing.
- AC-72: WHEN a field has no recorded canonical value, THE SYSTEM SHALL record the next candidate
  for that field unconditionally, applying neither the staleness rule, nor the `updated_at` ordering
  rule, nor the parse rule to it.
- AC-73: IF a field's recorded canonical value carries an `updated_at` that could not be parsed,
  THEN THE SYSTEM SHALL accept the next candidate whose `updated_at` does parse, superseding the
  recorded value.
- AC-29: WHEN a title is committed on a task carrying a pending agent-generated title, THE SYSTEM
  SHALL persist the user's title and SHALL clear the task's pending-agent-title metadata.
- AC-48: WHEN a description-only save is committed on a task carrying a pending agent-generated
  title, THE SYSTEM SHALL persist the new description, SHALL leave the pending-agent-title metadata
  intact, and SHALL leave the stored title unchanged.
- AC-30: THE SYSTEM SHALL offer the title and description editors regardless of the task's status
  and regardless of any session state, introducing no status-based or lifecycle-based gate.
- AC-47: THE SYSTEM SHALL NOT introduce an archived-task gate on either editor, and SHALL NOT port
  the `isArchived` condition from `apps/web/components/task/task-top-bar-title.tsx`.
- AC-31: THE SYSTEM SHALL NOT queue an agent run, wake an agent, or interrupt a running session as
  a result of a title or description edit.

**Interaction details**

- AC-33: WHEN the title element or the description element has keyboard focus AND the user presses
  Enter, THE SYSTEM SHALL open the corresponding editor.
- AC-34: WHEN a pointer interaction on the description ends with a non-empty text selection, THE
  SYSTEM SHALL NOT open the description editor.
- AC-35: WHILE the description editor is open with a dirty draft, WHEN the user opens and commits
  the title editor, THE SYSTEM SHALL commit the title and SHALL leave the description editor open
  with its draft intact.
- AC-36: WHILE the AC-19 confirmation is displayed, WHEN the user dismisses it by any means other
  than the explicit discard choice, THE SYSTEM SHALL return to the description editor with the draft
  intact.
- AC-37: WHILE the title editor is open AND the trimmed draft is empty, THE SYSTEM SHALL mark the
  input `aria-invalid` and display a visible message, and SHALL clear both as soon as the trimmed
  draft is non-empty.
- AC-57: WHILE the title editor is open, WHEN the user opens the description editor, THE SYSTEM
  SHALL close the title editor, discard its draft, restore its frozen baseline title, and send no
  request.

**Documentation reconciliation**

- AC-32: `docs/specs/office/tasks.md` SHALL be amended so that its editable-property list at
  `### D. Inline editable properties` names Title and Description, and its `PATCH /tasks/:id` entry
  under `## API surface` states that title and description are edited through
  `PATCH /api/v1/tasks/:id` and are not fields of the Office endpoint. No other behaviour in that
  document changes.

## Surfaces touched

- `apps/web/components/task/simple/OfficeSimplePane.tsx`: the `<h1>` at line 512 and the
  description block at lines 513-517. The component is 574 lines against a 600-line lint ceiling, so
  the editors are expected to be extracted into sibling components rather than added inline.
- **AC-25's data source is already in scope**: `OfficeSimplePaneProps` declares
  `sessions: TaskSession[]`, and `TaskSession["state"]` already includes `"RUNNING"`. No new fetch,
  prop, or store field is required for the running-session note.
- `apps/web/hooks/use-optimistic-task-mutation.ts`: `toOfficeTaskPatch` must carry `title` and
  `description`, and the section E rules apply to the store patch it writes. **This hook does not
  fit both fields as it stands, and that is expected, not a defect to work around.** Its only shape
  is apply-patch-then-await: it calls `ctx.applyPatch` and `patchTaskInStore` **before**
  `await apiCall()`, and on failure restores the **whole** `storeSnapshot`. That matches the title
  (optimistic at issue time, AC-11) and does **not** match the description (store patch on success
  only, AC-56), and its whole-object rollback is wrong for both (AC-55 requires a field-scoped
  restore). Build is expected to reshape or extend the hook rather than silently accept pre-await
  timing for the description; which of the two it is is an implementation decision, but the timings
  in AC-11 and AC-56 and the field-scoping in AC-55 are not.
- `apps/web/app/office/tasks/[id]/page.tsx`: `refetchTask` replaces the whole task object, so the
  per-field in-flight guard from section E is applied here. It carries no request ordering of any
  kind today, and `useOfficeRefetch` (`apps/web/hooks/use-office-refetch.ts`) has no debounce, no
  coalescing and no in-flight cancellation, so concurrent refetches for one task are the normal case
  rather than an edge case. That is why AC-60 exists.
- **`apps/web/lib/ws/handlers/office.ts`: the second store writer.** `normalizeIssueFields` (the
  `p.title` / `p.description` mapping) and `updateTaskStatus` (the raw `store.setState`) are both
  in scope, because together they write these two fields into the Office task store outside
  `patchTaskInStore`. AC-61 governs. Prior art for the ordering half of section E, worth reading
  before implementing it: `nextRequestSequence` / `isLatestRequest` in
  `apps/web/hooks/use-office-workspace-data.ts` already give every other Office loader
  last-request-wins, for exactly this reason.
- `apps/web/lib/api/domains/kanban-api.ts`: `updateTask()`, used as-is. Note its return value is now
  load-bearing: it resolves to the updated `Task`, and AC-58 records that response's value for the
  written field as canonical. A call site that discards the response cannot satisfy AC-58.
- `docs/specs/office/tasks.md`: reconciliation per AC-32.
- i18n: new copy in `en` plus `pt-pt`, `zh-cn`, `zh-hk`, `zh-tw`. `common:save`, `common:cancel`,
  `task:description`, `task:taskTitle`, `task:saving`, `task:updateFailed` and
  `office:addADescription` already exist and should be reused rather than duplicated. One trap:
  `task.json` carries **both** `description` (value `"description"`, lowercase, used today as an
  inline `KeyValueRow` label) and `description2` (value `"Description"`, capitalized). Pick by the
  element: a standalone visible label wants `task:description2`, an inline or aria-only string
  wants `task:description`. Do not add a third key for the same word.

No backend file changes are required by this spec. AC-48 adds a test over existing backend
behaviour; it does not change it.

## Test expectations

Every acceptance criterion has a declared verification below. AC ids are enumerated
individually rather than as ranges, so coverage can be checked mechanically: every id from AC-1 to
AC-73 appears at least once in this section.

- **Unit (vitest)**, alongside `apps/web/components/task/simple/OfficeSimplePane.test.tsx`:
  - Title editor open / commit / cancel / blur / clamp / paste-clamp / empty / unchanged / IME paths:
    AC-1, AC-2, AC-4, AC-5, AC-6, AC-7, AC-8, AC-10, AC-33, AC-37.
  - Title trim on send and on optimistic render: AC-3.
  - Title rollback, **all four branches** of the sequence rule, one test each: the
    last-word-standing case with no later commit at all (AC-9); a later commit that already
    succeeded (AC-44); a later commit still unresolved (AC-53); and a later commit that already
    resolved with **failure**, which restores (AC-65). AC-53 is the branch whose absence previously
    made AC-9 mandate the wrong outcome, and the fourth branch is the one whose absence let a
    chained double failure land on a never-persisted title, so both are asserted explicitly and
    neither is folded into AC-9's test.
  - **The rollback target** (AC-64): a failed commit restores the title's currently recorded
    canonical value, not a value captured from the display at commit-issue time. Assert this
    directly, with a canonical value that differs from what was displayed when the commit was
    issued, so a snapshot-based implementation fails.
  - **The chained double failure, asserted in both resolution orders** (AC-65). Two overlapping
    title commits, both failing: assert the displayed title and the Office task store entry both end
    at the canonical (pre-chain, actually stored) title when the earlier commit resolves first, and
    again when the later one does. This is the regression test for the defect the four-branch
    partition and AC-64 were written to close; a test that runs only one resolution order would have
    passed against the broken spec.
  - Office task store patch mapping, and the timing split between the two fields: AC-11 (title
    patched at commit-issue time, asserted before the request resolves) and AC-56 (description not
    patched at issue time, patched on success).
  - Field-scoped store rollback on a failed title commit: AC-55 asserts the store entry's `title`
    is restored and that a second field of the same store entry, changed in the meantime, is not.
  - The title's frozen baseline under a canonical update: AC-51.
  - Description open / placeholder / dirty / Save / Cancel / confirm / blur / Escape paths: AC-12,
    AC-13, AC-14, AC-15, AC-16, AC-17, AC-18, AC-19, AC-20, AC-21, AC-22, AC-23, AC-24, AC-34,
    AC-36.
  - Frozen baseline under a refetch: AC-40. Draft survives a refetch while open: AC-28.
  - In-flight control state and in-flight editing: AC-41, AC-42.
  - Whitespace-only draft on an empty description: AC-43.
  - No length limit and plain-text rendering: AC-45, AC-46.
  - Running-session note: AC-25.
  - In-flight guard for the optimistic title, editor closed and unrelated-mutation refetch:
    AC-38, AC-39.
  - The canonical/displayed rules from section E, one test each: recording is never suppressed
    (AC-49); the deferred apply fires when the guard releases, asserted through the case that
    motivated it, a remote description edit arriving during an open editor that the user then
    cancels (AC-50); a response whose request predates the local write is discarded rather than
    recorded (AC-52).
  - **The three record triggers, enumerated in section E.** The initial load records both fields
    (AC-59). A successful write records the value its own response persisted, asserted **before**
    the guard releases (AC-58). The regression this pair exists to prevent is asserted directly and
    on **both** fields: with no refetch in play at all, a successful title commit and a successful
    description save each leave the displayed value at the saved text after the guard releases,
    rather than reverting to the pre-write value.
  - **The `updated_at` ordering rule**, one test per branch: two refetches resolving out of order
    leave the newer value in place (AC-60); a refetch issued after a commit but carrying an older
    `updated_at` than the commit's own response is discarded (AC-62); two candidates with exactly
    equal `updated_at` resolve by request-issue sequence (AC-63).
  - **Cross-format parsing** (AC-66). The fixtures must be the **two layouts that actually occur**,
    not two variants of one: a refetch recording in the Office task DTO's raw SQLite form
    (`2026-08-20 14:30:05.123456789+00:00`, space-separated and offset-suffixed) and a
    write-response recording in `RFC3339Nano` (`2026-08-20T14:30:05.123456789Z`). Assert the
    ordering comes out right in both directions. A raw string comparison must fail this test, and
    so must an implementation that parses only the `T`/`Z` form, and so must one that hands the
    space-separated form to `new Date(...)` and assumes local time. AC-60's earlier prescribed
    fixtures (one RFC3339Nano value with a stripped trailing zero and one at second resolution)
    exercised only
    the same-format hazard and would have passed against a parser that is wrong for the majority of
    real comparisons; keep one such pair as an additional case, but it is not the primary one.
  - **The seed rule** (AC-72): with no canonical value recorded for a field, the next candidate is
    recorded even when it would fail the staleness, ordering or parse rule, so a failed commit
    always has a restore target (AC-64). And (AC-73) where the seed's `updated_at` did not parse,
    a later candidate whose `updated_at` does parse supersedes it, so the field is not pinned to
    the seed for the lifetime of the page.
  - **Unparseable `updated_at`** (AC-67): a candidate whose timestamp does not parse is discarded
    for that field, the previously recorded canonical value survives unchanged, and the candidate is
    **not** treated as newest. Assert the surviving value explicitly, because a "fall back to now"
    implementation passes any test that only checks the candidate was not recorded verbatim.
  - **Guard lifetime** (AC-68, AC-69, AC-70). Unmounting the detail page while an editor is open
    leaves the field unguarded, so a subsequent `office.task.updated` reaches the store entry
    (AC-68). A write that resolves after unmount still records its canonical value and still ends
    its contribution, and the store entry ends at the persisted value (AC-69). A **failed** write
    ends its contribution exactly as a successful one does, asserted by showing that a later event
    reaches the store entry after the failure (AC-70). These three are the tests that would catch a
    guard implemented as an independently-mutated flag rather than derived from its two facts.
  - **Absent-description normalisation** (AC-71): on a task with no description, a guard release
    applies nothing and fires no store patch, with the load path supplying `description: undefined`
    and the write path supplying `""`. Without normalisation the two compare unequal and the test
    sees a spurious apply.
  - **The WebSocket handler is subject to the guard** (AC-61): with a title commit in flight, an
    `office.task.updated` carrying the pre-commit title leaves the Office task store entry
    unchanged. This test must drive the real `office.ts` handler path (`normalizeIssueFields` plus
    `updateTaskStatus`), not `patchTaskInStore`, or it asserts nothing.
  - Convergence after two overlapping title commits that both succeed, with the responses
    delivered out of order: AC-54.
  - Both editors independent, title commits while description draft is dirty: AC-35. **And the
    other order**, which is the asymmetric one: opening the description editor while the title
    editor is open closes the title editor and discards its draft (AC-57).
  - No status or lifecycle gate, and no archived gate: AC-30, AC-47.
  - Only the edited field is sent, single-writer case: AC-26.
- **Backend (Go)**, alongside `apps/backend/internal/task/service/`:
  - A description-only update on a task carrying a pending agent title leaves the pending metadata
    and the stored title intact: AC-48. This exercises the conditional title statement in
    `updateTaskTx` described in section F.
  - A title update on a task carrying a pending agent title persists the human title and clears the
    pending metadata: AC-29.
  - A title or description update publishes `task.updated` **without** `assignee_agent_profile_id`
    on the payload, so `queueTaskAssignedRun` cannot queue a run: AC-31. This is the regression test
    that makes AC-31 a contract rather than a code-reading, and it fails the moment somebody adds
    that field to `publishTaskEventNow`.
- **E2E (Playwright)**, a new spec alongside `apps/web/e2e/tests/office/property-pickers.spec.ts`:
  - Editing the title and the description on a real seeded Office task, asserting both the rendered
    value and the persisted row: AC-3, AC-16, AC-17, AC-26.
  - A second browser context observing the edit converge without a reload: AC-27.
  - One route-mocked failure asserting title rollback: AC-9.
  - One route-mocked failure asserting the description editor survives with its draft intact: AC-18.
- **AC-32** is a documentation change with no runtime behaviour. It is verified by inspection of
  `docs/specs/office/tasks.md` in the same pull request; Build must confirm both edits landed.

## Out of scope

Each exclusion below is a decision, not an omission.

- **General optimistic concurrency control on task writes.** `Service.UpdateTask` keeps
  last-commit-wins for every column except the title compare-and-swap that already exists and is
  described in section F. Adding a version column or an `updated_at` precondition for the remaining
  columns would alter every existing caller of the endpoint and belongs in its own card. This
  feature neither adds nor removes concurrency control in the backend.
- **Guaranteeing which of two rapid renames wins.** Sequence numbers order this client's rollbacks;
  they do not order the network, and this feature adds no wire ordering, no re-send, and no
  server-side arbitration. Two overlapping title commits resolve to whichever write the server
  committed last, and the display converges on it (AC-54). A caller who needs a defined winner is
  asking for the concurrency control excluded above.
- **Description version history, undo, or drafts persisted across page loads.** Explicit Save exists
  precisely because none of these do. A draft lives in component state and is lost on navigation or
  reload.
- **Markdown rendering or a rich-text editor for the description.** The field stays plain text with
  preserved whitespace, exactly as it renders today. AC-46 pins this.
- **Extending `PATCH /api/v1/office/tasks/:id`** to accept title or description. The Office endpoint
  and its `hasAnyField()` guard are unchanged, and a `{"title": "..."}` body sent to it continues to
  return `400 "no mutation fields supplied"`.
- **Waking or re-prompting the assigned agent on a content edit.** Deliberately not done; see
  section D. Making a description edit re-brief a running agent is a product decision with its own
  failure modes and needs its own spec.
- **A minimum or maximum length for the description**, and any change to the 60-character title
  limit.
- **Editing title or description anywhere else in Office**: the task list, board cards, the sidebar,
  the inbox, and the advanced / dockview mode are untouched.
- **The sidebar property pickers.** They auto-save optimistically by design
  (`docs/specs/office/tasks.md:60`) and are E2E-proven. No Save button is added to them.
- **The blank Created by / Started / Completed rows.** Tracked separately.
- **An archived-task gate.** Not an exclusion of behaviour but of a gate: the Office detail page
  renders no archived state and the backend enforces no archived precondition, so no gate is added
  and the Kanban `isArchived` condition is not ported. See section A and AC-47. Introducing archived
  semantics to this page is separate work.
