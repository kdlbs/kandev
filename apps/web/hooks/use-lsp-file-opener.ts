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
import {
  canonicalFileUri,
  documentUriForModel,
  filePathToUri,
  isSessionModelUri,
  resolveFileUriInWorkspace,
  type WorkspaceFileLocation,
} from "@/lib/lsp/file-uri";
import { useDockviewStore } from "@/lib/state/dockview-store";

export function toWorkspaceRelativePath(filePath: string, workspacePath: string | null): string {
  const normalizedWorkspacePath = workspacePath?.replace(/\/+$/, "") ?? "";
  if (!normalizedWorkspacePath) return filePath;
  const workspacePrefix = `${normalizedWorkspacePath}/`;
  return filePath.startsWith(workspacePrefix) ? filePath.slice(workspacePrefix.length) : filePath;
}

export function preserveExistingEditorLocation(
  location: WorkspaceFileLocation,
  openFiles: ReadonlyMap<string, unknown>,
): WorkspaceFileLocation {
  if (!location.repo) return location;
  const taskRootPath = `${location.repo}/${location.path}`;
  return openFiles.has(taskRootPath) ? { path: taskRootPath } : location;
}

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
    return getSessionWorkspacePath(session) ?? null;
  });

  useEffect(() => {
    const opener = async (uri: string, line?: number, column?: number) => {
      if (!activeSessionId) return false;
      const documentUri =
        documentUriForModel(uri, activeSessionId) ??
        (isSessionModelUri(uri) ? null : canonicalFileUri(uri));
      if (!documentUri) return false;
      let fallbackWorkspaceUri: string | null = null;
      try {
        fallbackWorkspaceUri = worktreePath ? filePathToUri(worktreePath) : null;
      } catch {
        fallbackWorkspaceUri = null;
      }
      const workspaceUri =
        lspClientManager.getWorkspaceUriForSession(activeSessionId) ?? fallbackWorkspaceUri;
      if (!workspaceUri) return false;
      const resolvedLocation = resolveFileUriInWorkspace(
        documentUri,
        workspaceUri,
        lspClientManager.getRepositorySubpaths(activeSessionId),
      );
      if (!resolvedLocation) return false;
      const location = preserveExistingEditorLocation(
        resolvedLocation,
        useDockviewStore.getState().openFiles,
      );

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
      return true;
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
