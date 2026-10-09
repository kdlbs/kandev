"use client";

import { useCallback, useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { fetchTask } from "@/lib/api/domains/kanban-api";
import { resolveComposerWorkspaceId } from "@/components/task/chat/composer-workspace";

type LookupState = {
  taskId: string | null;
  retryAttempt: number;
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
  const [retryState, setRetryState] = useState({ taskId: null as string | null, attempt: 0 });
  const retryAttempt = retryState.taskId === taskId ? retryState.attempt : 0;
  const [lookup, setLookup] = useState<LookupState>({
    taskId: null,
    retryAttempt: 0,
    status: "loading",
    workspaceId: null,
  });

  useEffect(() => {
    if (!taskId) return;
    const currentLookup =
      lookup.taskId === taskId && lookup.retryAttempt === retryAttempt ? lookup : null;
    if (currentLookup && currentLookup.status !== "loading") return;
    if (!currentLookup && cachedWorkspaceId && retryAttempt === 0) return;

    let active = true;
    if (!currentLookup) {
      setLookup({ taskId, retryAttempt, status: "loading", workspaceId: null });
    }
    void fetchTaskWorkspace(taskId).then(
      (workspaceId) => {
        if (active) setLookup({ taskId, retryAttempt, status: "resolved", workspaceId });
      },
      () => {
        if (active) setLookup({ taskId, retryAttempt, status: "failed", workspaceId: null });
      },
    );
    return () => {
      active = false;
    };
  }, [cachedWorkspaceId, lookup, retryAttempt, taskId]);

  const retry = useCallback(() => {
    setRetryState((state) => ({
      taskId,
      attempt: state.taskId === taskId ? state.attempt + 1 : 1,
    }));
  }, [taskId]);
  const currentLookup =
    taskId && lookup.taskId === taskId && lookup.retryAttempt === retryAttempt ? lookup : null;
  let workspaceId: string | null = null;
  let status: LookupState["status"] = taskId ? "loading" : "resolved";
  if (!taskId) {
    status = "resolved";
  } else if (currentLookup) {
    status = currentLookup.status;
    if (currentLookup.status === "resolved") workspaceId = currentLookup.workspaceId;
  } else if (retryAttempt === 0 && cachedWorkspaceId) {
    workspaceId = cachedWorkspaceId;
    status = "resolved";
  }

  return { workspaceId, status, retry };
}
