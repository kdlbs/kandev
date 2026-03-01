import { useCallback } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { createFile, deleteFile, renameFile } from "@/lib/ws/workspace-files";
import { useToast } from "@/components/toast-provider";

const ERROR_VARIANT = "error" as const;
const UNKNOWN_ERROR = "An unknown error occurred";

export function useFileOperations(sessionId: string | null) {
  const { toast } = useToast();

  const handleCreateFile = useCallback(
    async (path: string): Promise<boolean> => {
      const client = getWebSocketClient();
      if (!client || !sessionId) return false;

      try {
        const response = await createFile(client, sessionId, path);
        if (!response.success) {
          toast({
            title: "Failed to create file",
            description: response.error || UNKNOWN_ERROR,
            variant: ERROR_VARIANT,
          });
          return false;
        }
        return true;
      } catch (error) {
        toast({
          title: "Failed to create file",
          description: error instanceof Error ? error.message : UNKNOWN_ERROR,
          variant: ERROR_VARIANT,
        });
        return false;
      }
    },
    [sessionId, toast],
  );

  const handleDeleteFile = useCallback(
    async (path: string): Promise<boolean> => {
      const client = getWebSocketClient();
      if (!client || !sessionId) return false;

      try {
        const response = await deleteFile(client, sessionId, path);
        if (!response.success) {
          toast({
            title: "Failed to delete file",
            description: response.error || UNKNOWN_ERROR,
            variant: ERROR_VARIANT,
          });
          return false;
        }
        return true;
      } catch (error) {
        toast({
          title: "Failed to delete file",
          description: error instanceof Error ? error.message : UNKNOWN_ERROR,
          variant: ERROR_VARIANT,
        });
        return false;
      }
    },
    [sessionId, toast],
  );

  const handleRenameFile = useCallback(
    async (oldPath: string, newPath: string): Promise<boolean> => {
      const client = getWebSocketClient();
      if (!client || !sessionId) return false;

      try {
        const response = await renameFile(client, sessionId, oldPath, newPath);
        if (!response.success) {
          toast({
            title: "Failed to rename file",
            description: response.error || UNKNOWN_ERROR,
            variant: ERROR_VARIANT,
          });
          return false;
        }
        return true;
      } catch (error) {
        toast({
          title: "Failed to rename file",
          description: error instanceof Error ? error.message : UNKNOWN_ERROR,
          variant: ERROR_VARIANT,
        });
        return false;
      }
    },
    [sessionId, toast],
  );

  return {
    createFile: handleCreateFile,
    deleteFile: handleDeleteFile,
    renameFile: handleRenameFile,
  };
}
