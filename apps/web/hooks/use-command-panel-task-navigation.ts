import { useCallback, useEffect, useState } from "react";

import { useRouter } from "@/lib/routing/client-router";
import { isTaskDetailPath, linkToTask } from "@/lib/links";
import type { Task } from "@/lib/types/http";
import { revealSidebarTask } from "@/lib/sidebar/task-navigation";

export function useCommandPanelTaskNavigation(pathname: string, activeTaskId: string | null) {
  const router = useRouter();
  const [pendingTaskRevealId, setPendingTaskRevealId] = useState<string | null>(null);

  const handleTaskNavigation = useCallback(
    (task: Task) => {
      setPendingTaskRevealId(task.id);
      router.push(linkToTask(task.id));
    },
    [router],
  );

  useEffect(() => {
    if (
      !pendingTaskRevealId ||
      activeTaskId !== pendingTaskRevealId ||
      !isTaskDetailPath(pathname, pendingTaskRevealId)
    ) {
      return;
    }

    const taskId = pendingTaskRevealId;
    void revealSidebarTask(taskId).finally(() => {
      setPendingTaskRevealId((current) => (current === taskId ? null : current));
    });
  }, [activeTaskId, pathname, pendingTaskRevealId]);

  return handleTaskNavigation;
}
