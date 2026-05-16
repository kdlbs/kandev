import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { RunEventAppendedPayload } from "@/lib/types/backend";

/**
 * Listeners registered for `run.event.appended` notifications, keyed
 * by run id. The run detail page registers a listener via
 * `subscribeRunEvents` while the run is live; the WS handler below
 * dispatches incoming payloads to all matching listeners.
 *
 * We deliberately keep this out of the zustand store: only the
 * currently-mounted run detail view cares about the event stream, and
 * tying it to component-local state via callbacks avoids spreading a
 * transient per-page concern into global state. The hook decides what
 * to do with the event (append, dedupe, terminal-status update).
 */
type RunEventListener = (payload: RunEventAppendedPayload) => void;

const listeners = new Map<string, Set<RunEventListener>>();

export function subscribeRunEvents(runId: string, listener: RunEventListener): () => void {
  let bucket = listeners.get(runId);
  if (!bucket) {
    bucket = new Set();
    listeners.set(runId, bucket);
  }
  bucket.add(listener);
  return () => {
    const set = listeners.get(runId);
    if (!set) return;
    set.delete(listener);
    if (set.size === 0) listeners.delete(runId);
  };
}

/**
 * Registers WS handlers for run-detail live updates. The handler
 * looks up listeners by `run_id` so notifications for runs the user
 * isn't viewing are silently dropped — the gateway already routes
 * per-run, but we filter again here defensively in case multiple
 * detail pages mount in different tabs.
 */
export function registerRunHandlers(): WsHandlers {
  return {
    "run.event.appended": (message) => {
      const payload = message.payload;
      if (!payload?.run_id) return;
      const bucket = listeners.get(payload.run_id);
      if (!bucket) return;
      bucket.forEach((listener) => listener(payload));
    },
  };
}
