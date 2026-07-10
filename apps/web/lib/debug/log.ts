/**
 * Namespaced debug logger for development.
 *
 * Active when any of the following is true:
 *   - `NODE_ENV !== "production"` (i.e. `make dev`)
 *   - `VITE_KANDEV_DEBUG=true` at build time (inlined into the bundle)
 *   - `window.__KANDEV_DEBUG === true` at runtime (set by `layout.tsx` when
 *     the server-side env var is present, e.g. `make start-debug`)
 *
 * The runtime `window` check exists because `make start-debug` re-uses the
 * already-built production web bundle and only flips the env var on the
 * server process. Without the runtime fallback, the inlined `process.env`
 * value stays `false` in the client bundle and no logs surface.
 *
 * The `debug()` call itself is free, but JavaScript evaluates its arguments
 * before the call — so callers that compute expensive values (O(n) maps,
 * `.reduce()`, spread of large objects) must guard with the exported constant:
 *
 *   if (isDebug()) { debug(...); }
 *
 * In a production build with no flag set, both `process.env` checks fold to
 * `false` and the `window` check short-circuits at runtime, so the guarded
 * block is effectively a no-op.
 *
 * Output format is logfmt-ish so logs are flat and grep/copy-friendly:
 *
 *   [namespace] message key1=value key2="value with space" key3={"nested":1}
 *
 * ## Filtering by task
 *
 * Most call sites only have a `sessionId` in scope, but triage usually happens
 * per *task*. When a session→task resolver is registered (the `StateProvider`
 * wires one up in dev — see `registerSessionTaskResolver`), every line that
 * carries a `sessionId` / `session_id` field is auto-annotated with a trailing
 * `task_id=<...>`:
 *
 *   [git-status:ws] status_update received sessionId=abc fileCount=3 task_id=t_42
 *
 * So `task_id=t_42` in the devtools console filter narrows *all* namespaces to a
 * single task. No call site has to thread the task ID through by hand — just keep
 * logging `sessionId` and the task ID rides along. (If a line already names a
 * task via `taskId` / `task_id`, it is left as-is.)
 *
 * ## Namespace convention
 *
 * Names use `<domain>:<aspect>` so a devtools console filter can narrow on
 * either part. Known namespaces in this codebase (keep this list current
 * when adding new loggers — it's the cheat-sheet for triage):
 *
 *   Git / Changes panel pipeline (bug: stale until refresh)
 *     [changes:visibility]   Changes tab auto-close/keep decision:
 *                            taskHasRepos, source task object, repositoryId,
 *                            repoCount, repoIds, and live Dockview panel IDs.
 *     [git-status:subscribe]  useSessionGitStatus subscribe/unsubscribe cycle
 *     [ws:dispatch]           every WS message — inbound notifications and
 *                             outbound sends (`message="send"`). Streaming
 *                             chunk traffic is denylisted.
 *     [git-status:ws]         git event handler — status_update / commit_created / ...
 *     [git-status:store]      setGitStatus — overwrite decision + prev/next counts
 *     [git-status:derive]     useSessionGit file aggregation across repos
 *
 *   Files panel pipeline (bug: stuck loading skeleton)
 *     [agentctl:status]       per-session agentctl status transitions
 *     [file-browser:load]     tree loader — init / ready-flip / start / retry / gave-up
 *     [file-browser:changes]  session.workspace.file.changes events + folder refresh
 *
 *   Task-create dialog (bug: "No compatible agent profiles for <executor>")
 *     [task-create:state]     dialog open-cycle reset decisions: draft vs
 *                              initial values, seeded repo/branch hints, and
 *                              source-mode/discovery reset.
 *     [task-create:selection] repository/executor/executor-profile auto-pick
 *                              decisions: localStorage id, validity, workspace
 *                              defaults from settings, selected source, and
 *                              races where another effect seeded rows first.
 *     [task-create:last-used] manual selector persistence: localStorage writes,
 *                              pending task_create_last_used payloads, DB sync
 *                              success/failure, and pending-sync recovery.
 *     [executor-compat:specs]  remote-auth catalog fetch (count + agent ids)
 *     [executor-compat]        per-agent compat decision: ok + reason
 *                              (no-spec / no-creds / files-match / env-secret / …)
 *     [executor-compat:autopick]
 *                              useDefaultSelectionsEffect decision per fire:
 *                              skip/defer/pick + reason + which profile was set.
 *                              Triage path when "No compatible" lingers after
 *                              specs land — diff the decision sequence against
 *                              the catalog log to localise the culprit.
 *     [executor-compat:workflow-autopick]
 *                              useWorkflowAgentProfileEffect decision per fire:
 *                              no-workflow / locked / locked-missing /
 *                              workflow-no-override. The last branch restores
 *                              localStorage `lastId` against the unfiltered
 *                              `agentProfiles` — set_to of an executor-incompatible
 *                              id is the smoking gun for that race.
 *
 *   Dockview column widths (bug: sidebar/center/right widths wrong during
 *   env-prepare and on first task switch with cleared localStorage)
 *     [dockview:widths]       Per-event width-pipeline snapshots:
 *                              build-default-{entry,done}     first paint / reset
 *                              env-switch-{resize,resize-col,done}
 *                                                              cross-task switch
 *                              preset-apply / preset-post-layout
 *                                                              layout-selector preset switch:
 *                                                              applied widths + snapshot before
 *                                                              and after the rAF api.layout
 *                              fixups-capture                 target captured vs cap in
 *                                                              applyLayoutFixups; caller=<chain>,
 *                                                              cols=<n>, sidebarOverCap=true means
 *                                                              the recorded target is unreachable
 *                              container-resize               DOM ResizeObserver fired
 *                              sash-drag-end                  user-released sash
 *                              store-sync                     live widths → store pinnedWidths
 *                              enforce-restore                target rewind via resizeView
 *                              Snapshot format (formatWidthsSnapshot):
 *                                L=240 C=842 R=320 cols=3 api=1402x900 tgt=L240/R320
 *                              `tgt=` is the pinned-targets map (drives the
 *                              enforcement loop); mismatch with L/R is the
 *                              smoking gun for a stale-target bug.
 *
 *   Chat panel rendering (bug: remote-executor agent reply persisted but UI
 *   doesn't render it until the user refreshes the page)
 *     [chat:virtuoso]                VirtuosoMessageList render-branch snapshots
 *                                    (fallback vs virtuoso) and VirtuosoBody mount —
 *                                    captures itemCount / firstItemIndex /
 *                                    initialTopMostItemIndex at the moment Virtuoso
 *                                    first anchors its scroll. If itemCount at
 *                                    `mount` is < the final item count, Virtuoso
 *                                    anchored on an earlier item and the new last
 *                                    item lands below the fold.
 *     [chat:virtuoso:scrollParent]   `useVisibleScrollParent` lifecycle —
 *                                    ref-callback-ready / ref-callback-defer /
 *                                    ro-attach / ro-ready. A long delay between
 *                                    items growing and `ro-ready` firing is the
 *                                    smoking gun for the mount-too-early race.
 *     [chat:virtuoso:firstIndex]     `useStableFirstItemIndex` transitions —
 *                                    init + key-list deltas. A non-monotonic
 *                                    `delta` between two transitions means
 *                                    Virtuoso saw the keyspace shift in a way
 *                                    that throws off scroll anchoring.
 *     [chat:prepare-progress]        PrepareProgress status / autoExpand / expanded
 *                                    transitions per session. Status stuck on
 *                                    "preparing" with `expanded=true` while
 *                                    Virtuoso is mounted explains the
 *                                    agent-reply-pushed-below-fold scenario.
 *
 *   Agent running-state (bug: ACP turn completes but chat still shows the agent
 *   as running). The chat "agent is working" indicator is driven by
 *   taskSessions.items[sessionId].state === "RUNNING"; it only clears when a
 *   session.state_changed WS event flips new_state off RUNNING.
 *     [session:state]         session.state_changed handler — old→new state per
 *                             session. If you never see a line transitioning
 *                             newState off RUNNING after the turn completes, the
 *                             backend never published the clear (look at the
 *                             orchestrator complete-event guard / handleAgentCompleted).
 *     [session:turns]         session.turn.started / session.turn.completed
 *                             handlers. A `turn.completed` line with no following
 *                             `[session:state] newState=WAITING_FOR_INPUT` is the
 *                             smoking gun: the turn ended but running-state stuck.
 *     [task-lifecycle:ws]     task/session lifecycle WS cache merge trace:
 *                             task.created / task.updated / task.state_changed
 *                             logs payload-vs-existing primary session state,
 *                             and session.state_changed logs whether kanban and
 *                             multi-workflow snapshots were patched. For stale
 *                             kanban spinners, look for task.updated preserving
 *                             beforeTaskPrimaryState=RUNNING after the session
 *                             store already moved to WAITING_FOR_INPUT.
 *     [kanban:task-status]    Kanban card mismatch detector. Emits only when
 *                             the card would render a spinner from
 *                             task.primarySessionState while the loaded
 *                             taskSessions row for the primary session would
 *                             not spin. This is the direct smoking gun for
 *                             sidebar-correct / kanban-still-spinning reports.
 *
 *   Review / Diff pipeline (bug: Review or Diff panel shows empty when
 *   commits exist — distinguishes "no cumulative diff" vs "missing PR data"
 *   vs "gh rate-limited" by tracing each source independently)
 *     [review:cumulative]     useCumulativeDiff fetch lifecycle —
 *                             fetch.start / fetch.success (with fileCount) /
 *                             fetch.error / cache.invalidated /
 *                             fetch.skip.in-flight / fetch.drain.pending. A
 *                             success with fileCount=0 means the backend
 *                             computed the merge-base diff and there are no
 *                             committed changes ahead of base. A skip.in-flight
 *                             followed by drain.pending then a fresh fetch.start
 *                             means an invalidation arrived mid-fetch and was
 *                             correctly coalesced (the Review dialog shows the
 *                             freshest worktree state, not the snapshot the
 *                             original git diff captured).
 *     [review:pr-diff]        usePRDiff fetch lifecycle — fetch.start /
 *                             fetch.success (with fileCount) / fetch.error
 *                             (with error message — surfaces gh rate-limit
 *                             and auth failures as the literal error string).
 *     [review:sources]        Merged review/diff list — total + per-source
 *                             counts (uncommitted/committed/pr) + load flags
 *                             + hasPR + hasCumulativeDiff. The smoking-gun
 *                             line: total=0 with hasPR=true & hasCumulativeDiff=true
 *                             means both sources returned but were empty;
 *                             total=0 with prDiffLoading=true means the PR
 *                             fetch is still in flight; total=0 with
 *                             hasPR=true & no preceding [review:pr-diff]
 *                             fetch.success/error means the PR fetch never
 *                             fired (look at useActiveTaskPR plumbing).
 *
 *   Other
 *     [ws:connection]         WS hook mount + status transitions
 *     [dockview:*]            layout restore / save / env-switch / session-tabs / task-select
 *     [messages:*]            message fetch / process / lazyload
 *     [session:env-mapping]   session → environment ID mapping
 *
 * Tip: in Chrome devtools the console filter input takes substrings and regex.
 * Use `[git-status:` for the whole git pipeline, `[ws:dispatch] action=session.git`
 * to scope WS dispatch to git, etc.
 *
 * Usage:
 *   const debug = createDebugLogger("git-status:ws");
 *   debug("status_update received", { sessionId, fileCount });
 *
 * Logs go through `console.debug`, which the log interceptor mirrors into the
 * ring buffer (see `lib/logger/intercept.ts`), so they also end up in
 * Improve Kandev reports without extra plumbing.
 */

