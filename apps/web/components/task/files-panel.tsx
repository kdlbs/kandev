"use client";

import { memo, useCallback, useRef, useState, type RefObject } from "react";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { useAppStore } from "@/components/state-provider";
import { useFileOperations } from "@/hooks/use-file-operations";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { FileBrowser } from "@/components/task/file-browser";
import { useEnvironmentSessionId } from "@/hooks/use-environment-session-id";
import type { OpenFileTab } from "@/lib/types/backend";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import { useIsTaskArchived, ArchivedPanelPlaceholder } from "./task-archived-context";
import { AddWorkspaceSourcesDialog } from "./add-workspace-sources/add-workspace-sources-dialog";
import {
  getAddSourcesDisabledReason,
  hasActiveTaskSourceWork as getHasActiveTaskSourceWork,
} from "./add-workspace-sources/add-workspace-sources-availability";

type FilesPanelProps = {
  onOpenFile: (file: OpenFileTab) => void;
};

type TaskSourceDialogProps = {
  activeTask: KanbanState["tasks"][number] | undefined;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string | null;
  opener: HTMLElement | null;
  openerRef: RefObject<HTMLButtonElement | null>;
};

function TaskSourceDialog({
  activeTask,
  open,
  onOpenChange,
  workspaceId,
  opener,
  openerRef,
}: TaskSourceDialogProps) {
  if (!activeTask || !(activeTask.repositoryId ?? activeTask.repositories?.length)) return null;
  return (
    <AddWorkspaceSourcesDialog
      open={open}
      onOpenChange={onOpenChange}
      taskId={activeTask.id}
      executorType={activeTask.primaryExecutorType}
      workspaceId={workspaceId}
      opener={opener}
      openerRef={openerRef}
    />
  );
}

function NoTaskSelected() {
  return (
    <div className="flex items-center justify-center h-full text-muted-foreground text-xs">
      No task selected
    </div>
  );
}

function useFilesPanelSourceDialog(activeSessionId: string | null) {
  const [addSourcesOpen, setAddSourcesOpen] = useState(false);
  const [addSourcesOpener, setAddSourcesOpener] = useState<HTMLElement | null>(null);
  const addSourcesButtonRef = useRef<HTMLButtonElement>(null);
  const activeTask = useAppStore((state) => {
    const taskId = state.tasks.activeTaskId;
    return state.kanban.tasks.find((task) => task.id === taskId);
  });
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const hasActiveTaskSourceWork = useAppStore((state) => {
    if (!activeTask) return false;
    const sessionIds =
      state.taskSessionsByTask.itemsByTaskId[activeTask.id]?.map((session) => session.id) ??
      [activeSessionId].filter((sessionId): sessionId is string => Boolean(sessionId));
    return getHasActiveTaskSourceWork(sessionIds, state.turns.activeBySession);
  });
  const sourceStateLoading = useAppStore((state) =>
    Boolean(
      state.kanban.isLoading ||
      (workspaceId && state.repositories.loadingByWorkspaceId[workspaceId]) ||
      (activeTask && state.taskSessionsByTask.loadingByTaskId[activeTask.id]),
    ),
  );
  const addSourcesDisabledReason = getAddSourcesDisabledReason({
    isLoading: sourceStateLoading,
    hasActiveTurn: hasActiveTaskSourceWork,
  });
  return {
    activeTask,
    workspaceId,
    addSourcesOpen,
    setAddSourcesOpen,
    addSourcesOpener,
    setAddSourcesOpener,
    addSourcesButtonRef,
    addSourcesDisabledReason,
  };
}

const FilesPanel = memo(function FilesPanel({ onOpenFile }: FilesPanelProps) {
  // Use environment-stable sessionId so the file browser doesn't re-fetch
  // when switching between sessions in the same environment.
  const activeSessionId = useEnvironmentSessionId();
  const environmentId = useAppStore((state) => {
    if (!activeSessionId) return null;
    return (
      state.environmentIdBySessionId[activeSessionId] ??
      state.taskSessions.items[activeSessionId]?.task_environment_id ??
      null
    );
  });
  const activeFilePath = useDockviewStore((s) => s.activeFilePath);
  const isArchived = useIsTaskArchived();
  const sourceDialog = useFilesPanelSourceDialog(activeSessionId);
  const {
    activeTask,
    workspaceId,
    addSourcesOpen,
    setAddSourcesOpen,
    addSourcesOpener,
    setAddSourcesOpener,
    addSourcesButtonRef,
    addSourcesDisabledReason,
  } = sourceDialog;
  const hasRepository = Boolean(activeTask?.repositoryId ?? activeTask?.repositories?.length);
  const resolvedAddSourcesDisabledReason = hasRepository
    ? addSourcesDisabledReason
    : "This task needs a repository before sources can be added.";
  const { createFile, deleteFile, renameFile, downloadFile } = useFileOperations(
    activeSessionId ?? null,
  );

  const handleCreateFile = useCallback(
    async (path: string): Promise<boolean> => {
      const ok = await createFile(path);
      if (ok) {
        const name = path.split("/").pop() || path;
        const { calculateHash } = await import("@/lib/utils/file-diff");
        const hash = await calculateHash("");
        onOpenFile({
          path,
          name,
          content: "",
          originalContent: "",
          originalHash: hash,
          isDirty: false,
        });
      }
      return ok;
    },
    [createFile, onOpenFile],
  );

  if (isArchived) return <ArchivedPanelPlaceholder />;

  return (
    <PanelRoot data-testid="files-panel">
      <PanelBody padding={false}>
        {activeSessionId ? (
          <FileBrowser
            key={environmentId ?? "files"}
            sessionId={activeSessionId}
            environmentId={environmentId}
            onOpenFile={onOpenFile}
            onCreateFile={handleCreateFile}
            onDeleteFile={deleteFile}
            onRenameFile={renameFile}
            onDownloadFile={downloadFile}
            activeFilePath={activeFilePath}
            onAddSources={
              hasRepository
                ? (opener) => {
                    setAddSourcesOpener(opener);
                    setAddSourcesOpen(true);
                  }
                : undefined
            }
            addSourcesDisabledReason={resolvedAddSourcesDisabledReason}
            addSourcesButtonRef={addSourcesButtonRef}
          />
        ) : (
          <NoTaskSelected />
        )}
      </PanelBody>
      <TaskSourceDialog
        activeTask={activeTask}
        open={addSourcesOpen}
        onOpenChange={setAddSourcesOpen}
        workspaceId={workspaceId}
        opener={addSourcesOpener}
        openerRef={addSourcesButtonRef}
      />
    </PanelRoot>
  );
});

export { FilesPanel };
