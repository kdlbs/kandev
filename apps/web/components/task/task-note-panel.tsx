"use client";

import { memo, useCallback, useRef } from "react";
import dynamic from "@/lib/routing/client-dynamic";
import { IconFileText, IconLoader2, IconRobot } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { PanelBody, PanelRoot } from "./panel-primitives";
import { TaskNotePanelHeader } from "./task-note-panel-header";
import { useTaskNote } from "@/hooks/domains/session/use-task-note";
import { useNoteActions } from "@/hooks/domains/kanban/use-note-actions";
import { useAppStore } from "@/components/state-provider";

const NoteEditor = dynamic(
  () =>
    import("@/components/editors/tiptap/tiptap-note-editor").then((mod) => mod.TipTapNoteEditor),
  {
    ssr: false,
    loading: () => (
      <div className="flex h-full items-center justify-center text-muted-foreground text-sm">
        Loading editor...
      </div>
    ),
  },
);

type TaskNotePanelProps = {
  taskId: string | null;
  visible?: boolean;
};

export const TaskNotePanel = memo(function TaskNotePanel({
  taskId,
  visible = true,
}: TaskNotePanelProps) {
  const { note, draftContent, setDraftContent, editorKey, isLoading } = useTaskNote(taskId, {
    visible,
  });
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const { enhanceNoteWithAI, isEnhancing } = useNoteActions({
    resolvedSessionId: activeSessionId,
    taskId,
  });
  const editorWrapperRef = useRef<HTMLDivElement>(null);

  const handleEmptyStateClick = useCallback(() => {
    const el = editorWrapperRef.current?.querySelector(".ProseMirror");
    if (el) (el as HTMLElement).focus();
  }, []);

  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center text-muted-foreground">
        <IconLoader2 className="mr-2 h-5 w-5 animate-spin" />
        <span className="text-sm">Loading notes...</span>
      </div>
    );
  }

  if (!taskId) {
    return (
      <div className="flex h-full items-center justify-center text-muted-foreground">
        <span className="text-sm">No task selected</span>
      </div>
    );
  }

  return (
    <PanelRoot data-testid="notes-panel">
      <TaskNotePanelHeader
        canEnhance={Boolean(activeSessionId) && draftContent.trim().length > 0}
        isEnhancing={isEnhancing}
        onEnhance={() => void enhanceNoteWithAI(draftContent)}
      />
      <PanelBody
        ref={editorWrapperRef}
        padding={false}
        scroll={false}
        className={cn("relative cursor-text transition-colors bg-background")}
        onClick={handleEmptyStateClick}
        data-panel-kind="notes"
      >
        <NoteEditor
          key={`${taskId}-${editorKey}`}
          value={draftContent}
          onChange={setDraftContent}
          placeholder="Start typing task notes..."
        />
        {!note && draftContent.trim() === "" && (
          <TaskNoteEmptyState onClick={handleEmptyStateClick} />
        )}
      </PanelBody>
    </PanelRoot>
  );
});

function TaskNoteEmptyState({ onClick }: { onClick: () => void }) {
  return (
    <div
      className="absolute inset-0 flex items-center justify-center pointer-events-none"
      onClick={onClick}
    >
      <div className="flex max-w-md flex-col items-center gap-5 px-6 text-center">
        <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-muted/50">
          <IconFileText className="h-6 w-6 text-muted-foreground" />
        </div>
        <div>
          <h3 className="mb-1 text-sm font-medium text-foreground">Capture task notes</h3>
          <p className="text-xs text-muted-foreground">
            Keep quick context, reminders, and handoff details with the task
          </p>
        </div>
        <div className="flex w-full flex-col gap-3">
          <div className="flex items-start gap-3">
            <IconRobot className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
            <p className="text-left text-xs text-muted-foreground">
              Ask the active agent to rewrite and polish the notes when you want a cleaner summary
            </p>
          </div>
          <div className="flex items-start gap-3">
            <IconFileText className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
            <p className="text-left text-xs text-muted-foreground">
              Notes auto-save as you type and stay shared across everyone viewing the task
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
