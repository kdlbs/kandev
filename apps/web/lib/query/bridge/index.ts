import type { QueryClient } from "@tanstack/react-query";
import type { WebSocketClient } from "@/lib/ws/client";
import { registerFeaturesBridge } from "./features";
import { registerCommentsBridge } from "./comments";
import { registerWorkspaceBridge } from "./workspace";
import { registerSettingsBridge } from "./settings";
import { registerAutomationsBridge } from "./automations";
import { registerIntegrationsBridge } from "./integrations";
import { registerGithubBridge } from "./github";
import { registerGitlabBridge } from "./gitlab";
import { registerJiraBridge } from "./jira";
import { registerLinearBridge } from "./linear";
import { registerKanbanBridge } from "./kanban";
import { registerOfficeBridge } from "./office";
import { registerSessionBridge } from "./session";
import { registerSessionStateBridge } from "./session-state";
import { registerSessionRuntimeBridge } from "./session-runtime";
import { registerSessionRuntimeStreamsBridge } from "./session-runtime-streams";

export interface QueryBridgeOptions {
  /** Returns the currently-active workspace ID, or undefined if none. */
  getActiveWorkspaceId: () => string | undefined;
  /** Resolves sessionId → environmentId for session-runtime cache key routing. */
  getEnvKey: (sessionId: string) => string;
  /**
   * Keeps the Zustand `environmentIdBySessionId` client index populated from the
   * session-state bridge (D6 stays Zustand-backed during the staged migration).
   */
  setEnvMapping: (sessionId: string, environmentId: string) => void;
  /**
   * True when the session is rendered on an ephemeral surface (quick-chat /
   * config-chat) rather than the main task chat. Read from Zustand UI state,
   * which remains client-only. Used by the session bridge to suppress the
   * empty-turn notice on those surfaces.
   */
  isEphemeralSurface: (sessionId: string) => boolean;
}

// ---------------------------------------------------------------------------
// Bridge audit (Workstream 0 of ws-event-accounting)
// ---------------------------------------------------------------------------

/**
 * WS actions intentionally NOT mirrored into the TanStack Query cache.
 *
 * Three categories of entry, all of which the audit helper treats as
 * "received but legitimately did not produce a bridge cache mutation":
 *
 *   1. Zustand-only server-state notifications. The slice authority lives
 *      in `lib/ws/handlers/<domain>.ts` and the UI reads from Zustand.
 *      `session.state_changed` is the canonical example.
 *
 *   2. Control-plane request/response acks. Every FE-initiated WS request
 *      (`subscribe`, `focus`, `task.session.status`, ...) gets the same
 *      `action` echoed back on the response envelope. Responses are
 *      handled via `pendingRequests` resolve/reject, never `ws.on()` —
 *      so no bridge handler can run for them. Each ack with a session_id
 *      in its payload would otherwise read as a `no-bridge-entry` drop.
 *
 *   3. Notifications that drive client-only effects (toasts, focus
 *      tracking, polling-tier hints) where the bridge has nothing to
 *      cache. `session.waiting_for_input` is just a desktop-notification
 *      trigger; `input.requested` is consumed by an in-page event bus.
 *
 * Removing an entry is a one-line change once a future bridge wave moves
 * the action into the TQ cache, or once the receipt-level audit learns to
 * skip `type === "response"` envelopes structurally.
 */
