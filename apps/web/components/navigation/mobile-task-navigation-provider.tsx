import {
  createContext,
  lazy,
  Suspense,
  useCallback,
  useContext,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import { Drawer } from "@kandev/ui/drawer";
import { useAppStore } from "@/components/state-provider";
import type { TaskSheetSelectionController } from "@/components/task/mobile/session-task-switcher-sheet-selection";

const TaskSidebar = lazy(() =>
  import("@/components/task/mobile/session-task-switcher-sheet").then((m) => ({
    default: m.SessionTaskSwitcherSheet,
  })),
);

type Outlet = {
  element: HTMLDivElement;
  close: () => void;
  navigate: (taskId: string) => void;
  selection?: TaskSheetSelectionController | null;
};
const Context = createContext<(outlet: Outlet | null) => void>(() => {});
export const useMobileTaskNavigationOutlet = () => useContext(Context);

/** Task action dialogs outlive the menu portal and responsive page headers. */
export function MobileTaskNavigationProvider({ children }: { children: ReactNode }) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const workflowId = useAppStore((s) => s.workflows.activeId);
  const [outlet, setOutlet] = useState<Outlet | null>(null);
  const configuration = useRef<Outlet | null>(null);
  const [requested, setRequested] = useState(false);
  const register = useCallback((next: Outlet | null) => {
    setOutlet(next);
    if (next) {
      configuration.current = next;
      setRequested(true);
    }
  }, []);
  return (
    <Context.Provider value={register}>
      {children}
      {requested && workspaceId && (
        <Suspense fallback={null}>
          <TaskSidebar
            key={workspaceId}
            workspaceId={workspaceId}
            workflowId={workflowId}
            open={!!outlet}
            onOpenChange={(open) => {
              if (!open) configuration.current?.close();
            }}
            navigate={configuration.current?.navigate}
            selection={configuration.current?.selection ?? undefined}
            presentation="drawer"
            renderInline={(body) =>
              outlet
                ? createPortal(
                    <Drawer open modal={false}>
                      {body}
                    </Drawer>,
                    outlet.element,
                  )
                : null
            }
          />
        </Suspense>
      )}
    </Context.Provider>
  );
}
