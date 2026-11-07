import { lazy, Suspense, useCallback } from "react";
import { Spinner } from "@kandev/ui/spinner";
import type { ViewContentProps } from "@/lib/kanban/view-registry";
import { BOARD_GRAPH_FOCUS_PARAM } from "@/lib/links";
import { useSearchParams } from "@/lib/routing/client-router";

// Lazy so the graph libraries load only when the Graph view is shown.
const BoardGraph = lazy(() =>
  import("@/components/task-graph/board-graph").then((m) => ({ default: m.BoardGraph })),
);

/** Board Graph view for one workflow lane, next to Pipeline. */
export function SwimlaneTaskGraphContent({ tasks, onPreviewTask, onOpenTask }: ViewContentProps) {
  const focusParam = useSearchParams().get(BOARD_GRAPH_FOCUS_PARAM);
  const focusId = focusParam && tasks.some((task) => task.id === focusParam) ? focusParam : null;
  const withTask = useCallback(
    (handler: (task: ViewContentProps["tasks"][number]) => void) => (taskId: string) => {
      const task = tasks.find((candidate) => candidate.id === taskId);
      if (task) handler(task);
    },
    [tasks],
  );
  if (tasks.length === 0) return null;
  return (
    <Suspense
      fallback={
        <div className="flex h-40 items-center justify-center">
          <Spinner aria-hidden="true" />
        </div>
      }
    >
      <BoardGraph
        tasks={tasks}
        focusId={focusId}
        onOpen={withTask(onOpenTask)}
        onPreview={withTask(onPreviewTask)}
      />
    </Suspense>
  );
}
