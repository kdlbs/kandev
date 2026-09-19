import {
  createContext,
  useContext,
  useEffect,
  useLayoutEffect,
  useState,
  type ReactNode,
} from "react";
import {
  createTaskSheetSelectionController,
  type TaskSheetSelectionController,
} from "./session-task-switcher-sheet-selection";

const TaskSheetSelectionContext = createContext<TaskSheetSelectionController | null>(null);

export function useTaskSheetSelectionController() {
  const shared = useContext(TaskSheetSelectionContext);
  const [local] = useState(createTaskSheetSelectionController);
  useEffect(() => () => local.invalidate(), [local]);
  return shared ?? local;
}

export function useWorkbenchTaskSelection() {
  return useContext(TaskSheetSelectionContext);
}

/** The embedded sidebar and title picker must supersede each other's pending selection requests. */
export function TaskSheetSelectionProvider({
  children,
  workspaceId,
}: {
  children: ReactNode;
  workspaceId?: string | null;
}) {
  const controller = useTaskSheetSelectionController();
  useLayoutEffect(() => () => controller.invalidate(), [controller, workspaceId]);
  return (
    <TaskSheetSelectionContext.Provider value={controller}>
      {children}
    </TaskSheetSelectionContext.Provider>
  );
}
