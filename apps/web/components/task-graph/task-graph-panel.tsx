import { lazy, Suspense } from "react";
import { Spinner } from "@kandev/ui/spinner";
import { useAppStore } from "@/components/state-provider";

// Lazy so the graph libraries load only when the tab is shown.
const TaskGraphContent = lazy(() =>
  import("./task-graph-content").then((m) => ({ default: m.TaskGraphContent })),
);

const noop = () => {};

/** Workbench tab: the graph of whichever task is open, with that task focused. */
export function TaskGraphPanel() {
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
  if (!taskId) return null;
  return (
    <div data-testid="task-graph-panel" className="flex h-full min-h-0 flex-col p-3">
      <Suspense
        fallback={
          <div className="flex flex-1 items-center justify-center">
            <Spinner aria-hidden="true" />
          </div>
        }
      >
        <TaskGraphContent key={taskId} taskId={taskId} onNavigate={noop} keepGraphOpen />
      </Suspense>
    </div>
  );
}
