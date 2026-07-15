import type { QueryClient } from "@tanstack/react-query";
import type { UnarchiveTaskResponse } from "@/lib/api/domains/kanban-api";
import { qk } from "./keys";

export function reconcileUnarchiveTaskQueries(
  queryClient: QueryClient,
  response: UnarchiveTaskResponse,
): void {
  for (const taskId of response.unarchived_ids) {
    const queryKey = qk.tasks.detail(taskId);
    queryClient.setQueryData(queryKey, (current: unknown) => {
      if (typeof current !== "object" || current === null || Array.isArray(current)) return current;
      return { ...current, archived_at: null };
    });
    void queryClient.invalidateQueries({ exact: true, queryKey });
  }

  void queryClient.invalidateQueries({ queryKey: ["tasks", "page"] });
  void queryClient.invalidateQueries({ queryKey: ["tasks", "infinite"] });
  void queryClient.invalidateQueries({
    predicate: (query) => query.queryKey[0] === "workflows" && query.queryKey[2] === "snapshot",
  });
}