export const BRIDGE_SKIPPED_ACTIONS: ReadonlySet<string> = new Set([
  // (1) Zustand-only notifications
  // NOTE: session.state_changed is now bridged into the TQ taskSession by-id /
  // by-task caches by bridge/session-state.ts (D4+D6 Stage 1). The Zustand
  // agent-session handler still owns adoption / promotion / failure toasts.
  "message.queue.status_changed", // queue UI tightly coupled to Zustand
  "session.waiting_for_input", // desktop Notification trigger only
  "input.requested", // permission/input UI event bus
  "permission.requested", // permission dialog UI bus, no cache key

  // (2a) Control-plane acks: subscription / focus lifecycle
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

  // (2b) Control-plane acks: session lifecycle operations
  "session.launch",
  "session.ensure",
  "session.recover",
  "session.stop",
  "session.delete",
  "session.reset_context",
  "session.set_primary",
  "session.set_plan_mode",
  "session.set_mode",

  // (2c) Control-plane acks: task-session polling / queries
  "task.session.status",
  "task.session.list",
  "task.session",

  // (2d) Control-plane acks: agent operations (session-scoped)
  "agent.prompt",
  "agent.cancel",
  "agent.stop",
  "agent.logs",
  "agent.status",
  "agent.stdin",
  "agent.resize",
  "permission.respond",

  // (2e) Control-plane acks: message queue mutations
  // (the corresponding queue.status_changed notification is also allowlisted above)
  "message.queue.add",
  "message.queue.cancel",
  "message.queue.get",
  "message.queue.update",
  "message.queue.append",
  "message.queue.remove",

  // (2f) Control-plane acks: task-plan request/response
  // (task.plan.created / updated / deleted / revision.created / reverted
  //  notifications ARE bridged in session.ts; these are the request actions.)
  "task.plan.get",
  "task.plan.create",
  "task.plan.update",
  "task.plan.delete",
  "task.plan.revert",
  "task.plan.revisions.list",
  "task.plan.revision.get",

  // (2g) Control-plane acks: session git queries (session_id in payload)
  "session.git.snapshots",
  "session.git.commits",
  "session.cumulative_diff",
  "session.commit_diff",

  // (2h) Control-plane acks: session file-review queries
  "session.file_review.get",
  "session.file_review.update",
  "session.file_review.reset",

  // (2i) Control-plane acks: shell / vscode / port operations
  "session.shell.status",
  "shell.subscribe",
  "shell.input",
  "vscode.start",
  "vscode.stop",
  "vscode.status",
  "vscode.openFile",

  // (3) High-frequency streams — handled by the ring-buffer registry
  // (lib/query/streams/ring.ts), NOT the TQ cache. TQ's per-chunk notify
  // is a perf cliff at thousands of chunks/sec (see apps/web/CLAUDE.md
  // "Streams"). These DO have bridge handlers, but they appendToRing
  // instead of setQueryData, so cacheChanged is always false by design.
  "session.shell.output",
  "session.process.output",
  "terminal.output",

  // (4) Zustand-only, not yet migrated to TQ. session.process.status is
  // low-frequency process lifecycle state that still lives in the Zustand
  // session-runtime slice (upsertProcessStatus) — see the comment in
  // bridge/session-runtime-streams.ts. Migrate + remove this entry when
  // the process-status UI reads from TQ.
  "session.process.status",

  // (5) Consumed by a component-level client.on() subscription, not a bridge
  // handler. session.workspace.file.changes drives an imperative file-tree
  // folder refresh in components/task/file-browser-hooks.ts; it intentionally
  // refetches affected folders rather than mutating a single TQ cache entry,
  // so it never flows through wrapBridgeHandler.
  "session.workspace.file.changes",
]);

/**
 * Action prefixes intentionally not mirrored into TanStack Query.
 *
 * NOTE: `session.agentctl_` (starting / ready / error) is NO LONGER skipped —
 * bridge/session-state.ts mirrors those events into the TQ taskSession caches
 * (env mapping + worktree fields) AND the agentctl status badge cache
 * (qk.session.agentctl). The status badge is now fully TQ-backed.
 *
 * `agentctl_` covers the in-container agentctl log / status channels that
 * surface via the agentctl HTTP API rather than the per-session bridge. (Note:
 * `session.agentctl_*` does NOT match this prefix — those start with `session.`
 * and are bridged above.)
 *
 * `user_shell.` covers every user_shell operation (list / create / stop /
 * destroy / rename / park / resume); all carry session_id in payload and
 * are pure request/response.
 */
export const BRIDGE_SKIPPED_PREFIXES: readonly string[] = ["agentctl_", "user_shell."];

/** Returns true when an action is intentionally excluded from bridge coverage. */
export function isBridgeSkippedAction(action: string): boolean {
  if (BRIDGE_SKIPPED_ACTIONS.has(action)) return true;
  return BRIDGE_SKIPPED_PREFIXES.some((p) => action.startsWith(p));
}

/**
 * One row in the bridge audit ring buffer. Records that the FE both received
 * an envelope AND ran a bridge handler for it, and whether that handler
 * actually mutated the TanStack Query cache.
 */
export interface BridgeAuditEntry {
  action: string;
  sessionId: string | null;
  taskId: string | null;
  cacheChanged: boolean;
  mutationCount: number;
  timestamp: number;
}

const AUDIT_BUFFER_SIZE = 5000;

// Module-level mutable so tests can flip the gate. Production reads it once
// per `wrapBridgeHandler` call (i.e. once at bridge-registration time) and
// returns the underlying handler unchanged when off — zero per-event overhead.
let bridgeAuditEnabled =
  typeof process !== "undefined" && process.env.NEXT_PUBLIC_KANDEV_E2E_MOCK === "true";

// Insertion-ordered Map ring buffer: O(1) append AND eviction (a plain array
// with .shift() is O(N) per eviction, which adds up once the buffer is full).
// values() yields in insertion order, so consumers see the same ordering as a
// plain array would. Mirrors the FE WsAccount ring in lib/ws/ws-account.ts.
const auditBuffer = new Map<number, BridgeAuditEntry>();
let auditSeq = 0;

declare global {
  interface Window {
    __kandev_bridge_audit__?: () => BridgeAuditEntry[];
    __kandev_bridge_audit_clear__?: () => void;
  }
}

function installWindowAuditAccessors(): void {
  if (typeof window === "undefined") return;
  window.__kandev_bridge_audit__ = () => Array.from(auditBuffer.values());
  window.__kandev_bridge_audit_clear__ = () => {
    auditBuffer.clear();
  };
}

if (bridgeAuditEnabled) {
  installWindowAuditAccessors();
}

