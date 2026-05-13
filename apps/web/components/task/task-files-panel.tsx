"use client";

import { memo } from "react";
import { SessionPanel, SessionPanelContent } from "@kandev/ui/pannel-session";
import { FileBrowser } from "@/components/task/file-browser";
import type { OpenFileTab } from "@/lib/types/backend";
import { useFilesPanelData } from "./task-files-panel-hooks";

function FilesTabContent({
  sessionId,
  onOpenFile,
  handleCreateFile,
  hookDeleteFile,
  hookRenameFile,
  activeFilePath,
}: {
  sessionId: string | null;
  onOpenFile: (file: OpenFileTab) => void;
  handleCreateFile: (path: string) => Promise<boolean>;
  hookDeleteFile: (path: string) => Promise<boolean>;
  hookRenameFile: (oldPath: string, newPath: string) => Promise<boolean>;
  activeFilePath?: string | null;
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
      activeFilePath={activeFilePath}
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
  const { activeSessionId, hookDeleteFile, hookRenameFile, handleCreateFile } =
    useFilesPanelData(onOpenFile);

  return (
    <SessionPanel borderSide="left">
      <SessionPanelContent>
        <FilesTabContent
          sessionId={activeSessionId}
          onOpenFile={onOpenFile}
          handleCreateFile={handleCreateFile}
          hookDeleteFile={hookDeleteFile}
          hookRenameFile={hookRenameFile}
          activeFilePath={activeFilePath}
        />
      </SessionPanelContent>
    </SessionPanel>
  );
});

export { TaskFilesPanel };
