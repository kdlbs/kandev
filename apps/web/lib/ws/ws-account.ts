/**
 * WS event accounting — Phase 1 + Workstream 1 (per-session sequencing).
 *
 * The backend stamps every outbound envelope with a monotonic per-connection
 * `seq` and a `connection_id`. Session-routed envelopes also carry a per-
 * session `session_seq` and (implicitly via the payload) a `session_id`.
 * This module records what the FE actually received so E2E tests can detect
 * dropped events by comparing the backend's "sent" set to the FE's
 * "processed" set, both per-connection AND per-session.
 *
 * Public surface:
 *  - `detectGaps(processed)` — pure helper, exported for unit tests and used
 *    by the snapshot to compute missing seqs.
 *  - `WsAccount` — owns a connection-wide ring buffer plus a Map of
 *    per-session ring buffers. Each inbound event is appended to the
 *    connection bucket; if it carries a `sessionId`, it is also appended to
 *    the matching per-session bucket. The connection bucket stays as the
 *    safety net that catches drops of non-routed events; per-session is
 *    additive and catches misrouting that the connection bucket cannot see.
 *
 * Activation is gated by the caller (see `client.ts` —
 * `NEXT_PUBLIC_KANDEV_E2E_MOCK === "true"`). In production this module is
 * imported but never instantiated/invoked from the dispatch hot path.
 */

export const WS_ACCOUNT_MAX_ENTRIES = 5000;

export interface WsAccountEntry {
  seq: number;
  /**
   * Per-session monotonic seq stamped by the backend for session-routed
   * events. `0` (or absent) for connection-wide notifications. Used by
   * per-session gap detection to surface cross-session misrouting that
   * per-connection seq alone cannot see.
   */
  sessionSeq?: number;
  type: string;
  action: string;
  /**
   * Session ID extracted from the envelope's payload (`payload.session_id`)
   * when present. `null` for non-session-scoped events (handshake, global
   * notifications, etc.). Carried per-entry so the e2e bridge-audit diff
   * can match received events against bridge cache-mutation entries by
   * `action + sessionId` — see `apps/web/lib/query/bridge-audit-diff.ts`.
   */
  sessionId: string | null;
  receivedAt: number;
}

/**
 * Per-event metadata exposed in the snapshot so e2e helpers can attribute
 * each received seq to an action + session without having to re-fetch the
 * envelope. Phase 1 only needed seqs; Phase 2's bridge-audit pass needs
 * the action and session to diff against `window.__kandev_bridge_audit__`.
 */
export interface WsAccountReceivedEvent {
  seq: number;
  sessionSeq?: number;
  action: string;
  sessionId: string | null;
  /**
   * Envelope `type` (`"notification"` | `"response"` | `"error"` | `"request"`).
   * Carried so the bridge-audit diff can skip responses to FE-initiated
   * requests — those are dispatched through `pendingRequests`, never through
   * bridge handlers, so they'd otherwise show up as `no-bridge-entry` drops.
   */
  type: string;
}

/**
 * Per-session view inside the snapshot. Keyed by `session_id`. Each bucket's
 * `processedSeqs` is the list of per-session `session_seq` values observed,
 * sorted ascending — same gap-detection logic as the connection bucket but
 * over a different seq dimension.
 */
export interface WsAccountSessionSnapshot {
  processedSeqs: number[];
  gaps: number[];
  maxSeq: number | null;
  minSeq: number | null;
}

export interface WsAccountSnapshot {
  connectionId: string | null;
  /** Connection-bucket processed per-connection seqs, sorted ascending. */
  processedSeqs: number[];
  /** Connection-bucket gaps. */
  gaps: number[];
  /** Connection-bucket max seq. */
  maxSeq: number | null;
  /** Connection-bucket min seq. */
  minSeq: number | null;
  /**
   * Per-event metadata for every seq in `processedSeqs`, in the same sort
   * order. Same length as `processedSeqs`. Added in Phase 2 for the
   * bridge-audit cache-mutation diff; tests that only look at gaps can
   * keep ignoring it.
   */
  receivedEvents: WsAccountReceivedEvent[];
  /**
   * Per-session buckets, indexed by `session_id`. Each bucket's
   * `processedSeqs` is the per-session `session_seq` stream — independent of
   * the connection stream, so cross-session misrouting is detectable as a
   * gap on a single session even when the connection stream is contiguous.
   */
  bySession: Record<string, WsAccountSessionSnapshot>;
}

/**
 * Returns the list of missing integers between min(processed) and
 * max(processed) (inclusive). Input MUST be sorted ascending.
 */
export function detectGaps(processed: number[]): number[] {
  if (processed.length < 2) return [];
  const min = processed[0];
  const max = processed[processed.length - 1];
  const present = new Set(processed);
  const gaps: number[] = [];
  for (let i = min + 1; i < max; i++) {
    if (!present.has(i)) gaps.push(i);
  }
  return gaps;
}

/**
 * Internal per-bucket store. The same shape used for both the connection
 * bucket (keyed by per-connection `seq`) and each per-session bucket (keyed
 * by per-session `session_seq`). Map insertion order is FIFO, so iteration
 * order = recording order = oldest-first eviction order.
 */
