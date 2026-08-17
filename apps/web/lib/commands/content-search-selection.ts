import { scrollEditorIfMounted, setPendingCursorPosition } from "@/hooks/file-editor-cursor";
import { useDockviewStore } from "@/lib/state/dockview-store";
import type { WorkspaceContentSearchResult } from "@/lib/types/backend";
import { getFileName } from "@/lib/utils/file-path";

export function openContentSearchResult(
  result: WorkspaceContentSearchResult,
  worktreePath: string | null,
  sessionId: string,
): void {
  const repo = result.repository_name || undefined;
  setPendingCursorPosition(result.path, result.line, result.column, repo, {
    sessionId,
    flashLine: true,
  });
  useDockviewStore.getState().addFileEditorPanel(result.path, getFileName(result.path), { repo });
  scrollEditorIfMounted(result.path, worktreePath, result.line, result.column, {
    repo,
    sessionId,
    flashLine: true,
  });
}
