import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useRef,
  useState,
  type MouseEvent,
  type ReactNode,
  type RefObject,
} from "react";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useTaskManagementFlow } from "@/hooks/use-task-management-flow";
import { TaskMenuButton } from "@/components/task/task-item-menu-button";
import { TaskManagementSurface } from "@/components/task/task-management-surface";

type Entry = {
  taskId: string | null;
  open: (taskId: string, trigger: HTMLElement, point?: { x: number; y: number }) => void;
};
const ActionsContext = createContext<Entry | null>(null);

function visibleTrigger(board: HTMLDivElement | null, preferred: HTMLElement | null) {
  const bounds = board?.getBoundingClientRect();
  const visible = (element: HTMLElement) => {
    const rect = element.getBoundingClientRect();
    return (
      rect.width > 0 &&
      rect.left >= (bounds?.left ?? 0) &&
      rect.right <= (bounds?.right ?? window.innerWidth)
    );
  };
  if (preferred?.isConnected && visible(preferred)) return preferred;
  return (
    Array.from(
      board?.querySelectorAll<HTMLButtonElement>("[data-thread-task-menu] button") ?? [],
    ).find(visible) ?? null
  );
}

export function ThreadTaskActionsProvider({
  children,
  boardRef,
}: {
  children: ReactNode;
  boardRef: RefObject<HTMLDivElement | null>;
}) {
  const flow = useTaskManagementFlow();
  const origin = useRef<HTMLElement | null>(null);
  const fallback = useRef<HTMLDivElement>(null);
  const [point, setPoint] = useState({ x: 0, y: 0 });
  const focusReturnRef = useMemo(
    () => ({
      get current() {
        return visibleTrigger(boardRef.current, origin.current) ?? fallback.current;
      },
    }),
    [boardRef],
  );
  const restoreFocus = useCallback(() => {
    const active = document.activeElement;
    if (
      document.querySelector(
        '[role="dialog"][data-state="open"], [role="alertdialog"][data-state="open"], [role="menu"][data-state="open"]',
      )
    )
      return;
    if (
      active instanceof HTMLElement &&
      active !== document.body &&
      active !== origin.current &&
      !active.closest('[data-state="closed"]') &&
      active !== fallback.current
    )
      return;
    focusReturnRef.current?.focus({ preventScroll: true });
  }, [focusReturnRef]);
  const open = (taskId: string, trigger: HTMLElement, coordinates?: { x: number; y: number }) => {
    origin.current = trigger;
    const bounds = trigger.getBoundingClientRect();
    setPoint(coordinates ?? { x: bounds.left, y: bounds.bottom });
    flow.open(taskId);
  };
  return (
    <ActionsContext.Provider
      value={{ open, taskId: flow.stage === "menu" ? (flow.identity?.taskId ?? null) : null }}
    >
      <div
        ref={fallback}
        tabIndex={-1}
        className="flex h-full min-h-0 min-w-0 flex-col outline-none"
        data-thread-actions-fallback
      >
        {children}
      </div>
      {flow.identity && (
        <TaskManagementSurface
          flow={flow}
          point={point}
          anchorRef={origin}
          focusReturnRef={focusReturnRef}
          onReturnFocus={restoreFocus}
        />
      )}
    </ActionsContext.Provider>
  );
}

export function useThreadTaskContextMenu(taskId: string) {
  const actions = useContext(ActionsContext);
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  return (event: MouseEvent<HTMLElement>) => {
    if (isMobile || !isFinePointer || !actions) return;
    if (
      (event.target as Element).closest(
        'button, a, input, textarea, [contenteditable="true"], [role="combobox"]',
      )
    )
      return;
    event.preventDefault();
    const trigger = event.currentTarget.querySelector<HTMLElement>(
      "[data-thread-task-menu] button",
    );
    if (trigger) actions.open(taskId, trigger, { x: event.clientX, y: event.clientY });
  };
}

export function ThreadTaskMenuButton({ taskId }: { taskId: string }) {
  const actions = useContext(ActionsContext);
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const touch = isMobile || !isFinePointer;
  return (
    <span data-thread-task-menu={taskId} className="flex shrink-0 items-center">
      <TaskMenuButton
        visible
        expanded={actions?.taskId === taskId}
        touchTarget={touch}
        popup={touch ? "dialog" : "menu"}
        onOpen={(event) => actions?.open(taskId, event.currentTarget)}
      />
    </span>
  );
}
