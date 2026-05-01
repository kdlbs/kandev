"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import { useRouter } from "next/navigation";
import { KanbanBoard } from "./kanban-board";
import { TaskPreviewPanel } from "./task-preview-panel";
import { useKanbanPreview } from "@/hooks/use-kanban-preview";
import { useKanbanLayout } from "@/hooks/use-kanban-layout";
import { useTaskSession } from "@/hooks/use-task-session";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useAppStore } from "@/components/state-provider";
import { Task } from "./kanban-card";
import type { KanbanState } from "@/lib/state/slices";
import { PREVIEW_PANEL } from "@/lib/settings/constants";
import { linkToTask } from "@/lib/links";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import {
  useEnsureTaskSession,
  type UseEnsureTaskSessionResult,
} from "@/hooks/domains/session/use-ensure-task-session";

type KanbanWithPreviewProps = {
  initialTaskId?: string;
  initialSessionId?: string;
};

function useUrlSync(selectedTaskId: string | null, selectedTaskSessionId: string | null) {
  useEffect(() => {
    if (typeof window === "undefined") return;

    const url = new URL(window.location.href);
    if (selectedTaskId) {
      url.searchParams.set("taskId", selectedTaskId);
    } else {
      url.searchParams.delete("taskId");
      url.searchParams.delete("sessionId");
    }

    if (selectedTaskId && selectedTaskSessionId) {
      url.searchParams.set("sessionId", selectedTaskSessionId);
    } else if (selectedTaskId) {
      // Task still open but no active session (e.g. all sessions deleted) —
      // drop any stale sessionId from the URL.
      url.searchParams.delete("sessionId");
    }

    window.history.replaceState({}, "", url.toString());
  }, [selectedTaskId, selectedTaskSessionId]);
}

function useEscapeKey(isOpen: boolean, close: () => void) {
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape" && isOpen) {
        close();
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, close]);
}

function useResizeHandler(
  isResizingRef: React.RefObject<boolean>,
  previewWidthPx: number,
  updatePreviewWidth: (width: number) => void,
) {
  return useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      (isResizingRef as React.MutableRefObject<boolean>).current = true;

      const startX = e.clientX;
      const startWidth = previewWidthPx;

      const handleMouseMove = (moveEvent: MouseEvent) => {
        if (!isResizingRef.current) return;
        const deltaX = startX - moveEvent.clientX;
        updatePreviewWidth(startWidth + deltaX);
      };

      const handleMouseUp = () => {
        (isResizingRef as React.MutableRefObject<boolean>).current = false;
        window.removeEventListener("mousemove", handleMouseMove);
        window.removeEventListener("mouseup", handleMouseUp);
      };

      window.addEventListener("mousemove", handleMouseMove);
      window.addEventListener("mouseup", handleMouseUp);
    },
    [isResizingRef, previewWidthPx, updatePreviewWidth],
  );
}

// Holds the user's explicit session pick for the selected task, resetting to null
// whenever the task changes. Uses React's "store previous props" pattern to avoid
// a setState-in-effect cascade (https://react.dev/reference/react/useState#storing-information-from-previous-renders).
function useSessionSelectionReset(
  selectedTaskId: string | null | undefined,
  initialValue: string | null = null,
): [string | null, Dispatch<SetStateAction<string | null>>] {
  const [value, setValue] = useState<string | null>(initialValue);
  const [prevTaskId, setPrevTaskId] = useState<string | null>(selectedTaskId ?? null);
  const currentTaskId = selectedTaskId ?? null;
  if (prevTaskId !== currentTaskId) {
    setPrevTaskId(currentTaskId);
    // Only reset on a real task-to-task transition. When prevTaskId is null we
    // are hydrating the initial task from the URL — preserve the seeded initialValue.
    if (prevTaskId !== null) {
      setValue(null);
    }
  }
  return [value, setValue];
}

