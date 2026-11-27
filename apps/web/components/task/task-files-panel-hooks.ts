"use client";

import { useCallback, type RefObject } from "react";
import { useAppStore } from "@/components/state-provider";
import { useFileOperations } from "@/hooks/use-file-operations";
import type { RenameAdapter } from "@/hooks/file-rename-continuity";
import type { OpenFileTab } from "@/lib/types/backend";
import { useTabletFileOpen } from "@/hooks/domains/session/use-tablet-file-admission";
import { bindTabletFileOpen } from "@/lib/state/tablet-file-authority";

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
  const beginOpen = useTabletFileOpen(activeSessionId);

  const handleCreateFile = useCallback(
    async (path: string): Promise<boolean> => {
      const operation = beginOpen({ path });
      if (!operation) return false;
      let delivered = false;
      try {
        const ok = await baseCreateFile(path);
        if (ok && operation.current()) {
          const name = path.split("/").pop() || path;
          const { calculateHash } = await import("@/lib/utils/file-diff");
          const hash = await calculateHash("");
          const file: OpenFileTab = {
            path,
            name,
            content: "",
            originalContent: "",
            originalHash: hash,
            isDirty: false,
          };
          if (operation.current(file)) {
            bindTabletFileOpen(file, operation);
            onOpenFile(file);
            delivered = true;
          }
        }
        return ok;
      } finally {
        if (!delivered) operation.finish();
      }
    },
    [baseCreateFile, onOpenFile, beginOpen],
  );

  return {
    activeSessionId,
    hookDeleteFile,
    hookRenameFile,
    hookDownloadFile,
    handleCreateFile,
  };
}
