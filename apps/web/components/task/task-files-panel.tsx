"use client";

import { memo, useRef, useState, type Ref } from "react";
import { SessionPanel, SessionPanelContent } from "@kandev/ui/pannel-session";
import { FileBrowser } from "@/components/task/file-browser";
import type { OpenFileTab } from "@/lib/types/backend";
import { useFilesPanelData } from "./task-files-panel-hooks";
import { useAppStore } from "@/components/state-provider";
import { AddWorkspaceSourcesDialog } from "./add-workspace-sources/add-workspace-sources-dialog";
import { ArchivedPanelPlaceholder, useIsTaskArchived } from "./task-archived-context";
import {
  getAddSourcesDisabledReason,
  hasActiveTaskSourceWork as getHasActiveTaskSourceWork,
} from "./add-workspace-sources/add-workspace-sources-availability";

function FilesTabContent({
  sessionId,
  onOpenFile,
  handleCreateFile,
  hookDeleteFile,
  hookRenameFile,
  hookDownloadFile,
  activeFilePath,
  onAddSources,
  addSourcesButtonRef,
  addSourcesDisabledReason,
}: {
  sessionId: string | null;
  onOpenFile: (file: OpenFileTab) => void;
  handleCreateFile: (path: string) => Promise<boolean>;
  hookDeleteFile: (path: string) => Promise<boolean>;
  hookRenameFile: (oldPath: string, newPath: string) => Promise<boolean>;
  hookDownloadFile: (path: string) => Promise<boolean>;
  activeFilePath?: string | null;
  onAddSources?: (opener: HTMLButtonElement) => void;
  addSourcesButtonRef?: Ref<HTMLButtonElement>;
  addSourcesDisabledReason?: string;
}) {
  if (!sessionId) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground text-xs">
        No task selected
      </div>
    );
  }
  return (
    <FileBrowser
      sessionId={sessionId}
      onOpenFile={onOpenFile}
      onCreateFile={handleCreateFile}
      onDeleteFile={hookDeleteFile}
      onRenameFile={hookRenameFile}
      onDownloadFile={hookDownloadFile}
      activeFilePath={activeFilePath}
      onAddSources={onAddSources}
      addSourcesButtonRef={addSourcesButtonRef}
      addSourcesDisabledReason={addSourcesDisabledReason}
    />
  );
}

type TaskFilesPanelProps = {
  onOpenFile: (file: OpenFileTab) => void;
  activeFilePath?: string | null;
};

const TaskFilesPanel = memo(function TaskFilesPanel({
  onOpenFile,
  activeFilePath,
}: TaskFilesPanelProps) {
  const [addSourcesOpen, setAddSourcesOpen] = useState(false);
  const [addSourcesOpener, setAddSourcesOpener] = useState<HTMLElement | null>(null);
  const addSourcesButtonRef = useRef<HTMLButtonElement>(null);
  const isArchived = useIsTaskArchived();
  const activeTask = useAppStore((state) =>
    state.kanban.tasks.find((task) => task.id === state.tasks.activeTaskId),
  );
  const { activeSessionId, hookDeleteFile, hookRenameFile, hookDownloadFile, handleCreateFile } =
    useFilesPanelData(onOpenFile);
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
  const hasRepository = Boolean(activeTask?.repositoryId || activeTask?.repositories?.length);
  const resolvedAddSourcesDisabledReason = hasRepository
    ? addSourcesDisabledReason
    : "This task needs a repository before sources can be added.";
  if (isArchived) return <ArchivedPanelPlaceholder />;
  return (
    <SessionPanel borderSide="left">
      <SessionPanelContent>
        <FilesTabContent
          sessionId={activeSessionId}
          onOpenFile={onOpenFile}
          handleCreateFile={handleCreateFile}
          hookDeleteFile={hookDeleteFile}
          hookRenameFile={hookRenameFile}
          hookDownloadFile={hookDownloadFile}
          activeFilePath={activeFilePath}
          onAddSources={
            hasRepository
              ? (opener) => {
                  setAddSourcesOpener(opener);
                  setAddSourcesOpen(true);
                }
              : undefined
          }
          addSourcesButtonRef={addSourcesButtonRef}
          addSourcesDisabledReason={resolvedAddSourcesDisabledReason}
        />
      </SessionPanelContent>
      {activeTask && hasRepository ? (
        <AddWorkspaceSourcesDialog
          open={addSourcesOpen}
          onOpenChange={setAddSourcesOpen}
          taskId={activeTask.id}
          executorType={activeTask.primaryExecutorType}
          workspaceId={workspaceId}
          opener={addSourcesOpener}
          openerRef={addSourcesButtonRef}
        />
      ) : null}
    </SessionPanel>
  );
});

export { TaskFilesPanel };
