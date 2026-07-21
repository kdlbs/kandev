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

type TaskDetailRouteProps = {
  taskId: string;
  sessionId?: string;
  layout?: string | null;
  simple?: string;
  mode?: string;
  initialData?: FetchedSessionData;
};

type TaskDetailRouteState =
  | { status: "loading"; data: null }
  | { status: "loaded"; data: FetchedSessionData }
  | { status: "error"; data: null };

function routeDataMatchesTask(
  data: FetchedSessionData | undefined,
  taskId: string,
): data is FetchedSessionData {
  return data?.task?.id === taskId;
}

function initialRouteState(
  initialData: FetchedSessionData | undefined,
  taskId: string,
): TaskDetailRouteState {
  if (routeDataMatchesTask(initialData, taskId)) {
    return { status: "loaded", data: initialData };
  }
  return { status: "loading", data: null };
}

export function TaskDetailRoute({
  taskId,
  sessionId,
  layout,
  simple,
  mode,
  initialData,
}: TaskDetailRouteProps) {
  const [routeState, setRouteState] = useState<TaskDetailRouteState>(() =>
    initialRouteState(initialData, taskId),
  );

  useEffect(() => {
    if (routeDataMatchesTask(initialData, taskId)) {
      setRouteState({ status: "loaded", data: initialData });
      return;
    }
    let cancelled = false;
    setRouteState({ status: "loading", data: null });
    fetchSessionDataForTask(taskId)
      .then((next) => {
        if (!cancelled) setRouteState({ status: "loaded", data: next });
      })
      .catch((error) => {
        if (!cancelled) {
          console.warn(
            "Could not load /t/:taskId route data; task page will fall back to client fetches:",
            error instanceof Error ? error.message : String(error),
          );
          setRouteState({ status: "error", data: null });
        }
      });
    return () => {
      cancelled = true;
    };
  }, [initialData, taskId]);

  if (routeState.status === "loading") {
    return <div className="h-full min-h-0 w-full bg-background" />;
  }

  const data = routeState.data;
  const activeSessionId = sessionId ?? data?.sessionId ?? null;
  const initialState = data?.initialState ?? null;
  const task = data?.task ?? null;

  return (
    <>
      {initialState ? (
        <StateHydrator initialState={initialState} sessionId={activeSessionId ?? undefined} />
      ) : null}
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
    </>
  );
}
