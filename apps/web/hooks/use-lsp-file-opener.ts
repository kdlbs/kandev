"use client";

import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  useFileEditors,
  setPendingCursorPosition,
  consumePendingCursorPosition,
} from "@/hooks/use-file-editors";
import { lspClientManager } from "@/lib/lsp/lsp-client-manager";
import { getMonacoInstance } from "@/components/editors/monaco/monaco-init";

/**
 * Try to scroll an already-mounted editor to the given position.
 * Returns true if the editor was found and scrolled, false otherwise.
 * If successful, consumes the pending cursor position so handleEditorDidMount
 * doesn't double-apply it.
 */
function scrollEditorIfMounted(
  relativePath: string,
  worktreePath: string | null,
  line: number,
  column: number,
): boolean {
  const monaco = getMonacoInstance();
  if (!monaco) return false;

  const monacoPath = worktreePath ? `${worktreePath}/${relativePath}` : relativePath;

  for (const editor of monaco.editor.getEditors()) {
    const model = editor.getModel();
    if (!model) continue;
    const modelPath = model.uri.path;
    if (modelPath === `/${monacoPath}` || modelPath === monacoPath) {
      consumePendingCursorPosition(relativePath);
      editor.setPosition({ lineNumber: line, column });
      editor.revealLineInCenter(line);
      editor.focus();
      return true;
    }
  }
  return false;
}

/**
 * Connects LSP Go-to-Definition / Find References navigation to dockview file tabs.
 * When Monaco's registerEditorOpener fires with a file:// URI, this hook converts
 * the absolute path to a workspace-relative path and opens it via useFileEditors.
 */
export function useLspFileOpener() {
  const { openFile } = useFileEditors();

  const worktreePath = useAppStore((state) => {
    const sessionId = state.tasks.activeSessionId;
    if (!sessionId) return null;
    const session = state.taskSessions.items[sessionId];
    return session?.worktree_path ?? null;
  });

  useEffect(() => {
    const opener = async (uri: string, line?: number, column?: number) => {
      // uri is like "file:///workspace/path/src/foo.ts"
      const filePath = uri.replace(/^file:\/\//, "");

      // Dispose the placeholder model since a real tab will create its own model
      lspClientManager.disposePlaceholderModel(uri);

      // Convert absolute path to workspace-relative path
      let relativePath = filePath;
      if (worktreePath && filePath.startsWith(worktreePath)) {
        relativePath = filePath.slice(worktreePath.length + 1); // +1 for the trailing /
      }

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
        scrollEditorIfMounted(relativePath, worktreePath, line, column ?? 1);
      }
    };

    lspClientManager.setFileOpener(opener);
    return () => {
      // Only clear if we're still the registered opener
      if (lspClientManager.getFileOpener() === opener) {
        lspClientManager.setFileOpener(null);
      }
    };
  }, [openFile, worktreePath]);
}
