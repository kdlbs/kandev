import { setPendingCursorPosition, scrollEditorIfMounted } from "@/hooks/use-file-editors";

export type OpenFileFn = (path: string, repo?: string) => void | Promise<void>;

/**
 * Open a file in an editor tab and reveal/center the given line — the walkthrough
 * "pointer" to the line being explained. Uses the same pending-cursor mechanism
 * as LSP go-to-definition: the position is consumed when the editor mounts (new
 * tab), and scrolled live if the tab is already open.
 */
export function revealFileAtLine(
  openFile: OpenFileFn,
  file: string,
  line: number,
  repo?: string,
): void {
  if (line > 0) setPendingCursorPosition(file, line, 1);
  void openFile(file, repo);
  if (line > 0) scrollEditorIfMounted(file, null, line, 1);
}
