import type { QueryClient } from "@tanstack/react-query";

const AUDIT_BUFFER_SIZE = 5000;

export type BridgeAuditStatus = "handled" | "allowlisted";

export interface BridgeAuditEntry {
  action: string;
  cacheChanged: boolean;
  mutationCount: number;
  reason?: string;
  sessionId: string | null;
  status: BridgeAuditStatus;
  taskId: string | null;
  timestamp: number;
  type: string | null;
}

type BridgeAuditWindow = Window & {
  __KANDEV_E2E_EXPOSE_STORE__?: boolean;
  __kandev_bridge_audit__?: () => BridgeAuditEntry[];
  __kandev_bridge_audit_clear__?: () => void;
};

type BridgeEnvelope = {
  action?: string;
  payload?: unknown;
  type?: string;
};

type CacheMutationMethod =
  | "clear"
  | "invalidateQueries"
  | "removeQueries"
  | "setQueriesData"
  | "setQueryData";

const SPIED_CACHE_METHODS: readonly CacheMutationMethod[] = [
  "setQueryData",
  "setQueriesData",
  "invalidateQueries",
  "removeQueries",
  "clear",
];

// Control-plane responses are resolved through WebSocketClient.pendingRequests,
// not query cache state. Keeping them explicit here lets the E2E audit explain
// why a parsed envelope intentionally has no bridge cache mutation.
const CONTROL_PLANE_REASON = "control-plane request/response handled outside the query cache";

// These notifications still have Zustand-backed readers during the migration.
// Remove entries as each domain wave moves both bridge writers and UI readers to
// TanStack Query.
const ZUSTAND_MIGRATION_REASON = "temporarily Zustand-backed until its domain query wave migrates";

// Client-only notifications produce toasts, dialogs, browser notifications, or
// imperative refreshes rather than durable server-state cache entries.
const CLIENT_EFFECT_REASON = "client-only effect with no durable query cache entry";

// High-volume streams stay outside QueryClient to avoid per-chunk observer churn.
const STREAM_REASON = "high-volume stream tracked outside TanStack Query";

export const BRIDGE_SKIPPED_ACTIONS = {
  "agent.cancel": CONTROL_PLANE_REASON,
  "agent.logs": CONTROL_PLANE_REASON,
  "agent.prompt": CONTROL_PLANE_REASON,
  "agent.resize": CONTROL_PLANE_REASON,
  "agent.status": CONTROL_PLANE_REASON,
  "agent.stdin": CONTROL_PLANE_REASON,
  "agent.stop": CONTROL_PLANE_REASON,
  "message.queue.add": CONTROL_PLANE_REASON,
  "message.queue.append": CONTROL_PLANE_REASON,
  "message.queue.cancel": CONTROL_PLANE_REASON,
  "message.queue.get": CONTROL_PLANE_REASON,
  "message.queue.remove": CONTROL_PLANE_REASON,
  "message.queue.update": CONTROL_PLANE_REASON,
  "permission.respond": CONTROL_PLANE_REASON,
  "run.subscribe": CONTROL_PLANE_REASON,
  "run.unsubscribe": CONTROL_PLANE_REASON,
  "session.commit_diff": CONTROL_PLANE_REASON,
  "session.cumulative_diff": CONTROL_PLANE_REASON,
  "session.delete": CONTROL_PLANE_REASON,
  "session.ensure": CONTROL_PLANE_REASON,
  "session.file_review.get": CONTROL_PLANE_REASON,
  "session.file_review.reset": CONTROL_PLANE_REASON,
  "session.file_review.update": CONTROL_PLANE_REASON,
  "session.focus": CONTROL_PLANE_REASON,
  "session.git.commits": CONTROL_PLANE_REASON,
  "session.git.snapshots": CONTROL_PLANE_REASON,
  "session.launch": CONTROL_PLANE_REASON,
  "session.recover": CONTROL_PLANE_REASON,
  "session.reset_context": CONTROL_PLANE_REASON,
  "session.set_mode": CONTROL_PLANE_REASON,
  "session.set_plan_mode": CONTROL_PLANE_REASON,
  "session.set_primary": CONTROL_PLANE_REASON,
  "session.shell.status": CONTROL_PLANE_REASON,
  "session.stop": CONTROL_PLANE_REASON,
  "session.subscribe": CONTROL_PLANE_REASON,
  "session.unfocus": CONTROL_PLANE_REASON,
  "session.unsubscribe": CONTROL_PLANE_REASON,
  "shell.input": CONTROL_PLANE_REASON,
  "shell.subscribe": CONTROL_PLANE_REASON,
  "system.metrics.subscribe": CONTROL_PLANE_REASON,
  "system.metrics.unsubscribe": CONTROL_PLANE_REASON,
  "task.plan.create": CONTROL_PLANE_REASON,
  "task.plan.delete": CONTROL_PLANE_REASON,
  "task.plan.get": CONTROL_PLANE_REASON,
  "task.plan.revision.get": CONTROL_PLANE_REASON,
  "task.plan.revisions.list": CONTROL_PLANE_REASON,
  "task.plan.revert": CONTROL_PLANE_REASON,
  "task.plan.update": CONTROL_PLANE_REASON,
  "task.session": CONTROL_PLANE_REASON,
  "task.session.list": CONTROL_PLANE_REASON,
  "task.session.status": CONTROL_PLANE_REASON,
  "task.subscribe": CONTROL_PLANE_REASON,
  "task.unsubscribe": CONTROL_PLANE_REASON,
  "user.subscribe": CONTROL_PLANE_REASON,
  "user.unsubscribe": CONTROL_PLANE_REASON,
  "vscode.openFile": CONTROL_PLANE_REASON,
  "vscode.start": CONTROL_PLANE_REASON,
  "vscode.status": CONTROL_PLANE_REASON,
  "vscode.stop": CONTROL_PLANE_REASON,

  "diff.update": ZUSTAND_MIGRATION_REASON,
  "run.event.appended": ZUSTAND_MIGRATION_REASON,

  "input.requested": CLIENT_EFFECT_REASON,
  "permission.requested": CLIENT_EFFECT_REASON,
  "session.waiting_for_input": CLIENT_EFFECT_REASON,
  "session.workspace.file.changes": CLIENT_EFFECT_REASON,
  "system.error": CLIENT_EFFECT_REASON,

  "session.process.output": STREAM_REASON,
  "session.shell.output": STREAM_REASON,
  "terminal.output": STREAM_REASON,
} as const satisfies Readonly<Record<string, string>>;

