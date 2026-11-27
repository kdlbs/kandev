import { useCallback, useEffect, useState } from "react";

import { useRouter } from "@/lib/routing/client-router";
import { isTaskDetailPath, linkToTask } from "@/lib/links";
import type { ConversationTask as Task } from "@/lib/types/conversation-search";
import { cancelSidebarTaskReveal, revealSidebarTask } from "@/lib/sidebar/task-navigation";

export function useCommandPanelTaskNavigation(pathname: string, activeTaskId: string | null) {
  const router = useRouter();
  const [pendingTaskRevealId, setPendingTaskRevealId] = useState<string | null>(null);

  const handleTaskNavigation = useCallback(
    (task: Task) => {
      cancelSidebarTaskReveal();
      setPendingTaskRevealId(task.id);
      const sessionId = task.conversation_match?.session_id;
      const href = linkToTask(task.id, { sessionId });
      if (sessionId) router.push(href, { taskSessionActivation: { taskId: task.id, sessionId } });
      else router.push(href);
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
