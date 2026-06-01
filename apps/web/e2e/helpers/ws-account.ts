/**
 * Test-side WS event accounting helpers — Phase 1.
 *
 * The backend stamps every outbound WS envelope with a monotonic per-connection
 * `seq` and keeps a ring buffer of what it sent (apps/backend/internal/gateway/
 * websocket/ + the `/api/v1/e2e/ws-sent` endpoint). The FE WS client
 * (`apps/web/lib/ws/ws-account.ts`) keeps a parallel ring buffer of every seq
 * it received and exposes `window.__kandev_ws_account__()` / `_clear__()`.
 *
 * This helper sits in the middle. Tests use it to:
 *
 *  - `clearWsAccount(page)` before a test body to scope accounting to one test
 *  - `assertNoWsGaps(page, apiClient)` after the test body to verify every seq
 *    the backend sent for this connection was actually received by the FE.
 *    A gap means a WS event was dropped — a real bug, NOT a flake.
 *
 * Phase 1 covers per-connection gap detection. Per-session sequencing and a
 * backend-side ack channel come in Phase 2. See the design discussion in the
 * PR thread for the long-term plan and the trade-offs around what this catches
 * vs. what it doesn't.
 */

import type { Page } from "@playwright/test";
import {
  BRIDGE_SKIPPED_ACTIONS,
  BRIDGE_SKIPPED_PREFIXES,
  diffBridgeAudit,
  formatHandlerSideDrops,
  isBridgeSkippedAction,
  type BridgeAuditEntry,
  type HandlerSideDrop,
} from "../../lib/query/bridge-audit-diff";

// Re-export the bridge-audit pure-logic surface so e2e specs (and the
// fixture) can import everything from one place. The actual logic lives
// in `lib/query/bridge-audit-diff.ts` so it ships under vitest coverage
// without bumping into `apps/web/vitest.config.ts`'s `e2e/**` exclude.
export {
  BRIDGE_SKIPPED_ACTIONS,
  BRIDGE_SKIPPED_PREFIXES,
  diffBridgeAudit,
  formatHandlerSideDrops,
  isBridgeSkippedAction,
};
export type { BridgeAuditEntry, HandlerSideDrop };

/** Per-event metadata exposed in the snapshot. Mirrors the FE-side type
 *  in `apps/web/lib/ws/ws-account.ts`. */
export type WsAccountReceivedEvent = {
  seq: number;
  sessionSeq?: number;
  action: string;
  sessionId: string | null;
};

/** Per-session bucket inside a snapshot. Mirrors `WsAccountSessionSnapshot`. */
export type WsAccountSessionSnapshot = {
  processedSeqs: number[];
  gaps: number[];
  minSeq: number | null;
  maxSeq: number | null;
};

/** Mirror of `window.__kandev_ws_account__()` return shape (FE side). */
export type WsAccountSnapshot = {
  /** Most-recently-seen connection_id, or null if no seq events recorded. */
  connectionId: string | null;
  /** Every seq number the FE received and recorded, sorted ascending. */
  processedSeqs: number[];
  /** Missing seq numbers between min(processedSeqs) and max(processedSeqs). */
  gaps: number[];
  /** Highest seq in the buffer, null if empty. */
  maxSeq: number | null;
  /** Lowest seq still in the buffer (may be >1 if eviction happened). */
  minSeq: number | null;
  /** Per-event metadata aligned with `processedSeqs`. Added in Phase 2
   *  so the bridge-audit diff can match receipts to bridge mutations.
   *  Older bundles without the field expose `undefined`; the helper
   *  treats absence as "no per-event detail available". */
  receivedEvents?: WsAccountReceivedEvent[];
  /** Per-session buckets keyed by `session_id`. Added in Workstream 1 to
   *  surface cross-session misrouting that the connection bucket cannot see.
   *  Older bundles without the field expose `undefined`; the helper treats
   *  absence as "per-session diff not available, fall back to connection". */
  bySession?: Record<string, WsAccountSessionSnapshot>;
};

/** Mirror of `GET /api/v1/e2e/ws-sent` response shape (backend side).
 *  When `session_id` was passed as a query param, `events` contains only
 *  the entries stamped for that session and `max_seq` is the max session_seq
 *  (not connection seq) for that (connection, session) pair. */
export type WsSentResponse = {
  connection_id: string;
  events: Array<{
    seq: number;
    session_seq?: number;
    session_id?: string;
    type: string;
    action: string;
    sent_at: string;
  }>;
  max_seq: number;
};

