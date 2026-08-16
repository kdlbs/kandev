"use client";

import { useCallback, useState } from "react";
import { startProcess, stopProcess } from "@/lib/api";
import { useAppStore } from "@/components/state-provider";
import { isProcessLive, toProcessStatusEntry } from "@/lib/state/process-status";

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
 * Start/stop control for a session's dev-script process.
 *
 * The dev process is owned by the backend, so `isRunning` is derived from the
 * store (seeded by the start response, kept current by `session.process.status`
 * WebSocket frames) rather than from local state. `isPending` only covers the
 * in-flight request so the button cannot be double-fired.
 */
export function useDevServerPreview(sessionId: string | null): DevServerPreview {
  const [isPending, setIsPending] = useState(false);
  const devProcessId = useAppStore((state) =>
    sessionId ? state.processes.devProcessBySessionId[sessionId] : undefined,
  );
  const devStatus = useAppStore((state) =>
    devProcessId ? state.processes.processesById[devProcessId]?.status : undefined,
  );
  const upsertProcessStatus = useAppStore((state) => state.upsertProcessStatus);
  const setActiveProcess = useAppStore((state) => state.setActiveProcess);

  const start = useCallback(async () => {
    if (!sessionId) return;
    setIsPending(true);
    try {
      const response = await startProcess(sessionId, { kind: "dev" });
      if (response?.process) {
        const status = toProcessStatusEntry(response.process);
        upsertProcessStatus(status);
        setActiveProcess(status.sessionId, status.processId);
      }
    } catch {
      // Process may already be running; the WS status frame is the source of truth.
    } finally {
      setIsPending(false);
    }
  }, [sessionId, upsertProcessStatus, setActiveProcess]);

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
