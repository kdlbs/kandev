"use client";

import { useEffect, useRef, useState } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { subscribeRunEvents } from "@/lib/ws/handlers/run";
import type { RunDetail, RunEvent } from "@/lib/api/domains/office-extended-api";
import { getRunDetail } from "@/lib/api/domains/office-runs-api";

type Status = RunDetail["status"];

const TERMINAL_EVENT_STATUS: Record<string, Status | undefined> = {
  complete: "finished",
  finished: "finished",
  error: "failed",
  failed: "failed",
};

const TERMINAL_STATUSES: ReadonlySet<Status> = new Set<Status>(["finished", "failed", "cancelled"]);

type Options = {
  agentId?: string;
  initialOutputSummary?: string;
};

/**
 * Returns the live-merged events list and an observed status that
 * follows terminal events emitted on the bus. While `initialStatus`
 * is `claimed` (the running state), the hook subscribes to
 * `run.subscribe` over the WS and appends `run.event.appended`
 * payloads to the events list. Terminal events (`complete` /
 * `finished` / `error` / `failed`) update the local status so the
 * header reflects the new state without a snapshot refetch.
 *
 * Idempotency: dedupes by event seq (Wave 1 enforces monotonic seq
 * per run id) so duplicate notifications from a reconnect or a race
 * with the snapshot fetch don't double-render rows.
 *
 * Cleanup: unsubscribes on unmount AND when the run reaches a
 * terminal status — there's no point holding the bus subscription
 * open for runs that can no longer emit events.
 */
export function useRunLiveSync(
  runId: string,
  initialEvents: RunEvent[],
  initialStatus: Status,
  options: Options = {},
): { events: RunEvent[]; status: Status; outputSummary: string } {
  const agentId = options.agentId ?? "";
  const initialOutputSummary = options.initialOutputSummary ?? "";
  const [events, setEvents] = useState<RunEvent[]>(initialEvents);
  const [status, setStatus] = useState<Status>(initialStatus);
  const [outputSummary, setOutputSummary] = useState(initialOutputSummary);
  // Live-event dedup only: seqs of events already appended over the WS (plus
  // the seqs of the mount-time snapshot, so a reconnect replay of a
  // snapshot-covered event cannot double-insert). Never reset to a snapshot:
  // doing so erases live events on a fresh-but-unchanged rerender.
  const seenSeqsRef = useRef<Set<number>>(new Set(initialEvents.map((e) => e.seq)));
  // Last incoming snapshot, tracked separately from the live dedup set. The
  // sync decision compares against this, so live-added seqs can never make
  // an unchanged snapshot look like a content change.
  const lastSnapshotRef = useRef<{ runId: string; seqs: Set<number> }>({
    runId,
    seqs: new Set(initialEvents.map((e) => e.seq)),
  });
  const outputSnapshotRef = useRef({ runId, value: initialOutputSummary });
  const refreshedTerminalRunRef = useRef<string | null>(null);

  useEffect(() => {
    const nextSeqs = new Set(initialEvents.map((e) => e.seq));
    const prev = lastSnapshotRef.current;
    if (
      prev.runId === runId &&
      prev.seqs.size === nextSeqs.size &&
      [...nextSeqs].every((seq) => prev.seqs.has(seq))
    ) {
      // Same run, same event set as the last incoming snapshot. Skip the state
      // write so an unstable `initialEvents` reference (a fresh array literal
      // on every render) cannot feed an endless render->effect cycle, and so a
      // stale snapshot cannot clobber events appended live over the WS.
      return;
    }
    lastSnapshotRef.current = { runId, seqs: nextSeqs };
    if (prev.runId === runId) {
      // Fresh snapshot for the same run: merge it with live-added events and
      // fold its seqs into the dedup set so reconnect replays cannot duplicate.
      for (const seq of nextSeqs) seenSeqsRef.current.add(seq);
      setEvents((current) => mergeRunEvents(current, initialEvents));
      return;
    } else {
      // Different run: restart dedup from the new snapshot; seqs are per-run.
      seenSeqsRef.current = new Set(nextSeqs);
    }
    setEvents(initialEvents);
  }, [initialEvents, runId]);

  useEffect(() => {
    setStatus(initialStatus);
  }, [initialStatus]);

  useEffect(() => {
    const previous = outputSnapshotRef.current;
    if (previous.runId === runId && previous.value === initialOutputSummary) return;
    outputSnapshotRef.current = { runId, value: initialOutputSummary };
    setOutputSummary(initialOutputSummary);
    if (previous.runId !== runId) {
      refreshedTerminalRunRef.current = null;
    }
  }, [initialOutputSummary, runId]);

  useEffect(() => {
    if (!agentId || !TERMINAL_STATUSES.has(status)) return;
    if (refreshedTerminalRunRef.current === runId) return;
    refreshedTerminalRunRef.current = runId;
    let cancelled = false;
    void getRunDetail(agentId, runId)
      .then((run) => {
        if (!cancelled) setOutputSummary(run.output_summary ?? "");
      })
      .catch(() => {
        // The terminal status and event stream remain useful when the detail
        // refresh fails. A later page reload can retry the snapshot.
      });
    return () => {
      cancelled = true;
    };
  }, [agentId, runId, status]);

  useEffect(() => {
    if (status !== "claimed") return;
    if (TERMINAL_STATUSES.has(status)) return;

    const client = getWebSocketClient();
    if (!client) return;

    const unsubscribeWs = client.subscribeRun(runId);
    const unsubscribeListener = subscribeRunEvents(runId, (payload) => {
      if (payload.run_id !== runId) return;
      const evt = payload.event;
      if (seenSeqsRef.current.has(evt.seq)) return;
      seenSeqsRef.current.add(evt.seq);
      setEvents((prev) => mergeRunEvent(prev, evt));
      const next = TERMINAL_EVENT_STATUS[evt.event_type];
      if (next) setStatus(next);
    });

    return () => {
      unsubscribeListener();
      unsubscribeWs();
    };
  }, [runId, status]);

  return { events, status, outputSummary };
}

// mergeRunEvent inserts evt into prev keeping seq-ascending order.
// Backend assigns monotonically increasing seq, so the common case
// is an append; we still binary-walk to handle out-of-order delivery
// from a reconnect-replay edge case.
function mergeRunEvent(prev: RunEvent[], evt: RunEvent): RunEvent[] {
  if (prev.length === 0 || evt.seq > prev[prev.length - 1].seq) {
    return [...prev, evt];
  }
  const next = [...prev];
  let lo = 0;
  let hi = next.length;
  while (lo < hi) {
    const mid = (lo + hi) >>> 1;
    if (next[mid].seq < evt.seq) lo = mid + 1;
    else hi = mid;
  }
  next.splice(lo, 0, evt);
  return next;
}

function mergeRunEvents(current: RunEvent[], snapshot: RunEvent[]): RunEvent[] {
  if (current.length === 0) return snapshot;
  if (snapshot.length === 0) return current;

  const bySeq = new Map<number, RunEvent>();
  for (const event of current) bySeq.set(event.seq, event);
  for (const event of snapshot) bySeq.set(event.seq, event);
  return [...bySeq.values()].sort((a, b) => a.seq - b.seq);
}
