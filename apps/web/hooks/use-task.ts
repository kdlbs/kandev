import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import type { KanbanState } from "@/lib/state/slices";

type Task = KanbanState["tasks"][number];

export function useTask(taskId: string | null) {
  // The active workflow's tasks live in `kanban.tasks`, but cross-workflow
  // tasks (PR-review boards, multi-workflow swimlanes) live in
  // `kanbanMulti.snapshots[*].tasks`. Mirror the lookup used by
  // `KanbanWithPreview.useSelectedTask` so consumers like the chat panel
  // can still resolve the task description for cross-workflow previews.
  const task = useAppStore((state) => {
    if (!taskId) return null;
    const fromActive = state.kanban.tasks.find((item: Task) => item.id === taskId);
    if (fromActive) return fromActive;
    return findTaskInSnapshots(taskId, state.kanbanMulti.snapshots);
  });

  useEffect(() => {
    if (!taskId) return;
    const client = getWebSocketClient();
    if (!client) return;
    const unsubscribe = client.subscribe(taskId);
    return () => {
      unsubscribe();
    };
  }, [taskId]);

  return task;
}
