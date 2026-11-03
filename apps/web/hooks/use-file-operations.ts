import { useCallback, type RefObject } from "react";
import type { RenameAdapter } from "./file-rename-continuity";
import { useFileRename } from "./use-file-rename";
import { runFileOp } from "./file-operation-utils";
import { useTranslation } from "react-i18next";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { WebSocketClient } from "@/lib/ws/client";
import { createFile, deleteFile, requestFileContent } from "@/lib/ws/workspace-files";
import { triggerFileDownload } from "@/lib/utils/file-download";
import { useToast } from "@/components/toast-provider";

const ERROR_VARIANT = "error" as const;

type DownloadResult = { ok: true } | { ok: false; error?: string };

/**
 * Fetch a file from the workspace and trigger a browser download.
 * Extracted from the hook so it can be unit-tested without React.
 */
export async function downloadFileContent(
  client: WebSocketClient,
  sessionId: string,
  path: string,
  unknownError: string,
): Promise<DownloadResult> {
  try {
    const response = await requestFileContent(client, sessionId, path);
    if (response.error) return { ok: false, error: response.error };
    triggerFileDownload({
      fileName: path,
      content: response.content,
      isBinary: !!response.is_binary,
    });
    return { ok: true };
  } catch (error) {
    return { ok: false, error: error instanceof Error ? error.message : unknownError };
  }
}

export function useFileOperations(
  sessionId: string | null,
  options?: { dockview?: boolean; adapter?: RefObject<RenameAdapter | null> },
) {
  const { toast } = useToast();
  const { t } = useTranslation("task");
  const unknownError = t("common:anUnknownErrorOccurred");

  const handleCreateFile = useCallback(
    async (path: string): Promise<boolean> => {
      const client = getWebSocketClient();
      if (!client || !sessionId) return false;
      return runFileOp(
        () => createFile(client, sessionId, path),
        t("task:failedToCreateFile"),
        unknownError,
        toast,
      );
    },
    [sessionId, t, toast, unknownError],
  );

  const handleDeleteFile = useCallback(
    async (path: string): Promise<boolean> => {
      const client = getWebSocketClient();
      if (!client || !sessionId) return false;
      return runFileOp(
        () => deleteFile(client, sessionId, path),
        t("task:failedToDeleteItem"),
        unknownError,
        toast,
      );
    },
    [sessionId, t, toast, unknownError],
  );

  const handleRenameFile = useFileRename(sessionId, options);

  const handleDownloadFile = useCallback(
    async (path: string): Promise<boolean> => {
      const client = getWebSocketClient();
      if (!client || !sessionId) return false;
      const result = await downloadFileContent(client, sessionId, path, unknownError);
      if (!result.ok) {
        toast({
          title: t("task:failedToDownloadFile"),
          description: result.error || unknownError,
          variant: ERROR_VARIANT,
        });
        return false;
      }
      return true;
    },
    [sessionId, t, toast, unknownError],
  );

  return {
    createFile: handleCreateFile,
    deleteFile: handleDeleteFile,
    renameFile: handleRenameFile,
    downloadFile: handleDownloadFile,
  };
}
