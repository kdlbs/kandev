import { useCallback } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { WebSocketClient } from "@/lib/ws/client";
import { createFile, deleteFile, renameFile, requestFileContent } from "@/lib/ws/workspace-files";
import { triggerFileDownload } from "@/lib/utils/file-download";
import { useToast } from "@/components/toast-provider";

type ToastFn = ReturnType<typeof useToast>["toast"];

const ERROR_VARIANT = "error" as const;
const UNKNOWN_ERROR = "An unknown error occurred";

type DownloadResult = { ok: true } | { ok: false; error?: string };

/**
 * Fetch a file from the workspace and trigger a browser download.
 * Extracted from the hook so it can be unit-tested without React.
 */
export async function downloadFileContent(
  client: WebSocketClient,
  sessionId: string,
  path: string,
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
    return { ok: false, error: error instanceof Error ? error.message : UNKNOWN_ERROR };
  }
}

async function runFileOp<T extends { success: boolean; error?: string }>(
  op: () => Promise<T>,
  title: string,
  toast: ToastFn,
): Promise<boolean> {
  try {
    const response = await op();
    if (!response.success) {
      toast({ title, description: response.error || UNKNOWN_ERROR, variant: ERROR_VARIANT });
      return false;
    }
    return true;
  } catch (error) {
    const description = error instanceof Error ? error.message : UNKNOWN_ERROR;
    toast({ title, description, variant: ERROR_VARIANT });
    return false;
  }
}

export function useFileOperations(sessionId: string | null) {
  const { toast } = useToast();

  const handleCreateFile = useCallback(
    async (path: string): Promise<boolean> => {
      const client = getWebSocketClient();
      if (!client || !sessionId) return false;
      return runFileOp(() => createFile(client, sessionId, path), "Failed to create file", toast);
    },
    [sessionId, toast],
  );

  const handleDeleteFile = useCallback(
    async (path: string): Promise<boolean> => {
      const client = getWebSocketClient();
      if (!client || !sessionId) return false;
      return runFileOp(() => deleteFile(client, sessionId, path), "Failed to delete item", toast);
    },
    [sessionId, toast],
  );

  const handleRenameFile = useCallback(
    async (oldPath: string, newPath: string): Promise<boolean> => {
      const client = getWebSocketClient();
      if (!client || !sessionId) return false;
      return runFileOp(
        () => renameFile(client, sessionId, oldPath, newPath),
        "Failed to rename item",
        toast,
      );
    },
    [sessionId, toast],
  );

  const handleDownloadFile = useCallback(
    async (path: string): Promise<boolean> => {
      const client = getWebSocketClient();
      if (!client || !sessionId) return false;
      const result = await downloadFileContent(client, sessionId, path);
      if (!result.ok) {
        toast({
          title: "Failed to download file",
          description: result.error || UNKNOWN_ERROR,
          variant: ERROR_VARIANT,
        });
        return false;
      }
      return true;
    },
    [sessionId, toast],
  );

  return {
    createFile: handleCreateFile,
    deleteFile: handleDeleteFile,
    renameFile: handleRenameFile,
    downloadFile: handleDownloadFile,
  };
}
