import { StateHydrator } from "@/components/state-hydrator";
import { readLayoutDefaults } from "@/lib/layout/read-layout-defaults";
import {
  type FetchedSessionData,
  extractInitialRepositories,
  extractInitialScripts,
  fetchSessionDataForTask,
} from "@/lib/ssr/session-page-state";
import { KanbanTaskShell } from "@/app/tasks/[id]/kanban-task-shell";

/**
 * `/t/:taskId` — canonical kanban task detail route.
 */
export default async function TaskPage({
  params,
  searchParams,
}: {
  params: Promise<{ taskId: string }>;
  searchParams: Promise<{ layout?: string; simple?: string; mode?: string }>;
}) {
  const { taskId } = await params;
  const search = await searchParams;
  const defaultLayouts = await readLayoutDefaults();

  let fetchedData: FetchedSessionData | null = null;
  try {
    fetchedData = await fetchSessionDataForTask(taskId);
  } catch (error) {
    console.warn(
      "Could not SSR /t/:taskId (client will load via WebSocket):",
      error instanceof Error ? error.message : String(error),
    );
  }

  const { task, sessionId, initialState, initialTerminals } = fetchedData ?? {
    task: null,
    sessionId: null,
    initialState: null,
    initialTerminals: [],
  };

  return (
    <>
      {initialState ? (
        <StateHydrator initialState={initialState} sessionId={sessionId ?? undefined} />
      ) : null}
      <KanbanTaskShell
        task={task}
        taskId={taskId}
        sessionId={sessionId}
        initialRepositories={extractInitialRepositories(initialState, task)}
        initialScripts={extractInitialScripts(initialState, task)}
        initialTerminals={initialTerminals}
        defaultLayouts={defaultLayouts}
        initialLayout={search.layout}
        urlSimple={search.simple}
        urlMode={search.mode}
      />
    </>
  );
}