function useSelectedTask(
  selectedTaskId: string | null | undefined,
  kanbanTasks: KanbanState["tasks"],
  snapshots: Record<string, { tasks: KanbanState["tasks"] }>,
) {
  return useMemo(() => {
    if (!selectedTaskId) return null;
    // The active workflow's tasks live in `kanban.tasks`, but cards from other
    // workflows can also appear in the board (multi-workflow swimlane view via
    // `kanbanMulti.snapshots`). Fall back to those so cross-workflow previews
    // are not auto-closed by the "task no longer exists" guard below.
    const task =
      kanbanTasks.find((t: KanbanState["tasks"][number]) => t.id === selectedTaskId) ??
      findTaskInSnapshots(selectedTaskId, snapshots);
    if (!task) return null;
    return {
      id: task.id,
      title: task.title,
      workflowStepId: task.workflowStepId,
      state: task.state,
      description: task.description,
      position: task.position,
      repositoryId: task.repositoryId,
      primarySessionId: task.primarySessionId,
    };
  }, [selectedTaskId, kanbanTasks, snapshots]);
}

export function KanbanWithPreview({ initialTaskId, initialSessionId }: KanbanWithPreviewProps) {
  const router = useRouter();
  const { isMobile } = useResponsiveBreakpoint();

  // Get tasks from the kanban store
  const kanbanTasks = useAppStore((state) => state.kanban.tasks);
  const kanbanMultiSnapshots = useAppStore((state) => state.kanbanMulti.snapshots);
  const setKanbanPreviewedTaskId = useAppStore((state) => state.setKanbanPreviewedTaskId);

  const { selectedTaskId, isOpen, previewWidthPx, open, close, updatePreviewWidth } =
    useKanbanPreview({
      initialTaskId,
      onClose: () => {
        // Cleanup handled by close
      },
    });

  // Mirror the previewed task id into the store so kanban cards can highlight
  // the currently-previewed card without prop-drilling through swimlanes.
  useEffect(() => {
    setKanbanPreviewedTaskId(isOpen ? (selectedTaskId ?? null) : null);
  }, [isOpen, selectedTaskId, setKanbanPreviewedTaskId]);
  useEffect(() => {
    return () => setKanbanPreviewedTaskId(null);
  }, [setKanbanPreviewedTaskId]);

  // Use custom hooks for layout and session management
  const { containerRef, shouldFloat, kanbanWidth } = useKanbanLayout(isOpen, previewWidthPx);
  const { sessionId: selectedTaskSessionId } = useTaskSession(selectedTaskId ?? null);

  // User-selected tab overrides the default primary session pick.
  // Reset when the selected task changes.
  const [userSelectedSessionId, setUserSelectedSessionId] = useSessionSelectionReset(
    selectedTaskId,
    initialSessionId ?? null,
  );

  // Track resize state
  const isResizingRef = useRef(false);

  const selectedTask = useSelectedTask(selectedTaskId, kanbanTasks, kanbanMultiSnapshots);

  // Close panel if selected task no longer exists in either the active
  // workflow's tasks or any loaded multi-workflow snapshot.
  useEffect(() => {
    if (isOpen && selectedTaskId && !selectedTask) {
      close();
    }
  }, [isOpen, selectedTaskId, selectedTask, close]);

  // Auto-start a session when the preview opens on a task with no session,
  // mirroring the full task page so the preview doesn't dead-end on
  // "No agents yet." The hook no-ops when no agent profile resolves.
  const ensureSession = useEnsureTaskSession(selectedTask, { enabled: isOpen });

  const handleNavigateToTask = useCallback(
    (task: Task) => {
      router.push(linkToTask(task.id));
    },
    [router],
  );

  const activeSessionId = selectedTaskId
    ? (userSelectedSessionId ?? selectedTask?.primarySessionId ?? selectedTaskSessionId)
    : null;

  useUrlSync(selectedTaskId ?? null, activeSessionId ?? null);

  const handlePreviewTaskWithData = useCallback(
    (task: Task) => {
      if (isOpen && selectedTaskId === task.id) {
        close();
      } else {
        open(task.id);
      }
    },
    [isOpen, selectedTaskId, open, close],
  );

  useEscapeKey(isOpen, close);

  const handleResizeMouseDown = useResizeHandler(isResizingRef, previewWidthPx, updatePreviewWidth);

  // On mobile, skip the preview panel entirely — card clicks navigate directly
  if (isMobile) {
    return (
      <div className="h-dvh w-full flex flex-col bg-background">
        <KanbanBoard />
      </div>
    );
  }

  return (
    <div ref={containerRef} className="h-screen w-full flex flex-col bg-background relative">
      {shouldFloat ? (
        <FloatingPreviewLayout
          kanbanWidth={kanbanWidth}
          previewWidthPx={previewWidthPx}
          selectedTask={selectedTask}
          activeSessionId={activeSessionId}
          ensureSession={ensureSession}
          onPreviewTask={handlePreviewTaskWithData}
          onNavigateToTask={handleNavigateToTask}
          onClose={close}
          onSessionChange={setUserSelectedSessionId}
          onResizeMouseDown={handleResizeMouseDown}
        />
      ) : (
        <InlinePreviewLayout
          kanbanWidth={kanbanWidth}
          previewWidthPx={previewWidthPx}
          isOpen={isOpen}
          selectedTask={selectedTask}
          activeSessionId={activeSessionId}
          ensureSession={ensureSession}
          onPreviewTask={handlePreviewTaskWithData}
          onNavigateToTask={handleNavigateToTask}
          onClose={close}
          onSessionChange={setUserSelectedSessionId}
          onResizeMouseDown={handleResizeMouseDown}
        />
      )}
    </div>
  );
}

