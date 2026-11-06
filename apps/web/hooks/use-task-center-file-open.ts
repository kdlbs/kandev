"use client";

import { useAppStoreApi } from "@/components/state-provider";
import { isRenameReserved, trackRenameOpen } from "./file-rename-continuity";
import { useCallback } from "react";
import type { OpenFileTab, FileContentResponse } from "@/lib/types/backend";
import { getWebSocketClient } from "@/lib/ws/connection";
import { requestFileContent } from "@/lib/ws/workspace-files";
import { calculateHash } from "@/lib/utils/file-diff";
import { t } from "@/lib/i18n";

type UseTaskCenterFileOpenOptions = {
  activeSessionId: string | null;
  addFileTab: (fileTab: OpenFileTab) => void;
  toast: (options: { title: string; description?: string; variant: "error" }) => void;
};

export function useTaskCenterFileOpen({
  activeSessionId,
  addFileTab,
  toast,
}: UseTaskCenterFileOpenOptions) {
  const store = useAppStoreApi();
  return useCallback(
    async (filePath: string, repo?: string) => {
      const client = getWebSocketClient();
      if (!client || !activeSessionId) return;
      if (isRenameReserved(store, activeSessionId, filePath, repo)) {
        toast({
          title: t("task:failedToOpenFile"),
          variant: "error",
        });
        return;
      }
      const pending = trackRenameOpen(store, activeSessionId, filePath, repo);
      try {
        const response: FileContentResponse = await requestFileContent(
          client,
          activeSessionId,
          filePath,
          repo,
        );
        const fileName = filePath.split("/").pop() || filePath;
        const hash = await calculateHash(response.content);
        if (!pending.current() || isRenameReserved(store, activeSessionId, filePath, repo)) return;
        addFileTab({
          path: filePath,
          repo,
          name: fileName,
          content: response.content,
          originalContent: response.content,
          originalHash: hash,
          isDirty: false,
          isBinary: response.is_binary,
          resolvedPath: response.resolved_path,
        });
      } catch (error) {
        toast({
          title: t("task:failedToOpenFile"),
          description: error instanceof Error ? error.message : t("task:unknownError"),
          variant: "error",
        });
      } finally {
        pending.dispose();
      }
    },
    [activeSessionId, addFileTab, toast, store],
  );
}
