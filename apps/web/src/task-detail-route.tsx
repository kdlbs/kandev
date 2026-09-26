"use client";

import { useEffect, useState } from "react";
import { StateHydrator } from "@/components/state-hydrator";
import { KanbanTaskShell } from "@/app/tasks/[id]/kanban-task-shell";
import {
  extractInitialRepositories,
  extractInitialScripts,
  fetchSessionDataForTask,
  type FetchedSessionData,
} from "@/lib/ssr/session-page-state";
import { useTranslation } from "react-i18next";
import { isDetachedManagedConversation } from "@/lib/plugins/retained-managed-conversation";
import { RetainedManagedConversationTranscript } from "@/components/plugins/retained-managed-conversation-transcript";

type TaskDetailRouteProps = {
  taskId: string;
  sessionId?: string;
  layout?: string | null;
  simple?: string;
  mode?: string;
  initialData?: FetchedSessionData;
};

type TaskDetailRouteState =
  | { routeKey: string; status: "loading"; data: null }
  | { routeKey: string; status: "loaded"; data: FetchedSessionData }
  | { routeKey: string; status: "error"; data: null };

function taskRouteKey(taskId: string, sessionId?: string): string {
  return `${taskId}\u0000${sessionId ?? ""}`;
}

function routeDataMatchesTask(
  data: FetchedSessionData | undefined,
  taskId: string,
): data is FetchedSessionData {
  return data?.task?.id === taskId;
}

function initialRouteState(
  initialData: FetchedSessionData | undefined,
  taskId: string,
  sessionId?: string,
): TaskDetailRouteState {
  const routeKey = taskRouteKey(taskId, sessionId);
  if (
    routeDataMatchesTask(initialData, taskId) &&
    (!sessionId || initialData.sessionId === sessionId)
  ) {
    return { routeKey, status: "loaded", data: initialData };
  }
  return { routeKey, status: "loading", data: null };
}

export function TaskDetailRoute({
  taskId,
  sessionId,
  layout,
  simple,
  mode,
  initialData,
}: TaskDetailRouteProps) {
  const { t } = useTranslation();
  const [routeState, setRouteState] = useState<TaskDetailRouteState>(() =>
    initialRouteState(initialData, taskId, sessionId),
  );
  const routeKey = taskRouteKey(taskId, sessionId);
  const currentRouteState =
    routeState.routeKey === routeKey
      ? routeState
      : initialRouteState(initialData, taskId, sessionId);

  useEffect(() => {
    if (
      routeDataMatchesTask(initialData, taskId) &&
      (!sessionId || initialData.sessionId === sessionId)
    ) {
      setRouteState({ routeKey, status: "loaded", data: initialData });
      return;
    }
    let cancelled = false;
    setRouteState({ routeKey, status: "loading", data: null });
    fetchSessionDataForTask(taskId, sessionId)
      .then((next) => {
        if (!cancelled) setRouteState({ routeKey, status: "loaded", data: next });
      })
      .catch((error) => {
        if (!cancelled) {
          console.warn(
            "Could not load /t/:taskId route data; task page will fall back to client fetches:",
            error instanceof Error ? error.message : String(error),
          );
          setRouteState({ routeKey, status: "error", data: null });
        }
      });
    return () => {
      cancelled = true;
    };
  }, [initialData, routeKey, sessionId, taskId]);

  if (currentRouteState.status === "loading") {
    return (
      <div className="flex h-full min-h-0 w-full items-center justify-center bg-background">
        <p role="status" aria-live="polite" className="text-sm text-muted-foreground">
          {t("common:loadingTask")}
        </p>
      </div>
    );
  }

  const data = currentRouteState.data;
  const activeSessionId = sessionId ?? data?.sessionId ?? null;
  const initialState = data?.initialState ?? null;
  const task = data?.task ?? null;

  return (
    <>
      {initialState ? (
        <StateHydrator initialState={initialState} sessionId={activeSessionId ?? undefined} />
      ) : null}
      {task && isDetachedManagedConversation(task) ? (
        <RetainedManagedConversationTranscript task={task} sessionId={activeSessionId} />
      ) : (
        <KanbanTaskShell
          task={task}
          taskId={taskId}
          sessionId={activeSessionId}
          initialRepositories={extractInitialRepositories(initialState, task)}
          initialScripts={extractInitialScripts(initialState, task)}
          initialTerminals={data?.initialTerminals ?? []}
          defaultLayouts={{}}
          initialLayout={layout}
          urlSimple={simple}
          urlMode={mode}
        />
      )}
    </>
  );
}
