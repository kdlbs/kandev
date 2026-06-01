/**
 * Bridge-audit diff — pure logic for the Phase 2 second-pass WS-account
 * check. Lives outside `lib/query/bridge/` so it has zero dependency on
 * the TanStack-Query bridge runtime (which depends on `QueryClient`,
 * WS handlers, etc.) and can be unit-tested under vitest without the
 * `happy-dom` query-client overhead.
 *
 * Consumers:
 *  - `apps/web/e2e/helpers/ws-account.ts` re-exports `diffBridgeAudit`
 *    and `BRIDGE_SKIPPED_ACTIONS` so the fixture teardown can ask
 *    "for every WS event I received with a session_id, did the bridge
 *     actually mutate the cache, OR is the action on the documented
 *     Zustand-only allowlist?"
 *  - The bridge runtime (`lib/query/bridge/index.ts`) keeps its own
 *    copy of the allowlist constants. Two copies is intentional —
 *    the diff helper is pure and the bridge runtime owns the source
 *    of truth; if they diverge the e2e check will surface the gap.
 */

/**
 * Allowlist of actions that the TanStack-Query bridge intentionally
 * leaves Zustand-only OR that are control-plane request/response acks the
 * bridge never sees as notifications. See the header comment on the
 * source-of-truth copy in `apps/web/lib/query/bridge/index.ts` for the
 * full taxonomy; the two lists are kept in sync so the e2e check will
 * surface a divergence as a fresh drop.
 *
 * Receipts for these actions never generate a `cache-unchanged` or
 * `no-bridge-entry` drop.
 */
export const BRIDGE_SKIPPED_ACTIONS: ReadonlySet<string> = new Set<string>([
  // (1) Zustand-only notifications
  // session.state_changed is now bridged into the TQ taskSession caches by
  // bridge/session-state.ts (D4+D6 Stage 1), so it is no longer skipped.
  "message.queue.status_changed",
  "session.waiting_for_input",
  "input.requested",
  "permission.requested",

  // (2a) Subscription / focus lifecycle acks
  "session.subscribe",
  "session.unsubscribe",
  "session.focus",
  "session.unfocus",
  "task.subscribe",
  "task.unsubscribe",
  "run.subscribe",
  "run.unsubscribe",
  "user.subscribe",
  "user.unsubscribe",

  // (2b) Session lifecycle acks
  "session.launch",
  "session.ensure",
  "session.recover",
  "session.stop",
  "session.delete",
  "session.reset_context",
  "session.set_primary",
  "session.set_plan_mode",
  "session.set_mode",

  // (2c) Task-session polling acks
  "task.session.status",
  "task.session.list",
  "task.session",

  // (2d) Agent operation acks
  "agent.prompt",
  "agent.cancel",
  "agent.stop",
  "agent.logs",
  "agent.status",
  "agent.stdin",
  "agent.resize",
  "permission.respond",

  // (2e) Message queue mutation acks
  "message.queue.add",
  "message.queue.cancel",
  "message.queue.get",
  "message.queue.update",
  "message.queue.append",
  "message.queue.remove",

  // (2f) Task-plan request acks
  "task.plan.get",
  "task.plan.create",
  "task.plan.update",
  "task.plan.delete",
  "task.plan.revert",
  "task.plan.revisions.list",
  "task.plan.revision.get",

  // (2g) Session git query acks
  "session.git.snapshots",
  "session.git.commits",
  "session.cumulative_diff",
  "session.commit_diff",

  // (2h) Session file-review acks
  "session.file_review.get",
  "session.file_review.update",
  "session.file_review.reset",

  // (2i) Shell / vscode / port operation acks
  "session.shell.status",
  "shell.subscribe",
  "shell.input",
  "vscode.start",
  "vscode.stop",
  "vscode.status",
  "vscode.openFile",

  // (3) High-frequency streams — ring-buffer registry, not TQ cache.
  // Handlers appendToRing instead of setQueryData, so cacheChanged is
  // always false by design. See apps/web/CLAUDE.md "Streams".
  "session.shell.output",
  "session.process.output",
  "terminal.output",

  // (4) Zustand-only, not yet migrated. session.process.status is
  // low-frequency process lifecycle state still in the Zustand slice.
  "session.process.status",

  // (5) Consumed by a component-level client.on() subscription (file-tree
  // folder refresh in file-browser-hooks.ts), not a bridge handler.
  "session.workspace.file.changes",
]);

/**
 * Prefix allowlist — any action starting with one of these strings is
 * treated as bridge-skipped. Kept in sync with the source-of-truth copy
 * in `apps/web/lib/query/bridge/index.ts`.
 *
 *   `session.agentctl_` is NO LONGER skipped — session.agentctl_starting /
 *      _ready / _error are bridged into the TQ taskSession caches AND the
 *      agentctl status badge cache (qk.session.agentctl) by
 *      bridge/session-state.ts. (Note: these start with `session.`, so the
 *      `agentctl_` prefix below does NOT match them.)
 *   `agentctl_`         — in-container agentctl log/status channels.
 *   `user_shell.`       — user_shell request/response acks (all carry
 *      session_id in payload).
 */