export type DebugLogger = (...args: unknown[]) => void;

let debugCached: boolean | undefined;

/** Whether namespaced debug logging is active. Evaluated lazily so production start-debug works. */
export function isDebug(): boolean {
  if (debugCached !== undefined) return debugCached;
  const env = getViteEnv();
  const nodeEnv = typeof process !== "undefined" ? process.env.NODE_ENV : undefined;
  const viteDev = env.DEV === true && nodeEnv !== "production";
  const processDev = typeof process !== "undefined" && process.env.NODE_ENV !== "production";
  const processDebug =
    typeof process !== "undefined" &&
    (process.env.KANDEV_DEBUG === "true" || process.env.VITE_KANDEV_DEBUG === "true");
  debugCached =
    viteDev ||
    processDev ||
    env.VITE_KANDEV_DEBUG === "true" ||
    processDebug ||
    (typeof window !== "undefined" && window.__KANDEV_DEBUG === true);
  return debugCached;
}

function getViteEnv(): {
  DEV?: boolean;
  VITE_KANDEV_DEBUG?: string;
} {
  return (
    (
      import.meta as unknown as {
        env?: {
          DEV?: boolean;
          VITE_KANDEV_DEBUG?: string;
        };
      }
    ).env ?? {}
  );
}

const BARE_VALUE_RE = /^[A-Za-z0-9_\-:./@+]+$/;

