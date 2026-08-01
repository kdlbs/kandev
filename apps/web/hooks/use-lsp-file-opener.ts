"use client";

import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { getSessionWorkspacePath } from "@/lib/session-workspace-path";
import {
  useFileEditors,
  setPendingCursorPosition,
  scrollEditorIfMounted,
} from "@/hooks/use-file-editors";
import { lspClientManager } from "@/lib/lsp/lsp-client-manager";

export function toWorkspaceRelativePath(filePath: string, workspacePath: string | null): string {
  if (workspacePath && filePath.startsWith(workspacePath)) {
    return filePath.slice(workspacePath.length + 1);
  }
  return filePath;
}

/**
 * Connects LSP Go-to-Definition / Find References navigation to dockview file tabs.
 * When Monaco's registerEditorOpener fires with a file:// URI, this hook converts
 * the absolute path to a workspace-relative path and opens it via useFileEditors.
 */
export function useLspFileOpener() {
  const { openFile } = useFileEditors();

  const workspacePath = useAppStore((state) => {
    const sessionId = state.tasks.activeSessionId;
    if (!sessionId) return null;
    const session = state.taskSessions.items[sessionId];
    return getSessionWorkspacePath(session) ?? null;
  });

  useEffect(() => {
    const opener = async (uri: string, line?: number, column?: number) => {
      // uri is like "file:///workspace/path/src/foo.ts"
      const filePath = uri.replace(/^file:\/\//, "");

      // Dispose the placeholder model since a real tab will create its own model
      lspClientManager.disposePlaceholderModel(uri);

      // Convert absolute path to workspace-relative path
      const relativePath = toWorkspaceRelativePath(filePath, workspacePath);

      // Set pending cursor position so the editor jumps to the correct line/column.
      // For new files: consumed by handleEditorDidMount when the editor mounts.
      // For already-open files: consumed by scrollEditorIfMounted below.
      if (line) {
        setPendingCursorPosition(relativePath, line, column ?? 1);
      }

      await openFile(relativePath);

      // For already-open files, the editor is already mounted so handleEditorDidMount
      // won't fire. Scroll the editor directly.
      if (line) {
        scrollEditorIfMounted(relativePath, workspacePath, line, column ?? 1);
      }
    };

    lspClientManager.setFileOpener(opener);
    return () => {
      // Only clear if we're still the registered opener
      if (lspClientManager.getFileOpener() === opener) {
        lspClientManager.setFileOpener(null);
      }
    };
  }, [openFile, workspacePath]);
}