export const BRIDGE_SKIPPED_PREFIXES: readonly string[] = ["agentctl_", "user_shell."];

export interface BridgeAuditEntry {
  action: string;
  sessionId: string | null;
  taskId: string | null;
  cacheChanged: boolean;
  mutationCount: number;
  timestamp: number;
}

/**
 * Subset of `WsAccountSnapshot` that the diff needs. Declared locally so
 * this module stays free of the `ws-account.ts` import (which would pull
 * in the runtime ring buffer just to read a type).
 */
export interface WsAccountReceivedEvent {
  seq: number;
  action: string;
  sessionId: string | null;
  /** Envelope type — only `"notification"` is bridge-eligible. */
  type?: string;
}

export interface WsAccountSnapshotLike {
  receivedEvents: WsAccountReceivedEvent[];
}

export type HandlerSideDropReason = "no-bridge-entry" | "cache-unchanged";

export interface HandlerSideDrop {
  action: string;
  sessionId: string | null;
  /**
   * `no-bridge-entry`: the WS receipt has no matching audit entry at all
   * (the bridge handler never ran for this action+session).
   * `cache-unchanged`: an audit entry exists but `mutationCount === 0`
   * (the handler ran but didn't change the cache — likely wrong query
   * key or a no-op write).
   */
  reason: HandlerSideDropReason;
}

/**
 * Returns true when the action is on the bridge-skipped allowlist (either
 * exact match in `skippedActions` or any prefix in `skippedPrefixes`
 * matches). Mirrors `isBridgeSkippedAction` in `lib/query/bridge/index.ts`.
 */
export function isBridgeSkippedAction(
  action: string,
  skippedActions: ReadonlySet<string> = BRIDGE_SKIPPED_ACTIONS,
  skippedPrefixes: readonly string[] = BRIDGE_SKIPPED_PREFIXES,
): boolean {
  if (skippedActions.has(action)) return true;
  for (const prefix of skippedPrefixes) {
    if (action.startsWith(prefix)) return true;
  }
  return false;
}

/**
 * Diff WS receipts (FE-side) against bridge cache-mutation entries.
 * Returns the list of events that were received but didn't cause a cache
 * mutation — handler-side drops.
 *
 * Filters applied before diffing (any one matches → not a drop):
 *  - The action is on the bridge-skipped allowlist (exact or prefix).
 *  - The event has no `sessionId` (non-session-scoped events don't go
 *    through the per-session bridges; the cache-mutation semantics
 *    don't apply).
 *
 * For each remaining received event we look for ANY audit entry that
 * matches on `action` AND `sessionId`:
 *  - No match at all → `no-bridge-entry` drop.
 *  - Match exists but no audit entry for the (action, sessionId) had
 *    `cacheChanged === true` → `cache-unchanged` drop.
 *  - At least one audit entry mutated the cache → no drop.
 */
export function diffBridgeAudit(
  feSnapshot: WsAccountSnapshotLike,
  bridgeAudit: readonly BridgeAuditEntry[],
  skippedActions: ReadonlySet<string> = BRIDGE_SKIPPED_ACTIONS,
  skippedPrefixes: readonly string[] = BRIDGE_SKIPPED_PREFIXES,
): HandlerSideDrop[] {
  const drops: HandlerSideDrop[] = [];
  for (const event of feSnapshot.receivedEvents) {
    if (event.sessionId === null) continue;
    // Only notifications go through bridge handlers. Responses and errors are
    // dispatched via pendingRequests in client.ts and never reach ws.on, so
    // they'd always show up as no-bridge-entry drops if we didn't skip them.
    // An empty type from older bundles (pre-Phase 2) is treated as notification
    // to preserve back-compat.
    if (event.type && event.type !== "notification") continue;
    if (isBridgeSkippedAction(event.action, skippedActions, skippedPrefixes)) continue;
    const matches = bridgeAudit.filter(
      (e) => e.action === event.action && e.sessionId === event.sessionId,
    );
    if (matches.length === 0) {
      drops.push({ action: event.action, sessionId: event.sessionId, reason: "no-bridge-entry" });
      continue;
    }
    if (!matches.some((e) => e.cacheChanged)) {
      drops.push({ action: event.action, sessionId: event.sessionId, reason: "cache-unchanged" });
    }
  }
  return drops;
}

/**
 * Format a handler-side-drop list for a test failure / warning message.
 * Capped at 20 entries to keep CI logs scannable.
 */
export function formatHandlerSideDrops(drops: readonly HandlerSideDrop[]): string {
  if (drops.length === 0) return "";
  const lines = drops
    .slice(0, 20)
    .map((d) => `  action=${d.action}  session=${d.sessionId ?? "<none>"}  reason=${d.reason}`);
  const more = drops.length > 20 ? `\n  ... and ${drops.length - 20} more` : "";
  return `${drops.length} WS event(s) received but never mutated the TQ cache:\n${lines.join("\n")}${more}`;
}
