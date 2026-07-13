import { getMonacoInstance } from "@/components/editors/monaco/monaco-init";
import { walkthroughFileMatches } from "@/lib/diff/walkthrough-match";
import { buildRepoScopedItemId } from "@/lib/state/dockview-panel-actions";

const pendingCursorPositions = new Map<string, { line: number; column: number }>();

function pendingCursorKey(path: string, repo?: string): string {
  return buildRepoScopedItemId(path, repo);
}

export function setPendingCursorPosition(
  path: string,
  line: number,
  column: number,
  repo?: string,
) {
  pendingCursorPositions.set(pendingCursorKey(path, repo), { line, column });
}

export function consumePendingCursorPosition(
  path: string,
  repo?: string,
): { line: number; column: number } | undefined {
  const key = pendingCursorKey(path, repo);
  const pos = pendingCursorPositions.get(key);
  if (pos) pendingCursorPositions.delete(key);
  return pos;
}

function pathSegments(path: string): string[] {
  return path.trim().replaceAll("\\", "/").split("/").filter(Boolean);
}

function repoScopedModelMatches(modelPath: string, repo: string | undefined, path: string) {
  const repoSegments = pathSegments(repo ?? "");
  if (repoSegments.length === 0) return false;
  const modelSegments = pathSegments(modelPath);
  const targetSegments = [...repoSegments, ...pathSegments(path)];
  if (targetSegments.length > modelSegments.length) return false;
  const offset = modelSegments.length - targetSegments.length;
  return targetSegments.every((segment, index) => modelSegments[offset + index] === segment);
}

function editorModelMatches(modelPath: string, monacoPath: string, path: string, repo?: string) {
  const exactMatch = modelPath === `/${monacoPath}` || modelPath === monacoPath;
  if (repo) return repoScopedModelMatches(modelPath, repo, path);
  return exactMatch || walkthroughFileMatches(modelPath, path);
}

export function scrollEditorIfMounted(
  path: string,
  worktreePath: string | null,
  line: number,
  column: number,
  repo?: string,
): boolean {
  const monaco = getMonacoInstance();
  if (!monaco) return false;

  const monacoPath = worktreePath ? `${worktreePath}/${path}` : path;
  for (const editor of monaco.editor.getEditors()) {
    const model = editor.getModel();
    if (!model) continue;
    const modelPath = model.uri.path;
    const matches = worktreePath
      ? modelPath === `/${monacoPath}` || modelPath === monacoPath
      : editorModelMatches(modelPath, monacoPath, path, repo);
    if (matches) {
      consumePendingCursorPosition(path, repo);
      editor.setPosition({ lineNumber: line, column });
      editor.revealLineInCenter(line);
      editor.focus();
      return true;
    }
  }
  return false;
}
