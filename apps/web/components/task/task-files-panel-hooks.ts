"use client";

import { useCallback, type RefObject } from "react";
import { useAppStore } from "@/components/state-provider";
import { useFileOperations } from "@/hooks/use-file-operations";
import type { RenameAdapter } from "@/hooks/file-rename-continuity";
import type { OpenFileTab } from "@/lib/types/backend";

export function useFilesPanelData(
  onOpenFile: (file: OpenFileTab) => void,
  adapter?: RefObject<RenameAdapter | null>,
) {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const {
    createFile: baseCreateFile,
    deleteFile: hookDeleteFile,
    renameFile: hookRenameFile,
    downloadFile: hookDownloadFile,
  } = useFileOperations(activeSessionId ?? null, { dockview: false, adapter });

  const handleCreateFile = useCallback(
    async (path: string): Promise<boolean> => {
      const ok = await baseCreateFile(path);
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
    [baseCreateFile, onOpenFile],
  );

  return {
    activeSessionId,
    hookDeleteFile,
    hookRenameFile,
    hookDownloadFile,
    handleCreateFile,
  };
}