function formatValue(value: unknown): string {
  if (value === null) return "null";
  if (value === undefined) return "undefined";
  if (typeof value === "string") {
    return BARE_VALUE_RE.test(value) ? value : JSON.stringify(value);
  }
  if (typeof value === "number" || typeof value === "boolean" || typeof value === "bigint") {
    return String(value);
  }
  if (value instanceof Error) {
    return JSON.stringify({ name: value.name, message: value.message });
  }
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  if (value === null || typeof value !== "object") return false;
  if (Array.isArray(value)) return false;
  const proto = Object.getPrototypeOf(value);
  return proto === Object.prototype || proto === null;
}

function flattenArgs(args: unknown[]): string {
  const parts: string[] = [];
  for (const arg of args) {
    if (typeof arg === "string") {
      parts.push(arg);
      continue;
    }
    if (isPlainObject(arg)) {
      for (const [key, val] of Object.entries(arg)) {
        parts.push(`${key}=${formatValue(val)}`);
      }
      continue;
    }
    parts.push(formatValue(arg));
  }
  return parts.join(" ");
}

export type SessionTaskResolver = (sessionId: string) => string | undefined;

let sessionTaskResolver: SessionTaskResolver | null = null;
let sessionTaskResolverToken = 0;

/**
 * Register a function that maps a session ID to its owning task ID, and return
 * an unregister callback.
 *
 * Most debug call sites only have a `sessionId` in scope, but when triaging a
 * problem you usually think in terms of a *task*. Once a resolver is registered,
 * every `debug(...)` call that carries a `sessionId` / `session_id` field is
 * automatically annotated with a trailing `task_id=<...>` — so a devtools
 * console filter (or a grep over an exported log) can scope to a single task
 * without every call site having to thread the task ID through.
 *
 * The frontend store is created per-`StateProvider` (it is not a module-level
 * singleton), so the provider wires this up on mount and calls the returned
 * unregister on unmount — see `components/state-provider.tsx`. The resolver
 * itself *is* a single module-level global, so the unregister callback only
 * clears it when this registration is still the active one: during HMR or a
 * provider swap the new provider mounts (and registers) before the old one's
 * cleanup runs, and an unconditional null-clear would silently kill annotation
 * until a full reload. Calls are no-ops in production because `isDebug()` is
 * false and `createDebugLogger` skips output.
 */
