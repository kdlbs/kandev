import { useEffect, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { linkToTask } from "@/lib/links";
import { useRouter, useSearchParams } from "@/lib/routing/client-router";
import {
  TASK_GRAPH_PANEL,
  takePendingTaskGraphFocusTarget,
} from "@/hooks/domains/task/use-open-task-graph";
import { TaskGraphDialog } from "./task-graph-dialog";

type EnvLookupState = {
  tasks?: { activeSessionId: string | null };
  taskSessions?: { items: Record<string, { task_id: string } | undefined> };
  environmentIdBySessionId?: Record<string, string>;
};

/**
 * The environment whose layout the workbench will show for the routed task:
 * null when the task has no environment yet, "" while the active session still
 * belongs to the previous task.
 */
function routedTaskEnvId(state: EnvLookupState, taskId: string): string | null {
  const sessionId = state.tasks?.activeSessionId;
  if (!sessionId) return null;
  const session = state.taskSessions?.items[sessionId];
  if (session && session.task_id !== taskId) return "";
  return state.environmentIdBySessionId?.[sessionId] ?? null;
}

/**
 * Fulfils `?panel=task-graph`: opens the workbench tab (desktop) or the sheet
 * (phones) for the routed task, then drops the param so reloads do not reopen.
 */
export function TaskGraphPanelRequest({
  taskId,
  usesDockview,
}: {
  taskId: string;
  usesDockview: boolean;
}) {
  const params = useSearchParams();
  const router = useRouter();
  const requested = params.get("panel") === TASK_GRAPH_PANEL;
  const activeTaskId = useAppStore((state) => state.tasks?.activeTaskId ?? null);
  const taskEnvId = useAppStore((state) => routedTaskEnvId(state, taskId));
  const api = useDockviewStore((state) => state.api);
  const restoring = useDockviewStore((state) => state.isRestoringLayout);
  const layoutEnvId = useDockviewStore((state) => state.currentLayoutEnvId);
  const [sheetOpen, setSheetOpen] = useState(false);
  const returnFocusTo = useRef<HTMLElement | null>(null);
  // The task + layout env the request was fulfilled in; see the effect below.
  const fulfilled = useRef<{ taskId: string; envId: string | null } | null>(null);

  useEffect(() => {
    if (!requested || activeTaskId !== taskId) return;
    // Wait for the routed task's environment layout: restoring it replaces panels.
    const layoutSettled = taskEnvId === null || taskEnvId === layoutEnvId;
    if (usesDockview && (!api || restoring || !layoutSettled)) return;
    // A frame later, so a same-tick environment layout restore cannot drop the tab.
    const frame = requestAnimationFrame(() => {
      if (usesDockview) {
        useDockviewStore.getState().addTaskGraphPanel();
        fulfilled.current = { taskId, envId: layoutEnvId };
      } else {
        returnFocusTo.current = takePendingTaskGraphFocusTarget();
        setSheetOpen(true);
      }
      const next = new URLSearchParams(params);
      next.delete("panel");
      router.replace(linkToTask(taskId, { searchParams: next }));
    });
    return () => cancelAnimationFrame(frame);
  }, [
    requested,
    activeTaskId,
    taskId,
    taskEnvId,
    layoutEnvId,
    usesDockview,
    api,
    restoring,
    params,
    router,
  ]);

  // Opening a task for the first time can create its environment afterwards;
  // adopting that environment's layout replaces the panels, so re-add the tab once.
  useEffect(() => {
    const done = fulfilled.current;
    if (!usesDockview || !done || done.taskId !== taskId || activeTaskId !== taskId) return;
    if (!api || restoring || layoutEnvId === done.envId || taskEnvId !== layoutEnvId) return;
    fulfilled.current = { taskId, envId: layoutEnvId };
    if (!api.getPanel(TASK_GRAPH_PANEL)) useDockviewStore.getState().addTaskGraphPanel();
  }, [usesDockview, taskId, activeTaskId, api, restoring, layoutEnvId, taskEnvId]);

  if (usesDockview) return null;
  return (
    <TaskGraphDialog
      taskId={taskId}
      open={sheetOpen}
      onOpenChange={setSheetOpen}
      returnFocusTo={returnFocusTo}
    />
  );
}