/**
 * Read the FE's WS account snapshot from the live browser page.
 *
 * Returns `null` if the window hook isn't installed — usually means the FE
 * bundle wasn't built with accounting enabled (Phase 1 ships always-on, but a
 * stale bundle from before the rollout would have no hook). Callers MUST treat
 * `null` as "not enabled, skip the check" rather than as a test failure: the
 * suite has to keep passing during the migration window.
 */
export async function readWsAccount(page: Page): Promise<WsAccountSnapshot | null> {
  return page.evaluate(() => {
    type Hook = () => WsAccountSnapshot;
    const w = window as unknown as { __kandev_ws_account__?: Hook };
    return w.__kandev_ws_account__ ? w.__kandev_ws_account__() : null;
  });
}

/**
 * Reset the FE's WS account buffer. Called at the top of each test body so
 * accounting is scoped to one test — events from a previous test, the
 * fixture's `e2eReset`, etc. don't contaminate the gap check.
 *
 * No-op if the window hook isn't installed (see `readWsAccount` rationale).
 */
export async function clearWsAccount(page: Page): Promise<void> {
  await page.evaluate(() => {
    type Hook = () => void;
    const w = window as unknown as { __kandev_ws_account_clear__?: Hook };
    if (w.__kandev_ws_account_clear__) w.__kandev_ws_account_clear__();
  });
}

/**
 * Information about a missing event surfaced to the test author.
 *
 * `type` and `action` are filled from the backend's ring buffer so the
 * failure message names WHICH event was dropped, not just a bare seq number.
 * `session_id` and `session_seq` are present for session-routed drops
 * (sourced via the per-session diff path); absent for connection-bucket
 * drops.
 */
export type DroppedEvent = {
  seq: number;
  session_seq?: number;
  session_id?: string;
  type: string;
  action: string;
  sent_at: string;
};

/**
 * Compute the events the backend sent but the FE never processed. Returns
 * an empty array when there are no drops.
 *
 * Pure function — no I/O. Driver: pull from page + endpoint, pass into here.
 */
export function diffWsAccount(
  feSnapshot: WsAccountSnapshot,
  backendSent: WsSentResponse,
): DroppedEvent[] {
  const received = new Set(feSnapshot.processedSeqs);
  return backendSent.events
    .filter((event) => !received.has(event.seq))
    .map((event) => ({
      seq: event.seq,
      session_seq: event.session_seq,
      session_id: event.session_id,
      type: event.type,
      action: event.action,
      sent_at: event.sent_at,
    }));
}

/**
 * Compute the events the backend sent for a specific session but the FE never
 * processed under that session's per-session stream. Diffs by session_seq, not
 * by per-connection seq, so cross-session misrouting (event for A delivered to
 * B's handler with a valid connection seq) surfaces as a gap here even though
 * `diffWsAccount` wouldn't catch it.
 */
export function diffWsAccountForSession(
  feSnapshot: WsAccountSnapshot,
  backendSent: WsSentResponse,
  sessionId: string,
): DroppedEvent[] {
  const sessionBucket = feSnapshot.bySession?.[sessionId];
  const received = new Set(sessionBucket?.processedSeqs ?? []);
  return backendSent.events
    .filter((event) => {
      const ss = event.session_seq;
      return typeof ss === "number" && ss > 0 && !received.has(ss);
    })
    .map((event) => ({
      seq: event.seq,
      session_seq: event.session_seq,
      session_id: event.session_id ?? sessionId,
      type: event.type,
      action: event.action,
      sent_at: event.sent_at,
    }));
}

/** Minimal subset of ApiClient surface this helper depends on. Avoids a
 *  circular import on the full client and keeps the helper itself testable. */
export type WsSentFetcher = {
  getWsSent(connectionId: string, sinceSeq?: number, sessionId?: string): Promise<WsSentResponse>;
};

/**
 * Options for `computeWsDrops`.
 *
 * When `sessionId` is set, only the named session's per-session stream is
 * diffed. Without it, the diff covers the connection bucket AND every
 * per-session bucket the FE has observed — that's the broadest coverage and
 * the default the fixture uses.
 */
export type ComputeWsDropsOptions = {
  sessionId?: string;
};

