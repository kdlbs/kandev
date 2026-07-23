"use client";

import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  useFileEditors,
  setPendingCursorPosition,
  scrollEditorIfMounted,
} from "@/hooks/use-file-editors";
import { lspClientManager } from "@/lib/lsp/lsp-client-manager";
import { documentUriForModel, filePathToUri, resolveFileUriInWorkspace } from "@/lib/lsp/file-uri";

/**
 * Connects LSP Go-to-Definition / Find References navigation to dockview file tabs.
 * When Monaco's registerEditorOpener fires with a file:// URI, this hook converts
 * the absolute path to a workspace-relative path and opens it via useFileEditors.
 */
export function useLspFileOpener() {
  const { openFile } = useFileEditors();

  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);

  const worktreePath = useAppStore((state) => {
    const sessionId = state.tasks.activeSessionId;
    if (!sessionId) return null;
    const session = state.taskSessions.items[sessionId];
    return session?.worktree_path ?? null;
  });

  useEffect(() => {
    const opener = async (uri: string, line?: number, column?: number) => {
      if (!activeSessionId) return;
      const documentUri = documentUriForModel(uri, activeSessionId);
      if (!documentUri) return;
      let fallbackWorkspaceUri: string | null = null;
      try {
        fallbackWorkspaceUri = worktreePath ? filePathToUri(worktreePath) : null;
      } catch {
        fallbackWorkspaceUri = null;
      }
      const workspaceUri =
        lspClientManager.getWorkspaceUriForSession(activeSessionId) ?? fallbackWorkspaceUri;
      if (!workspaceUri) return;
      const location = resolveFileUriInWorkspace(
        documentUri,
        workspaceUri,
        lspClientManager.getRepositorySubpaths(activeSessionId),
      );
      if (!location) return;

      // Dispose the placeholder model since a real tab will create its own model
      lspClientManager.disposePlaceholderModel(uri);

      // Set pending cursor position so the editor jumps to the correct line/column.
      // For new files: consumed by handleEditorDidMount when the editor mounts.
      // For already-open files: consumed by scrollEditorIfMounted below.
      if (line) {
        setPendingCursorPosition(location.path, line, column ?? 1, location.repo, activeSessionId);
      }

      await openFile(location.path, location.repo);

      // For already-open files, the editor is already mounted so handleEditorDidMount
      // won't fire. Scroll the editor directly.
      if (line) {
        scrollEditorIfMounted(location.path, workspaceUri, line, column ?? 1, {
          repo: location.repo,
          sessionId: activeSessionId,
        });
      }
    };

    lspClientManager.setFileOpener(opener);
    return () => {
      // Only clear if we're still the registered opener
      if (lspClientManager.getFileOpener() === opener) {
        lspClientManager.setFileOpener(null);
      }
    };
  }, [activeSessionId, openFile, worktreePath]);
}