class WsAccountBucket {
  // The seq used as the map key differs by bucket: the connection bucket
  // keys by per-connection `seq`; a per-session bucket keys by `session_seq`.
  // Both are monotonic per their respective stream.
  private entries = new Map<number, WsAccountEntry>();

  constructor(private readonly maxEntries: number) {}

  record(key: number, entry: WsAccountEntry): void {
    if (this.entries.has(key)) {
      // Refresh insertion order so eviction reflects last-seen.
      this.entries.delete(key);
    }
    this.entries.set(key, entry);
    while (this.entries.size > this.maxEntries) {
      const oldestKey = this.entries.keys().next().value;
      if (oldestKey === undefined) break;
      this.entries.delete(oldestKey);
    }
  }

  clear(): void {
    this.entries.clear();
  }

  size(): number {
    return this.entries.size;
  }

  snapshot(): {
    processedSeqs: number[];
    receivedEvents: WsAccountReceivedEvent[];
    minSeq: number | null;
    maxSeq: number | null;
    gaps: number[];
  } {
    const processedSeqs = Array.from(this.entries.keys()).sort((a, b) => a - b);
    const minSeq = processedSeqs.length > 0 ? processedSeqs[0] : null;
    const maxSeq = processedSeqs.length > 0 ? processedSeqs[processedSeqs.length - 1] : null;
    const receivedEvents: WsAccountReceivedEvent[] = processedSeqs.map((key) => {
      const e = this.entries.get(key);
      return {
        seq: e?.seq ?? key,
        sessionSeq: e?.sessionSeq,
        action: e?.action ?? "",
        sessionId: e?.sessionId ?? null,
        type: e?.type ?? "",
      };
    });
    return {
      processedSeqs,
      receivedEvents,
      minSeq,
      maxSeq,
      gaps: detectGaps(processedSeqs),
    };
  }
}

export class WsAccount {
  private connectionId: string | null = null;
  private connection: WsAccountBucket;
  private bySession = new Map<string, WsAccountBucket>();
  private readonly maxEntries: number;

  constructor(maxEntries: number = WS_ACCOUNT_MAX_ENTRIES) {
    this.maxEntries = maxEntries;
    this.connection = new WsAccountBucket(maxEntries);
  }

  /**
   * Record an inbound envelope. The connection bucket always receives the
   * entry keyed by `entry.seq` (per-connection); if `entry.sessionId` is
   * non-null AND a non-zero `entry.sessionSeq` was stamped by the backend,
   * the entry is also recorded into the matching per-session bucket keyed
   * by `entry.sessionSeq`.
   *
   * If `connectionId` differs from the previously seen one, both the
   * connection bucket and ALL per-session buckets are cleared because the
   * backend resets its seq counters on each new connection.
   */
  record(entry: WsAccountEntry, connectionId?: string): void {
    if (connectionId && connectionId !== this.connectionId) {
      this.connection.clear();
      this.bySession.clear();
      this.connectionId = connectionId;
    }
    this.connection.record(entry.seq, entry);
    if (entry.sessionId && entry.sessionSeq && entry.sessionSeq > 0) {
      let bucket = this.bySession.get(entry.sessionId);
      if (!bucket) {
        bucket = new WsAccountBucket(this.maxEntries);
        this.bySession.set(entry.sessionId, bucket);
      }
      bucket.record(entry.sessionSeq, entry);
    }
  }

  /**
   * Returns a sorted snapshot suitable for E2E inspection. Sorting on
   * read keeps the write path O(1); the buffer is bounded so the cost
   * is paid only when a test asks for it.
   */
  snapshot(): WsAccountSnapshot {
    const conn = this.connection.snapshot();
    const bySession: Record<string, WsAccountSessionSnapshot> = {};
    for (const [sessionId, bucket] of this.bySession) {
      const s = bucket.snapshot();
      bySession[sessionId] = {
        processedSeqs: s.processedSeqs,
        gaps: s.gaps,
        minSeq: s.minSeq,
        maxSeq: s.maxSeq,
      };
    }
    return {
      connectionId: this.connectionId,
      processedSeqs: conn.processedSeqs,
      gaps: conn.gaps,
      maxSeq: conn.maxSeq,
      minSeq: conn.minSeq,
      receivedEvents: conn.receivedEvents,
      bySession,
    };
  }

  /**
   * Drop all entries — connection bucket and every per-session bucket.
   * Preserves `connectionId` so subsequent envelopes on the same connection
   * are still merged into the same logical bucket.
   */
  clear(): void {
    this.connection.clear();
    this.bySession.clear();
  }

  /**
   * Drop a single per-session bucket. Used by tests and by callers that
   * observe a session shutdown (e.g. task closed, navigation away) and want
   * to release the buffer without affecting the connection-wide accounting.
   * No-op if the session was never recorded.
   */
  clearSession(sessionId: string): void {
    this.bySession.delete(sessionId);
  }

  /** Test helper — connection bucket size. */
  size(): number {
    return this.connection.size();
  }

  /** Test helper — number of live per-session buckets. */
  sessionBucketCount(): number {
    return this.bySession.size;
  }
}