/** Test-only: flip the gate. Production code must not call this. */
export function __setBridgeAuditEnabledForTests(enabled: boolean): void {
  bridgeAuditEnabled = enabled;
  if (enabled) {
    installWindowAuditAccessors();
  } else if (typeof window !== "undefined") {
    delete window.__kandev_bridge_audit__;
    delete window.__kandev_bridge_audit_clear__;
  }
}

/** Test-only helper. */
export function __getBridgeAuditBufferForTests(): BridgeAuditEntry[] {
  return Array.from(auditBuffer.values());
}

/** Test-only helper. */
export function __clearBridgeAuditBufferForTests(): void {
  auditBuffer.clear();
}

function pushAuditEntry(entry: BridgeAuditEntry): void {
  auditBuffer.set(auditSeq++, entry);
  while (auditBuffer.size > AUDIT_BUFFER_SIZE) {
    const oldest = auditBuffer.keys().next().value;
    if (oldest === undefined) break;
    auditBuffer.delete(oldest);
  }
}

function readStringField(payload: unknown, key: string): string | null {
  if (typeof payload !== "object" || payload === null) return null;
  const value = (payload as Record<string, unknown>)[key];
  return typeof value === "string" ? value : null;
}

/**
 * Wraps a bridge handler so it records to the audit ring buffer when
 * `NEXT_PUBLIC_KANDEV_E2E_MOCK === "true"`. Returns the original handler
 * unchanged when the gate is off (production / non-e2e builds), so there is
 * zero per-event overhead in production.
 *
 * "Handled" means the handler invoked a cache operation — `setQueryData`,
 * `setQueriesData`, `invalidateQueries`, `removeQueries`, or `clear`. We count
 * the CALLS, not just observable data changes: an invalidation-based handler
 * (common for office/dashboard events) legitimately invalidates a key that was
 * never fetched in this test, which mutates nothing yet is correct handling.
 * A `getQueryCache().subscribe` counter would miss those and report a false
 * `cache-unchanged` drop. Spying the methods captures intent regardless of
 * whether a matching cache entry currently exists.
 *
 * WS dispatch is synchronous and single-threaded, and `handler(message)` runs
 * to completion before any other handler — so temporarily swapping the methods
 * on the shared client for the duration of one call is safe.
 *
 * Returns the original handler unchanged when the gate is off (production /
 * non-e2e builds), so there is zero per-event overhead in production.
 */
const SPIED_CACHE_METHODS = [
  "setQueryData",
  "setQueriesData",
  "invalidateQueries",
  "removeQueries",
  "clear",
] as const;

export function wrapBridgeHandler<T extends { payload?: unknown }>(
  queryClient: QueryClient,
  action: string,
  handler: (message: T) => void,
): (message: T) => void {
  if (!bridgeAuditEnabled) return handler;
  return (message: T) => {
    let cacheOps = 0;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const qc = queryClient as any;
    const originals: Record<string, unknown> = {};
    for (const name of SPIED_CACHE_METHODS) {
      originals[name] = qc[name];
      const orig = qc[name].bind(queryClient);
      qc[name] = (...args: unknown[]) => {
        cacheOps++;
        return orig(...args);
      };
    }
    try {
      handler(message);
    } finally {
      for (const name of SPIED_CACHE_METHODS) qc[name] = originals[name];
      pushAuditEntry({
        action,
        sessionId: readStringField(message.payload, "session_id"),
        taskId: readStringField(message.payload, "task_id"),
        cacheChanged: cacheOps > 0,
        mutationCount: cacheOps,
        timestamp: Date.now(),
      });
    }
  };
}

/**
 * Registers the WS → TanStack Query bridge.
 *
 * Single entry point called from QueryProvider on mount. Returns a
 * cleanup function that unregisters every per-domain handler.
 *
 * Each per-domain module mirrors lib/ws/handlers/<domain>.ts 1:1 but
 * writes into the TQ cache (queryClient.setQueryData) instead of the
 * Zustand store. Migration waves add their registrar to the list below.
 */
export function registerQueryBridge(
  ws: WebSocketClient,
  queryClient: QueryClient,
  options: QueryBridgeOptions,
): () => void {
  const cleanups: Array<() => void> = [
    registerFeaturesBridge(ws, queryClient),
    registerCommentsBridge(ws, queryClient),
    registerWorkspaceBridge(ws, queryClient),
    registerSettingsBridge(ws, queryClient),
    registerAutomationsBridge(ws, queryClient),
    registerIntegrationsBridge(ws, queryClient),
    registerGithubBridge(ws, queryClient),
    registerGitlabBridge(ws, queryClient),
    registerJiraBridge(ws, queryClient),
    registerLinearBridge(ws, queryClient),
    registerKanbanBridge(ws, queryClient),
    registerOfficeBridge(ws, queryClient, options.getActiveWorkspaceId),
    registerSessionBridge(ws, queryClient, {
      isEphemeralSurface: options.isEphemeralSurface,
    }),
    registerSessionStateBridge(ws, queryClient, {
      setEnvMapping: options.setEnvMapping,
    }),
    registerSessionRuntimeBridge(ws, queryClient, options.getEnvKey),
    registerSessionRuntimeStreamsBridge(ws),
  ];
  return () => {
    for (const fn of cleanups) fn();
  };
}
