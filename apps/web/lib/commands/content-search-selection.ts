import { scrollEditorIfMounted, setPendingCursorPosition } from "@/hooks/file-editor-cursor";
import { useDockviewStore } from "@/lib/state/dockview-store";
import type { WorkspaceContentSearchResult } from "@/lib/types/backend";
import { getFileName } from "@/lib/utils/file-path";

export function openContentSearchResult(
  result: WorkspaceContentSearchResult,
  worktreePath: string | null,
): void {
  const repo = result.repository_name || undefined;
  setPendingCursorPosition(result.path, result.line, result.column, repo);
  scrollEditorIfMounted(result.path, worktreePath, result.line, result.column, repo);
  useDockviewStore.getState().addFileEditorPanel(result.path, getFileName(result.path), { repo });
}
