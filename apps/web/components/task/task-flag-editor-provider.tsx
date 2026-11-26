import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useSaveTaskFlag } from "@/hooks/use-save-task-flag";
import { TaskFlagEditor } from "./task-flag-editor";
import type { TaskSwitcherItem } from "./task-switcher-types";

export type OpenTaskFlag = (
  task: TaskSwitcherItem,
  invoker: HTMLElement | null,
  readOnly?: boolean,
) => void;
type OpenTaskFlagPopover = (invoker: HTMLElement | null) => void;

type TaskFlagContextValue = {
  open: OpenTaskFlag;
  save: ReturnType<typeof useSaveTaskFlag>;
  restoreFocus: (...targets: Array<HTMLElement | null>) => void;
  registerPopover: (anchor: HTMLElement, open: OpenTaskFlagPopover) => (() => void) | undefined;
};

const TaskFlagContext = createContext<TaskFlagContextValue | undefined>(undefined);
export const useTaskFlagEditorContext = () => useContext(TaskFlagContext);
export const useOpenTaskFlag = () => useTaskFlagEditorContext()?.open;

function flagRow(element: HTMLElement | null) {
  return element?.closest<HTMLElement>("[data-slot='context-menu-trigger']");
}

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
  const popovers = useRef(new WeakMap<HTMLElement, OpenTaskFlagPopover>());
  const registerPopover = useCallback<TaskFlagContextValue["registerPopover"]>((anchor, open) => {
    const row = flagRow(anchor);
    if (!row) return;
    popovers.current.set(row, open);
    return () => popovers.current.delete(row);
  }, []);
  const save = useSaveTaskFlag();
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const touch = isMobile || !isFinePointer;
  const open = useCallback<OpenTaskFlag>(
    (task, invoker, readOnly = false) => {
      const row = flagRow(invoker);
      const openPopover = !touch && task.flagNote && row && popovers.current.get(row);
      if (openPopover) {
        openPopover(invoker);
        return;
      }
      closing.current = false;
      onHandoff?.(true);
      setTarget({
        task,
        invoker,
        row: invoker?.closest<HTMLElement>("[data-task-row-id]") ?? null,
        readOnly,
      });
    },
    [onHandoff, touch],
  );

  const restoreFocus = useCallback(
    (...targets: Array<HTMLElement | null>) => {
      const fallback = focusFallback?.current;
      const target = [...targets, fallback].find(
        (element) => element?.isConnected && element !== document.body,
      );
      (target ?? document.querySelector<HTMLElement>(fallbackSelector))?.focus({
        preventScroll: true,
      });
    },
    [fallbackSelector, focusFallback],
  );
  const context = useMemo(
    () => ({ open, save, registerPopover, restoreFocus }),
    [open, save, registerPopover, restoreFocus],
  );
  return (
    <TaskFlagContext.Provider value={context}>
      {children}
      {target && (
        <TaskFlagEditor
          key={target.task.id}
          savedNote={target.task.flagNote ?? ""}
          readOnly={target.readOnly}
          touch={touch}
          onSubmit={(note) => save(target.task.id, note)}
          onClose={() => {
            closing.current = true;
            setTarget(null);
            onHandoff?.(false);
          }}
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            if (!closing.current) return;
            restoreFocus(target.invoker, target.row);
          }}
        />
      )}
    </TaskFlagContext.Provider>
  );
}
