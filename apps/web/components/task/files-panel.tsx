"use client";

import { memo, useCallback } from "react";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { useAppStore } from "@/components/state-provider";
import { useFileOperations } from "@/hooks/use-file-operations";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { FileBrowser } from "@/components/task/file-browser";
import { useEnvironmentSessionId } from "@/hooks/use-environment-session-id";
import type { OpenFileTab } from "@/lib/types/backend";
import { useIsTaskArchived, ArchivedPanelPlaceholder } from "./task-archived-context";

type FilesPanelProps = {
  onOpenFile: (file: OpenFileTab) => void;
};

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
  const { createFile, deleteFile, renameFile } = useFileOperations(activeSessionId ?? null);

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
            activeFilePath={activeFilePath}
          />
        ) : (
          <div className="flex items-center justify-center h-full text-muted-foreground text-xs">
            No task selected
          </div>
        )}
      </PanelBody>
    </PanelRoot>
  );
});

export { FilesPanel };
