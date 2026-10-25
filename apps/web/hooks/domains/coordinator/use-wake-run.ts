"use client";

import { useEffect, useState, useSyncExternalStore } from "react";
import { fetchTask } from "@/lib/api/domains/kanban-api";
import { useTaskById } from "@/hooks/domains/kanban/use-task-by-id";
import {
  ensureWakeRun,
  getWakeRun,
  refreshOpenWakeRun,
  subscribeWakeRuns,
  wakeRunKey,
  type WakeRunEntry,
} from "@/lib/coordinator/wake-run-store";
import { useWebSocketClient } from "@/lib/ws/connection";

type Target = { workspaceId: string; coordinatorId: string } | null;

function targetOf(task: {
  workspaceId?: string | null;
  metadata?: Record<string, unknown> | null;
}): Target {
  const coordinatorId = task.metadata?.coordinator_id;
  if (typeof coordinatorId !== "string" || coordinatorId === "" || !task.workspaceId) return null;
  return { workspaceId: task.workspaceId, coordinatorId };
}

const fetched = new Map<string, Target>();

/** The conversation task's coordinator, from the loaded task or one fetch of it. */
function useWakeTarget(taskId: string): { target: Target; settled: boolean } {
  const loaded = useTaskById(taskId);
  const [remote, setRemote] = useState<{ taskId: string; target: Target } | null>(null);
  const cached = fetched.get(taskId);
  useEffect(() => {
    if (loaded || fetched.has(taskId)) return;
    let live = true;
    const settle = (target: Target) => {
      fetched.set(taskId, target);
      if (live) setRemote({ taskId, target });
    };
    fetchTask(taskId)
      .then((task) =>
        settle(
          targetOf({
            workspaceId: task.workspace_id,
            metadata: task.metadata as Record<string, unknown> | null | undefined,
          }),
        ),
      )
      .catch(() => settle(null));
    return () => {
      live = false;
    };
  }, [loaded, taskId]);
  if (loaded) return { target: targetOf(loaded), settled: true };
  if (cached !== undefined) return { target: cached, settled: true };
  if (remote?.taskId === taskId) return { target: remote.target, settled: true };
  return { target: null, settled: false };
}

/**
 * The run behind an unattended-turn message: one read per turn id through the
 * shared store, re-read while it is open on `coordinator.updated` with
 * `autonomy_changed`. A message without a resolvable coordinator is unavailable.
 */
export function useWakeRun(taskId: string, turnId: string): WakeRunEntry {
  const { target, settled } = useWakeTarget(taskId);
  const coordinatorId = target?.coordinatorId;
  const workspaceId = target?.workspaceId;
  const key = coordinatorId ? wakeRunKey(coordinatorId, turnId) : "";
  const entry = useSyncExternalStore(
    subscribeWakeRuns,
    () => (key ? getWakeRun(key) : undefined),
    () => undefined,
  );
  useEffect(() => {
    if (coordinatorId && workspaceId) ensureWakeRun(workspaceId, coordinatorId, turnId);
  }, [workspaceId, coordinatorId, turnId]);

  const wsClient = useWebSocketClient();
  useEffect(() => {
    if (!wsClient || !coordinatorId) return;
    return wsClient.on("coordinator.updated", (message) => {
      const payload = message.payload;
      if (payload.coordinator_id !== coordinatorId || payload.autonomy_changed !== true) return;
      refreshOpenWakeRun(coordinatorId, turnId);
    });
  }, [wsClient, coordinatorId, turnId]);

  if (settled && !target) return { status: "unavailable", run: null };
  return entry ?? { status: "loading", run: null };
}
