import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from "react";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useSaveTaskFlag } from "@/hooks/use-save-task-flag";
import { TaskFlagEditor } from "./task-flag-editor";
import type { TaskSwitcherItem } from "./task-switcher-types";

export type OpenTaskFlag = (
  task: TaskSwitcherItem,
  invoker: HTMLElement | null,
  readOnly?: boolean,
) => void;
const TaskFlagContext = createContext<OpenTaskFlag | undefined>(undefined);
export const useOpenTaskFlag = () => useContext(TaskFlagContext);

export function TaskFlagEditorProvider({
  children,
  onHandoff,
  focusFallback,
  fallbackSelector = "[data-sidebar-toggle]",
}: {
  children: ReactNode;
  onHandoff?: (active: boolean) => void;
  focusFallback?: { current: HTMLElement | null };
  fallbackSelector?: string;
}) {
  const [target, setTarget] = useState<{
    task: TaskSwitcherItem;
    invoker: HTMLElement | null;
    row: HTMLElement | null;
    readOnly: boolean;
  } | null>(null);
  const closing = useRef(false);
  const saveTaskFlag = useSaveTaskFlag();
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const open = useCallback<OpenTaskFlag>(
    (task, invoker, readOnly = false) => {
      closing.current = false;
      onHandoff?.(true);
      setTarget({
        task,
        invoker,
        row: invoker?.closest<HTMLElement>("[data-task-row-id]") ?? null,
        readOnly,
      });
    },
    [onHandoff],
  );

  return (
    <TaskFlagContext.Provider value={open}>
      {children}
      {target && (
        <TaskFlagEditor
          key={target.task.id}
          savedNote={target.task.flagNote ?? ""}
          readOnly={target.readOnly}
          touch={isMobile || !isFinePointer}
          onSubmit={(note) => saveTaskFlag(target.task.id, note)}
          onClose={() => {
            closing.current = true;
            setTarget(null);
            onHandoff?.(false);
          }}
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            if (!closing.current) return;
            const fallback = focusFallback?.current;
            const focusTarget = [target.invoker, target.row, fallback].find(
              (element) => element?.isConnected && element !== document.body,
            );
            (focusTarget ?? document.querySelector<HTMLElement>(fallbackSelector))?.focus({
              preventScroll: true,
            });
          }}
        />
      )}
    </TaskFlagContext.Provider>
  );
}
