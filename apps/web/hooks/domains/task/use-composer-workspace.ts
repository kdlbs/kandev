"use client";

import { useCallback, useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { fetchTask } from "@/lib/api/domains/kanban-api";
import { resolveComposerWorkspaceId } from "@/components/task/chat/composer-workspace";

type LookupState = {
  taskId: string | null;
  status: "loading" | "resolved" | "failed";
  workspaceId: string | null;
};

const taskWorkspaceRequests = new Map<string, Promise<string>>();

function fetchTaskWorkspace(taskId: string): Promise<string> {
  const existing = taskWorkspaceRequests.get(taskId);
  if (existing) return existing;

  const request = fetchTask(taskId, { cache: "no-store" })
    .then((task) => {
      if (!task.workspace_id) throw new Error("Task has no workspace");
      return task.workspace_id;
    })
    .finally(() => {
      if (taskWorkspaceRequests.get(taskId) === request) taskWorkspaceRequests.delete(taskId);
    });
  taskWorkspaceRequests.set(taskId, request);
  return request;
}

export function useComposerWorkspace(sessionId: string | null, taskId: string | null) {
  const cachedWorkspaceId = useAppStore((state) =>
    resolveComposerWorkspaceId({
      sessionId,
      taskId,
      quickChatSessions: state.quickChat.sessions,
      activeWorkflowId: state.kanban.workflowId,
      activeTasks: state.kanban.tasks,
      snapshots: Object.values(state.kanbanMulti.snapshots),
      workflows: state.workflows.items,
      officeTasks: state.office.tasks.items,
    }),
  );
  const [retryVersion, setRetryVersion] = useState(0);
  const [lookup, setLookup] = useState<LookupState>({
    taskId: null,
    status: "loading",
    workspaceId: null,
  });

  useEffect(() => {
    if (!taskId || cachedWorkspaceId) return;

    let active = true;
    setLookup({ taskId, status: "loading", workspaceId: null });
    void fetchTaskWorkspace(taskId).then(
      (workspaceId) => {
        if (active) setLookup({ taskId, status: "resolved", workspaceId });
      },
      () => {
        if (active) setLookup({ taskId, status: "failed", workspaceId: null });
      },
    );
    return () => {
      active = false;
    };
  }, [cachedWorkspaceId, retryVersion, taskId]);

  const retry = useCallback(() => setRetryVersion((version) => version + 1), []);
  const currentLookup = lookup.taskId === taskId ? lookup : null;
  const workspaceId = cachedWorkspaceId ?? currentLookup?.workspaceId ?? null;
  const status =
    workspaceId || !taskId
      ? "resolved"
      : (currentLookup?.status ?? (taskId ? "loading" : "resolved"));

  return { workspaceId, status, retry };
}