function ResizeHandle({ onMouseDown }: { onMouseDown: (e: React.MouseEvent) => void }) {
  return (
    <div
      className="w-1 bg-border hover:bg-primary cursor-col-resize flex-shrink-0 relative group"
      onMouseDown={onMouseDown}
    >
      <div className="absolute inset-y-0 -left-2 -right-2" />
      <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-1 h-8 bg-border group-hover:bg-primary rounded-full transition-colors" />
    </div>
  );
}

type PreviewLayoutProps = {
  kanbanWidth: number;
  previewWidthPx: number;
  selectedTask: Task | null;
  activeSessionId: string | null;
  ensureSession: UseEnsureTaskSessionResult;
  onPreviewTask: (task: Task) => void;
  onNavigateToTask: (task: Task) => void;
  onClose: () => void;
  onSessionChange: (sessionId: string | null) => void;
  onResizeMouseDown: (e: React.MouseEvent) => void;
};

function FloatingPreviewLayout({
  kanbanWidth,
  previewWidthPx,
  selectedTask,
  activeSessionId,
  ensureSession,
  onPreviewTask,
  onNavigateToTask,
  onClose,
  onSessionChange,
  onResizeMouseDown,
}: PreviewLayoutProps) {
  return (
    <>
      <div className="flex-1 overflow-hidden" style={{ width: `${kanbanWidth}px` }}>
        <KanbanBoard onPreviewTask={onPreviewTask} onOpenTask={onNavigateToTask} />
      </div>
      <div
        className="fixed inset-0 bg-black/30 z-30"
        onClick={onClose}
        aria-label="Close preview"
      />
      <div
        className="fixed right-0 top-0 bottom-0 z-40 shadow-2xl bg-background flex"
        style={{
          width: `${previewWidthPx}px`,
          maxWidth: `${PREVIEW_PANEL.MAX_WIDTH_VW}vw`,
        }}
      >
        <ResizeHandle onMouseDown={onResizeMouseDown} />
        <div className="flex-1 min-w-0 overflow-hidden">
          <TaskPreviewPanel
            task={selectedTask}
            sessionId={activeSessionId}
            ensureSession={ensureSession}
            onClose={onClose}
            onMaximize={(task) => onNavigateToTask(task)}
            onSessionChange={onSessionChange}
          />
        </div>
      </div>
    </>
  );
}

function InlinePreviewLayout({
  kanbanWidth,
  previewWidthPx,
  isOpen,
  selectedTask,
  activeSessionId,
  ensureSession,
  onPreviewTask,
  onNavigateToTask,
  onClose,
  onSessionChange,
  onResizeMouseDown,
}: PreviewLayoutProps & { isOpen: boolean }) {
  return (
    <div className="flex-1 flex overflow-hidden">
      <div className="overflow-hidden" style={{ width: `${kanbanWidth}px` }}>
        <KanbanBoard onPreviewTask={onPreviewTask} onOpenTask={onNavigateToTask} />
      </div>
      {isOpen && (
        <div
          className="flex-shrink-0 border-l bg-background flex"
          style={{ width: `${previewWidthPx}px` }}
        >
          <ResizeHandle onMouseDown={onResizeMouseDown} />
          <div className="flex-1 min-w-0 overflow-hidden">
            <TaskPreviewPanel
              task={selectedTask}
              sessionId={activeSessionId}
              ensureSession={ensureSession}
              onClose={onClose}
              onMaximize={(task) => onNavigateToTask(task)}
              onSessionChange={onSessionChange}
            />
          </div>
        </div>
      )}
    </div>
  );
}