/**
 * Read the FE buffer + the backend's `/ws-sent` log, diff them, and return
 * the dropped-event list. Returns an empty list when the FE hook isn't
 * installed (migration-safe — see `readWsAccount`).
 *
 * Doesn't throw on drops by itself — the caller decides whether a non-empty
 * list is a hard fail or a soft warning. The fixture wires it as a hard
 * fail (assertion in `afterEach`) when `KANDEV_E2E_WS_ASSERT=1`; otherwise
 * the diff is computed and logged but the test passes either way. That's the
 * Phase 1 rollout knob — flip the env on per-spec to enable enforcement, fix
 * what surfaces, then flip globally.
 *
 * With `opts.sessionId` set, only the named session is diffed. Without it,
 * the diff covers the connection bucket plus every session bucket the FE
 * has observed.
 */
export async function computeWsDrops(
  page: Page,
  apiClient: WsSentFetcher,
  opts: ComputeWsDropsOptions = {},
): Promise<DroppedEvent[]> {
  const feSnapshot = await readWsAccount(page);
  if (!feSnapshot || !feSnapshot.connectionId) {
    // Hook absent (old bundle) or no events seen yet — nothing to check.
    return [];
  }
  if (opts.sessionId !== undefined) {
    return computeWsDropsForOneSession(apiClient, feSnapshot, opts.sessionId);
  }
  return computeWsDropsAllStreams(apiClient, feSnapshot);
}

async function computeWsDropsForOneSession(
  apiClient: WsSentFetcher,
  feSnapshot: WsAccountSnapshot,
  sessionId: string,
): Promise<DroppedEvent[]> {
  let backendSent: WsSentResponse;
  try {
    backendSent = await apiClient.getWsSent(
      feSnapshot.connectionId as string,
      undefined,
      sessionId,
    );
  } catch {
    return [];
  }
  return diffWsAccountForSession(feSnapshot, backendSent, sessionId);
}

async function computeWsDropsAllStreams(
  apiClient: WsSentFetcher,
  feSnapshot: WsAccountSnapshot,
): Promise<DroppedEvent[]> {
  // Scope the backend query to the FE's current window. If a test called
  // clearWsAccount(), the FE ring is emptied but the connection (and its
  // backend seq counter) is preserved — fetching from seq 0 would return all
  // pre-clear events and diffWsAccount would flag every one as a false drop.
  // minSeq-1 excludes anything before the FE's earliest retained event.
  const sinceSeq = feSnapshot.minSeq !== null ? feSnapshot.minSeq - 1 : undefined;
  let backendSent: WsSentResponse;
  try {
    backendSent = await apiClient.getWsSent(feSnapshot.connectionId as string, sinceSeq);
  } catch {
    // Endpoint missing (old backend), 404 on unknown connection_id, or
    // network glitch. Don't synthesize a test failure from infra noise.
    return [];
  }
  const connectionDrops = diffWsAccount(feSnapshot, backendSent);
  const sessionDrops = await computeAllSessionBucketDrops(apiClient, feSnapshot);
  return dedupeDrops([...connectionDrops, ...sessionDrops]);
}

async function computeAllSessionBucketDrops(
  apiClient: WsSentFetcher,
  feSnapshot: WsAccountSnapshot,
): Promise<DroppedEvent[]> {
  const sessions = Object.keys(feSnapshot.bySession ?? {});
  const all: DroppedEvent[] = [];
  for (const sessionId of sessions) {
    try {
      const resp = await apiClient.getWsSent(
        feSnapshot.connectionId as string,
        undefined,
        sessionId,
      );
      all.push(...diffWsAccountForSession(feSnapshot, resp, sessionId));
    } catch {
      // Per-session endpoint failure (404 on unknown connection_id, etc.)
      // is treated as "no per-session info available" — don't synthesize
      // drops from infra noise.
    }
  }
  return all;
}

// Two diff passes (connection + per-session) can attribute the same
// envelope as a drop under both seqs. Dedupe by `connection seq` because
// every dropped envelope has a unique per-connection seq even when its
// session_seq is shared with another session.
function dedupeDrops(drops: DroppedEvent[]): DroppedEvent[] {
  const seen = new Set<number>();
  const out: DroppedEvent[] = [];
  for (const d of drops) {
    if (seen.has(d.seq)) continue;
    seen.add(d.seq);
    out.push(d);
  }
  return out;
}

/**
 * Read the bridge-audit ring buffer from the live browser page.
 *
 * Returns `null` when the hook isn't installed (either the bridge-audit
 * instrumentation hasn't shipped yet, or the bundle was built without
 * `NEXT_PUBLIC_KANDEV_E2E_MOCK=true`). Callers MUST treat `null` as
 * "not enabled, skip the check" — same migration-safety rationale as
 * `readWsAccount`.
 */
