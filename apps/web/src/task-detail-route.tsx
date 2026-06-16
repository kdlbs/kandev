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
};

export function TaskDetailRoute({ taskId, sessionId, layout, simple, mode }: TaskDetailRouteProps) {
  const [data, setData] = useState<FetchedSessionData | null>(null);

  useEffect(() => {
    let cancelled = false;
    setData(null);
    fetchSessionDataForTask(taskId)
      .then((next) => {
        if (!cancelled) setData(next);
      })
      .catch((error) => {
        if (!cancelled) {
          console.warn(
            "Could not load /t/:taskId route data; task page will fall back to client fetches:",
            error instanceof Error ? error.message : String(error),
          );
          setData(null);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [taskId]);

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
