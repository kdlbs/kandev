"use client";

import { useEffect, useRef, useState } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { subscribeRunEvents } from "@/lib/ws/handlers/run";
import type { RunDetail, RunEvent } from "@/lib/api/domains/office-extended-api";

type Status = RunDetail["status"];

const TERMINAL_EVENT_STATUS: Record<string, Status | undefined> = {
  complete: "finished",
  finished: "finished",
  error: "failed",
  failed: "failed",
};

const TERMINAL_STATUSES: ReadonlySet<Status> = new Set<Status>(["finished", "failed", "cancelled"]);

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
): { events: RunEvent[]; status: Status } {
  const [events, setEvents] = useState<RunEvent[]>(initialEvents);
  const [status, setStatus] = useState<Status>(initialStatus);
  const seenSeqsRef = useRef<Set<number>>(new Set(initialEvents.map((e) => e.seq)));

  useEffect(() => {
    seenSeqsRef.current = new Set(initialEvents.map((e) => e.seq));
    setEvents(initialEvents);
  }, [initialEvents]);

  useEffect(() => {
    setStatus(initialStatus);
  }, [initialStatus]);

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

  return { events, status };
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
