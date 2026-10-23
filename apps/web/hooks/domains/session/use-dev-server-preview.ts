"use client";

import { useCallback } from "react";
import { useStore } from "zustand";
import { getDevServerCommands, runDevServerCommand } from "@/lib/state/dev-server-commands";
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
  const store = useAppStoreApi();
  const commands = getDevServerCommands(store);
  const isPending = useStore(commands, (state) => !!sessionId && !!state[sessionId]);
  const devProcessId = useAppStore((state) =>
    sessionId ? state.processes.devProcessBySessionId[sessionId] : undefined,
  );
  const devStatus = useAppStore((state) =>
    devProcessId ? state.processes.processesById[devProcessId]?.status : undefined,
  );

  const start = useCallback(async () => {
    if (!sessionId) return;
    await runDevServerCommand(commands, sessionId, async () => {
      const response = await startProcess(sessionId, { kind: "dev" });
      if (!response?.process) return;
      const started = toProcessStatusEntry(response.process);
      const state = store.getState();
      const existing = state.processes.processesById[started.processId];
      if (applyStartedProcess(existing, started)) {
        state.upsertProcessStatus(started);
      }
      state.setActiveProcess(started.sessionId, started.processId);
    });
  }, [sessionId, store, commands]);

  const stop = useCallback(async () => {
    if (!sessionId) return;
    const inFlight = commands.getState()[sessionId];
    if (inFlight) {
      await inFlight;
      return;
    }
    if (!devProcessId) return;
    await runDevServerCommand(commands, sessionId, async () => {
      await stopProcess(sessionId, { process_id: devProcessId });
    });
  }, [sessionId, devProcessId, commands]);

  return {
    devProcessId,
    isRunning: isProcessLive(devStatus),
    isPending,
    start,
    stop,
  };
}