export function registerSessionTaskResolver(resolver: SessionTaskResolver | null): () => void {
  const token = ++sessionTaskResolverToken;
  sessionTaskResolver = resolver;
  return () => {
    if (sessionTaskResolverToken === token) sessionTaskResolver = null;
  };
}

const SESSION_ID_KEYS = ["sessionId", "session_id"];
const TASK_ID_KEYS = ["taskId", "task_id"];

function readStringKey(args: unknown[], keys: string[]): string | undefined {
  for (const arg of args) {
    if (!isPlainObject(arg)) continue;
    for (const key of keys) {
      const val = arg[key];
      if (typeof val === "string" && val.length > 0) return val;
    }
  }
  return undefined;
}

/**
 * Build the trailing ` task_id=<...>` annotation for a log line from the
 * `sessionId` it already carries. Returns "" (no annotation) when there is no
 * resolver, the line already names a task, no session is present, or the
 * session can't be mapped. Never throws — a faulty resolver must not break a
 * debug log.
 */
function resolveTaskAnnotation(args: unknown[]): string {
  if (!sessionTaskResolver) return "";
  if (readStringKey(args, TASK_ID_KEYS)) return ""; // already filterable by task
  const sid = readStringKey(args, SESSION_ID_KEYS);
  if (!sid) return "";
  try {
    const taskId = sessionTaskResolver(sid);
    return taskId ? ` task_id=${formatValue(taskId)}` : "";
  } catch {
    return "";
  }
}

export function createDebugLogger(namespace: string): DebugLogger {
  const prefix = `[${namespace}]`;
  return (...args: unknown[]) => {
    if (!isDebug()) return;
    console.debug(`${prefix} ${flattenArgs(args)}${resolveTaskAnnotation(args)}`);
  };
}
