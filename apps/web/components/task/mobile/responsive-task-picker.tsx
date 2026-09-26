import { lazy, Suspense, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

const TaskPicker = lazy(() =>
  import("./session-task-switcher-sheet").then((module) => ({
    default: module.SessionTaskSwitcherSheet,
  })),
);

/** Owns task dialogs above layout changes so rotating a phone preserves drafts. */
export function ResponsiveTaskPicker({
  workspaceId,
  workflowId,
}: {
  workspaceId: string | null;
  workflowId: string | null;
}) {
  const { isMobile } = useResponsiveBreakpoint();
  const open = useAppStore((state) => state.mobileSession.isTaskSwitcherOpen);
  const setOpen = useAppStore((state) => state.setMobileSessionTaskSwitcherOpen);
  const [requested, setRequested] = useState(false);
  const [entry, setEntry] = useState({ open: false, opener: null as HTMLElement | null });
  if (open && !requested) setRequested(true);
  if (entry.open !== open) {
    setEntry({
      open,
      opener:
        open && document.activeElement instanceof HTMLElement
          ? document.activeElement
          : entry.opener,
    });
  }
  if (!requested && !open) return null;
  return (
    <Suspense fallback={null}>
      <TaskPicker
        workspaceId={workspaceId}
        workflowId={workflowId}
        open={open}
        onOpenChange={setOpen}
        presentation={isMobile ? "drawer" : "sheet"}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          if (entry.opener?.isConnected) entry.opener.focus({ preventScroll: true });
        }}
      />
    </Suspense>
  );
}
