"use client";

import { useCallback, useState } from "react";
import { startProcess, stopProcess } from "@/lib/api";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { isProcessLive, toProcessStatusEntry } from "@/lib/state/process-status";
import type { ProcessStatusEntry } from "@/lib/state/slices";

export type DevServerPreview = {
  /** Process id of the session's dev process, when one is known. */
  devProcessId: string | undefined;
  /** True while the dev process is starting or running. */
  isRunning: boolean;
  /** True while a start or stop request is in flight. */
  isPending: boolean;
  start: () => Promise<void>;
  stop: () => Promise<void>;
};

/**
 * In-flight start requests, keyed by session.
 *
 * The same control renders in two dockview headers, each with its own local
 * pending flag, so two clicks inside one round trip would both pass their own
 * guard. The backend deduplicates by listing the session's processes, which is
 * not atomic with the start, so a second request can win that race and leave a
 * second dev process fighting for the port with no control pointing at it.
 * Sharing the promise at module scope makes every control in the tab issue one
 * request and await the same result.
 */
const startsInFlight = new Map<string, Promise<void>>();

/**
 * Apply a start response without clobbering a newer WebSocket frame.
 *
 * A dev script that exits immediately can have its terminal
 * `session.process.status` frame land before the start POST resolves. Writing
 * the response unconditionally would then replace `exited` with the response's
 * older `running`, freezing the control on Stop with nothing left to stop and
 * no later frame to correct it.
 */
function applyStartedProcess(
  existing: ProcessStatusEntry | undefined,
  started: ProcessStatusEntry,
): boolean {
  if (!existing || existing.processId !== started.processId) return true;
  if (!existing.updatedAt || !started.updatedAt) return true;
  return existing.updatedAt <= started.updatedAt;
}

/**
 * Start/stop control for a session's dev-script process.
 *
 * The dev process is owned by the backend, so `isRunning` is derived from the
 * store (seeded by the start response, kept current by `session.process.status`
 * WebSocket frames) rather than from local state. `isPending` only covers the
 * in-flight request so the button cannot be double-fired.
 */
export function useDevServerPreview(sessionId: string | null): DevServerPreview {
  const [isPending, setIsPending] = useState(false);
  const store = useAppStoreApi();
  const devProcessId = useAppStore((state) =>
    sessionId ? state.processes.devProcessBySessionId[sessionId] : undefined,
  );
  const devStatus = useAppStore((state) =>
    devProcessId ? state.processes.processesById[devProcessId]?.status : undefined,
  );

  const start = useCallback(async () => {
    if (!sessionId) return;
    setIsPending(true);
    try {
      const inFlight = startsInFlight.get(sessionId);
      if (inFlight) {
        await inFlight;
        return;
      }
      const request = startProcess(sessionId, { kind: "dev" })
        .then((response) => {
          if (!response?.process) return;
          const started = toProcessStatusEntry(response.process);
          const state = store.getState();
          const existing = state.processes.processesById[started.processId];
          if (applyStartedProcess(existing, started)) {
            state.upsertProcessStatus(started);
          }
          state.setActiveProcess(started.sessionId, started.processId);
        })
        .catch(() => {
          // Already running, or the start failed; the WS status frame is the
          // source of truth either way.
        })
        .finally(() => {
          startsInFlight.delete(sessionId);
        });
      startsInFlight.set(sessionId, request);
      await request;
    } finally {
      setIsPending(false);
    }
  }, [sessionId, store]);

  const stop = useCallback(async () => {
    if (!sessionId || !devProcessId) return;
    setIsPending(true);
    try {
      await stopProcess(sessionId, { process_id: devProcessId });
    } catch {
      // Already gone; the WS status frame reconciles the button.
    } finally {
      setIsPending(false);
    }
  }, [sessionId, devProcessId]);

  return {
    devProcessId,
    isRunning: isProcessLive(devStatus),
    isPending,
    start,
    stop,
  };
}
