import { useCallback } from "react";
import { useAppStoreApi } from "@/components/state-provider";
import { updateTask } from "@/lib/api";
import { applyTaskOverviewPatch } from "@/lib/ws/handlers/task-overview";

export function useSaveTaskFlag() {
  const store = useAppStoreApi();

  return useCallback(
    async (taskId: string, note: string) => {
      const scope = store.getState().taskOverview.scope;
      const task = await updateTask(taskId, { flag_note: note });
      if (typeof task.flag_note !== "string") throw new Error("Task flag update was not confirmed");
      const patch = { id: taskId, flagNote: task.flag_note, updatedAt: task.updated_at };
      store.setState((state) =>
        state.taskOverview.scope === scope
          ? applyTaskOverviewPatch(state, taskId, patch, task.workspace_id)
          : state,
      );
    },
    [store],
  );
}
