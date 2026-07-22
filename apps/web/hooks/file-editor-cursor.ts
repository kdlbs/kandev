import { getMonacoInstance } from "@/components/editors/monaco/monaco-init";
import { walkthroughFileMatches } from "@/lib/diff/walkthrough-match";
import { buildRepoScopedItemId } from "@/lib/state/dockview-panel-actions";
import {
  canonicalFileUri,
  documentUriForModel,
  filePathToUri,
  fileUrisEqual,
  isSessionModelUri,
  joinFileUri,
} from "@/lib/lsp/file-uri";

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

type EditorFileScope = { repo?: string; sessionId?: string };

type ModelMatchContext = EditorFileScope & {
  targetUri: string | null;
  monacoPath: string;
  path: string;
};

function editorModelMatchesTarget(
  model: { uri: { path: string; toString(): string } },
  context: ModelMatchContext,
): boolean {
  const { targetUri, sessionId, monacoPath, path, repo } = context;
  if (targetUri && sessionId) {
    const modelDocumentUri = documentUriForModel(model.uri.toString(), sessionId);
    return modelDocumentUri !== null && fileUrisEqual(modelDocumentUri, targetUri);
  }

  const modelUri = canonicalFileUri(model.uri.toString());
  if (targetUri && modelUri && !isSessionModelUri(model.uri.toString())) {
    return fileUrisEqual(modelUri, targetUri);
  }
  return editorModelMatches(model.uri.path, monacoPath, path, repo);
}

export function scrollEditorIfMounted(
  path: string,
  worktreePath: string | null,
  line: number,
  column: number,
  scope: EditorFileScope = {},
): boolean {
  const { repo } = scope;
  const monaco = getMonacoInstance();
  if (!monaco) return false;

  let targetUri: string | null = null;
  if (worktreePath) {
    try {
      const workspaceUri = canonicalFileUri(worktreePath) ?? filePathToUri(worktreePath);
      targetUri = joinFileUri(workspaceUri, repo, path);
    } catch {
      targetUri = null;
    }
  }
  const monacoPath = worktreePath ? `${worktreePath}/${repo ? `${repo}/` : ""}${path}` : path;
  for (const editor of monaco.editor.getEditors()) {
    const model = editor.getModel();
    if (!model) continue;
    if (editorModelMatchesTarget(model, { targetUri, monacoPath, path, ...scope })) {
      consumePendingCursorPosition(path, repo);
      editor.setPosition({ lineNumber: line, column });
      editor.revealLineInCenter(line);
      editor.focus();
      return true;
    }
  }
  return false;
}
