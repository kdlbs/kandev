import { lazy, Suspense, useCallback, useEffect, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { isOfficeWorkspace, selectActiveWorkspace } from "@/lib/state/slices/workspace/selectors";
import { linkToTask } from "@/lib/links";
import { useRouter } from "@/lib/routing/client-router";

const TaskViews = lazy(() =>
  import("@/components/task/mobile/session-task-switcher-sheet").then((module) => ({
    default: module.SessionTaskSwitcherSheet,
  })),
);

function TaskViewSurface({
  workspaceId,
  open,
  onOpenChange,
  restoreFocus,
}: {
  workspaceId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  restoreFocus: (event: Event) => void;
}) {
  const workflowId = useAppStore((state) => state.workflows.activeId);
  const router = useRouter();
  const navigate = useCallback((taskId: string) => router.push(linkToTask(taskId)), [router]);
  return (
    <Suspense fallback={null}>
      <TaskViews
        open={open}
        onOpenChange={onOpenChange}
        workspaceId={workspaceId}
        workflowId={workflowId}
        presentation="drawer"
        navigate={navigate}
        onCloseAutoFocus={restoreFocus}
      />
    </Suspense>
  );
}

export function useTaskViewNavigation(closeMenu: () => void) {
  const workspace = useAppStore(selectActiveWorkspace);
  const { isMobile } = useResponsiveBreakpoint();
  const kanbanWorkspace = workspace && !isOfficeWorkspace(workspace);
  const [open, setOpen] = useState(false);
  const [mounted, setMounted] = useState(false);
  const requested = useRef(false);
  const frame = useRef(0);
  const opener = useRef<HTMLElement | null>(null);
  useEffect(() => () => cancelAnimationFrame(frame.current), []);

  const openTaskViews =
    isMobile && kanbanWorkspace
      ? () => {
          requested.current = true;
          closeMenu();
        }
      : undefined;
  const onMenuCloseAutoFocus = () => {
    if (!requested.current) return;
    requested.current = false;
    // The menu restores its own trigger before the task drawer takes focus.
    frame.current = requestAnimationFrame(() => {
      opener.current =
        document.activeElement instanceof HTMLElement ? document.activeElement : null;
      setMounted(true);
      setOpen(true);
    });
  };
  const restoreFocus = (event: Event) => {
    event.preventDefault();
    if (opener.current?.isConnected) opener.current.focus();
  };
  return {
    openTaskViews,
    onMenuCloseAutoFocus,
    // Child dialogs outlive the drawer, including phone-to-tablet layout changes.
    dialog:
      kanbanWorkspace && mounted ? (
        <TaskViewSurface
          key={workspace.id}
          workspaceId={workspace.id}
          open={open}
          onOpenChange={setOpen}
          restoreFocus={restoreFocus}
        />
      ) : null,
  };
}
