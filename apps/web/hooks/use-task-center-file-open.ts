"use client";

import { useCallback } from "react";
import type { OpenFileTab, FileContentResponse } from "@/lib/types/backend";
import { getWebSocketClient } from "@/lib/ws/connection";
import { requestFileContent } from "@/lib/ws/workspace-files";
import { calculateHash } from "@/lib/utils/file-diff";
import { t } from "@/lib/i18n";

type UseTaskCenterFileOpenOptions = {
  activeSessionId: string | null;
  addFileTab: (fileTab: OpenFileTab) => void;
  toast: (options: { title: string; description: string; variant: "error" }) => void;
};

export function useTaskCenterFileOpen({
  activeSessionId,
  addFileTab,
  toast,
}: UseTaskCenterFileOpenOptions) {
  return useCallback(
    async (filePath: string, repo?: string) => {
      const client = getWebSocketClient();
      if (!client || !activeSessionId) return;
      try {
        const response: FileContentResponse = await requestFileContent(
          client,
          activeSessionId,
          filePath,
          repo,
        );
        const fileName = filePath.split("/").pop() || filePath;
        const hash = await calculateHash(response.content);
        addFileTab({
          path: filePath,
          repo,
          name: fileName,
          content: response.content,
          originalContent: response.content,
          originalHash: hash,
          isDirty: false,
          isBinary: response.is_binary,
        });
      } catch (error) {
        toast({
          title: t("task:failedToOpenFile"),
          description: error instanceof Error ? error.message : t("task:unknownError"),
          variant: "error",
        });
      }
    },
    [activeSessionId, addFileTab, toast],
  );
}