export async function readBridgeAudit(page: Page): Promise<BridgeAuditEntry[] | null> {
  return page.evaluate(() => {
    type Hook = () => BridgeAuditEntry[];
    const w = window as unknown as { __kandev_bridge_audit__?: Hook };
    return w.__kandev_bridge_audit__ ? w.__kandev_bridge_audit__() : null;
  });
}

/**
 * Reset the bridge-audit ring buffer. Mirrors `clearWsAccount`. No-op when
 * the hook isn't installed.
 */
export async function clearBridgeAudit(page: Page): Promise<void> {
  await page.evaluate(() => {
    type Hook = () => void;
    const w = window as unknown as { __kandev_bridge_audit_clear__?: Hook };
    if (w.__kandev_bridge_audit_clear__) w.__kandev_bridge_audit_clear__();
  });
}

/**
 * Read the FE's WS-account snapshot AND the bridge-audit ring, then diff
 * them to surface handler-side drops — events that the FE received but
 * which didn't cause a TanStack-Query cache mutation (and aren't on the
 * documented Zustand-only allowlist).
 *
 * Returns:
 *  - `[]` when the bridge audit hook isn't installed (migration-safe).
 *  - `[]` when the WS-account snapshot lacks per-event metadata
 *    (`receivedEvents`) — older bundle without the Phase 2 extension.
 *  - `HandlerSideDrop[]` otherwise.
 */
export async function computeBridgeAuditDrops(page: Page): Promise<HandlerSideDrop[]> {
  const feSnapshot = await readWsAccount(page);
  if (!feSnapshot || !feSnapshot.receivedEvents) return [];
  const bridgeAudit = await readBridgeAudit(page);
  if (!bridgeAudit) return [];
  return diffBridgeAudit(
    { receivedEvents: feSnapshot.receivedEvents },
    bridgeAudit,
    BRIDGE_SKIPPED_ACTIONS,
    BRIDGE_SKIPPED_PREFIXES,
  );
}

/**
 * Format a dropped-event list for a test failure message. Groups output by
 * session when drops span multiple sessions so cross-session bugs are easy
 * to spot at a glance; falls back to the flat list when only the connection
 * bucket has drops or only one session is involved.
 */
export function formatDroppedEvents(drops: DroppedEvent[]): string {
  if (drops.length === 0) return "";
  const sessions = new Set<string>();
  for (const d of drops) {
    if (d.session_id) sessions.add(d.session_id);
  }
  const hasConnectionOnly = drops.some((d) => !d.session_id);
  // Single-session OR pure-connection drops → flat list (Phase 1 behavior).
  if (sessions.size <= 1 && !(sessions.size === 1 && hasConnectionOnly)) {
    return formatDroppedEventsFlat(drops);
  }
  return formatDroppedEventsGrouped(drops);
}

function formatDroppedEventsFlat(drops: DroppedEvent[]): string {
  const lines = drops.slice(0, 20).map(formatOneDrop);
  const more = drops.length > 20 ? `  ... and ${drops.length - 20} more` : "";
  return `${drops.length} WS event(s) sent by backend but never processed by FE:\n${lines.join("\n")}${more}`;
}

function formatDroppedEventsGrouped(drops: DroppedEvent[]): string {
  const groups = new Map<string, DroppedEvent[]>();
  for (const d of drops) {
    const key = d.session_id ?? "(connection)";
    const arr = groups.get(key) ?? [];
    arr.push(d);
    groups.set(key, arr);
  }
  const sections: string[] = [];
  for (const [sid, list] of groups) {
    const head = `  ${sid}: ${list.length} drop(s)`;
    const body = list.slice(0, 10).map((d) => `    ${formatOneDropInner(d)}`);
    const more = list.length > 10 ? `    ... and ${list.length - 10} more` : "";
    sections.push([head, ...body, more].filter(Boolean).join("\n"));
  }
  return `${drops.length} WS event(s) sent by backend but never processed by FE (grouped by session):\n${sections.join("\n")}`;
}

function formatOneDrop(d: DroppedEvent): string {
  return `  ${formatOneDropInner(d)}`;
}

function formatOneDropInner(d: DroppedEvent): string {
  const ssPart =
    typeof d.session_seq === "number" && d.session_seq > 0 ? ` session_seq=${d.session_seq}` : "";
  const sidPart = d.session_id ? ` session=${d.session_id}` : "";
  return `seq=${d.seq}${ssPart}${sidPart} ${d.type}/${d.action} sent_at=${d.sent_at}`;
}