// Prefix skips cover request/response families that are not emitted as typed
// BackendMessageType notifications. They are still parsed WS envelopes, so the
// audit needs a rationale instead of reporting a missing bridge.
export const BRIDGE_SKIPPED_PREFIXES = ["agentctl_", "user_shell."] as const;

const BRIDGE_SKIPPED_PREFIX_REASONS: Readonly<Record<string, string>> = {
  agentctl_: "agentctl container channels are consumed through agentctl HTTP surfaces",
  "user_shell.": "user shell operations are request/response control-plane envelopes",
};

const auditBuffer = new Map<number, BridgeAuditEntry>();
let auditSeq = 0;

export function isBridgeSkippedAction(action: string): boolean {
  return getBridgeSkipReason(action) !== null;
}

export function getBridgeSkipReason(action: string): string | null {
  if (Object.prototype.hasOwnProperty.call(BRIDGE_SKIPPED_ACTIONS, action)) {
    return BRIDGE_SKIPPED_ACTIONS[action as keyof typeof BRIDGE_SKIPPED_ACTIONS];
  }
  const prefix = BRIDGE_SKIPPED_PREFIXES.find((item) => action.startsWith(item));
  return prefix ? BRIDGE_SKIPPED_PREFIX_REASONS[prefix] : null;
}

export function clearBridgeAuditRows(): void {
  auditBuffer.clear();
}

export function getBridgeAuditRows(): BridgeAuditEntry[] {
  return Array.from(auditBuffer.values());
}

export function installBridgeAuditAccessors(): void {
  const win = getAuditWindow();
  if (!win || !isBridgeAuditEnabled()) return;
  win.__kandev_bridge_audit__ = getBridgeAuditRows;
  win.__kandev_bridge_audit_clear__ = clearBridgeAuditRows;
}

export function isBridgeAuditEnabled(): boolean {
  return getAuditWindow()?.__KANDEV_E2E_EXPOSE_STORE__ === true;
}

export function recordBridgeAllowlistedEvent(message: BridgeEnvelope): void {
  const action = message.action;
  if (!action || !isBridgeAuditEnabled()) return;
  const reason = getBridgeSkipReason(action);
  if (!reason) return;
  pushAuditEntry({
    action,
    cacheChanged: false,
    mutationCount: 0,
    reason,
    sessionId: readStringField(message.payload, "session_id"),
    status: "allowlisted",
    taskId: readStringField(message.payload, "task_id"),
    timestamp: Date.now(),
    type: message.type ?? null,
  });
}

export function wrapBridgeHandler<T extends BridgeEnvelope>(
  queryClient: QueryClient,
  action: string,
  handler: (message: T) => void,
): (message: T) => void {
  if (!isBridgeAuditEnabled()) return handler;
  installBridgeAuditAccessors();
  return (message: T) => {
    let mutationCount = 0;
    const queryClientMethods = queryClient as unknown as Record<
      CacheMutationMethod,
      (...args: unknown[]) => unknown
    >;
    const originals = new Map<CacheMutationMethod, (...args: unknown[]) => unknown>();

    for (const method of SPIED_CACHE_METHODS) {
      const original = queryClientMethods[method];
      originals.set(method, original);
      queryClientMethods[method] = (...args: unknown[]) => {
        mutationCount++;
        return original.apply(queryClient, args);
      };
    }

    try {
      handler(message);
    } finally {
      for (const [method, original] of originals) {
        queryClientMethods[method] = original;
      }
      pushAuditEntry({
        action,
        cacheChanged: mutationCount > 0,
        mutationCount,
        sessionId: readStringField(message.payload, "session_id"),
        status: "handled",
        taskId: readStringField(message.payload, "task_id"),
        timestamp: Date.now(),
        type: message.type ?? null,
      });
    }
  };
}

function pushAuditEntry(entry: BridgeAuditEntry): void {
  if (!isBridgeAuditEnabled()) return;
  installBridgeAuditAccessors();
  auditBuffer.set(auditSeq++, entry);
  while (auditBuffer.size > AUDIT_BUFFER_SIZE) {
    const oldest = auditBuffer.keys().next().value;
    if (oldest === undefined) break;
    auditBuffer.delete(oldest);
  }
}

function readStringField(payload: unknown, key: string): string | null {
  if (!payload || typeof payload !== "object") return null;
  const value = (payload as Record<string, unknown>)[key];
  return typeof value === "string" && value !== "" ? value : null;
}

function getAuditWindow(): BridgeAuditWindow | null {
  if (typeof window === "undefined") return null;
  return window as BridgeAuditWindow;
}
