import { useEffect, type RefObject } from "react";
import type { DockviewApi } from "dockview-react";
import { panelPortalManager } from "@/lib/layout/panel-portal-manager";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useAppStore } from "@/components/state-provider";

export function useCompactDockviewDefault(compact: boolean) {
  const setDefaultPreset = useDockviewStore((s) => s.setDefaultPreset);
  useEffect(() => {
    setDefaultPreset(compact ? "compact" : "default");
    return () => setDefaultPreset("default");
  }, [compact, setDefaultPreset]);
}

export function useDockviewUnmountCleanup(
  saveTimerRef: RefObject<ReturnType<typeof setTimeout> | null>,
  readyDisposersRef: RefObject<Array<() => void>>,
  ownedApiRef: RefObject<DockviewApi | null>,
) {
  useEffect(() => {
    const timerRef = saveTimerRef;
    const disposersRef = readyDisposersRef;
    return () => {
      for (const dispose of disposersRef.current.splice(0)) dispose();
      if (timerRef.current) clearTimeout(timerRef.current);
      const owner = ownedApiRef.current;
      const state = useDockviewStore.getState();
      if (owner && state.api === owner) {
        panelPortalManager.releaseAll();
        state.setApi(null);
      }
    };
  }, [ownedApiRef, readyDisposersRef, saveTimerRef]);
}

/**
 * Dockview's session-scoped panels must agree with the task page's ownership
 * guard. The global active selection wins only while its row belongs to the
 * rendered task; a stale selection from a previous route falls back to the
 * guarded session prop instead of leaking another task's environment, preview,
 * or review context into the retained layout.
 */
export function resolveDockviewSessionId(params: {
  activeSessionId: string | null;
  activeSessionTaskId: string | null;
  taskId: string | null;
  guardedSessionId: string | null | undefined;
}): string | null {
  const { activeSessionId, activeSessionTaskId, taskId, guardedSessionId } = params;
  if (activeSessionId && taskId && activeSessionTaskId === taskId) return activeSessionId;
  return guardedSessionId ?? null;
}

/** Owned-session selection for the desktop dockview surface. */
export function useOwnedDockviewSessionId(
  taskId: string | null,
  guardedSessionId: string | null | undefined,
): string | null {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const activeSessionTaskId = useAppStore((state) =>
    activeSessionId ? (state.taskSessions.items[activeSessionId]?.task_id ?? null) : null,
  );
  return resolveDockviewSessionId({
    activeSessionId,
    activeSessionTaskId,
    taskId,
    guardedSessionId,
  });
}
