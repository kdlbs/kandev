import { StateHydrator } from "@/components/state-hydrator";
import { readLayoutDefaults } from "@/lib/layout/read-layout-defaults";
import { TaskPageContent } from "@/components/task/task-page-content";
import {
  type FetchedSessionData,
  fetchSessionDataForTask,
  extractInitialRepositories,
  extractInitialScripts,
} from "@/lib/ssr/session-page-state";

export default async function TaskPage({
  params,
  searchParams,
}: {
  params: Promise<{ taskId: string }>;
  searchParams: Promise<{ layout?: string }>;
}) {
  let fetchedData: FetchedSessionData | null = null;
  const defaultLayouts = await readLayoutDefaults();
  const { layout: initialLayout } = await searchParams;
  const { taskId } = await params;

  try {
    fetchedData = await fetchSessionDataForTask(taskId);
  } catch (error) {
    console.warn(
      "Could not SSR task page (client will load via WebSocket):",
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
      <TaskPageContent
        task={task}
        taskId={taskId}
        sessionId={sessionId}
        initialRepositories={extractInitialRepositories(initialState, task)}
        initialScripts={extractInitialScripts(initialState, task)}
        initialTerminals={initialTerminals}
        defaultLayouts={defaultLayouts}
        initialLayout={initialLayout}
      />
    </>
  );
}
