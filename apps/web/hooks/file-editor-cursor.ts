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
  parseSessionModelUri,
} from "@/lib/lsp/file-uri";

export type CursorPosition = { line: number; column: number; flashLine?: boolean };
export type CursorRevealOptions = { sessionId?: string; flashLine?: boolean };
type CodeMirrorCursorRevealer = (
  line: number,
  column: number,
  options: CursorRevealOptions,
) => boolean;
type MonacoInstance = NonNullable<ReturnType<typeof getMonacoInstance>>;
type MountedMonacoEditor = ReturnType<MonacoInstance["editor"]["getEditors"]>[number];
type MonacoCursorFlash = {
  collection: ReturnType<MountedMonacoEditor["createDecorationsCollection"]>;
  timeout: ReturnType<typeof setTimeout>;
};

export const EDITOR_CONTENT_SEARCH_FLASH_CLASS = "editor-content-search-flash";
export const EDITOR_CONTENT_SEARCH_FLASH_DURATION_MS = 1_200;

const pendingCursorPositions = new Map<string, CursorPosition>();
const monacoCursorFlashes = new WeakMap<MountedMonacoEditor, MonacoCursorFlash>();
const codeMirrorCursorRevealers = new Map<
  string,
  Map<string | undefined, Set<CodeMirrorCursorRevealer>>
>();

function pendingCursorKey(path: string, repo?: string, sessionId?: string): string {
  const fileKey = buildRepoScopedItemId(path, repo);
  return sessionId === undefined ? fileKey : JSON.stringify([sessionId, fileKey]);
}

function takePendingCursor(key: string): CursorPosition | undefined {
  const position = pendingCursorPositions.get(key);
  if (position) pendingCursorPositions.delete(key);
  return position;
}

export function setPendingCursorPosition(
  path: string,
  line: number,
  column: number,
  repo?: string,
  sessionIdOrOptions?: string | CursorRevealOptions,
) {
  const options =
    typeof sessionIdOrOptions === "string"
      ? { sessionId: sessionIdOrOptions }
      : (sessionIdOrOptions ?? {});
  pendingCursorPositions.set(pendingCursorKey(path, repo, options.sessionId), {
    line,
    column,
    ...(options.flashLine ? { flashLine: true } : {}),
  });
}

export function consumePendingCursorPosition(
  path: string,
  repo?: string,
  sessionId?: string,
): CursorPosition | undefined {
  const scopedPosition = takePendingCursor(pendingCursorKey(path, repo, sessionId));
  if (scopedPosition || sessionId === undefined) return scopedPosition;
  return takePendingCursor(pendingCursorKey(path, repo));
}

export function registerCodeMirrorCursorRevealer(
  path: string,
  repo: string | undefined,
  sessionId: string | undefined,
  reveal: CodeMirrorCursorRevealer,
): () => void {
  const fileKey = buildRepoScopedItemId(path, repo);
  const revealersBySession = codeMirrorCursorRevealers.get(fileKey) ?? new Map();
  const revealers = revealersBySession.get(sessionId) ?? new Set();
  revealers.add(reveal);
  revealersBySession.set(sessionId, revealers);
  codeMirrorCursorRevealers.set(fileKey, revealersBySession);

  return () => {
    revealers.delete(reveal);
    if (revealers.size === 0) revealersBySession.delete(sessionId);
    if (revealersBySession.size === 0) codeMirrorCursorRevealers.delete(fileKey);
  };
}

function runCodeMirrorRevealers(
  revealers: Set<CodeMirrorCursorRevealer> | undefined,
  target: CursorPosition,
): boolean {
  if (!revealers) return false;
  for (const reveal of [...revealers].reverse()) {
    if (reveal(target.line, target.column, { flashLine: target.flashLine })) return true;
  }
  return false;
}

function revealMountedCodeMirror(
  path: string,
  repo: string | undefined,
  sessionId: string | undefined,
  target: CursorPosition,
): boolean {
  const revealersBySession = codeMirrorCursorRevealers.get(buildRepoScopedItemId(path, repo));
  if (!revealersBySession) return false;
  if (runCodeMirrorRevealers(revealersBySession.get(sessionId), target)) {
    consumePendingCursorPosition(path, repo, sessionId);
    return true;
  }
  if (sessionId !== undefined) return false;

  const scopedRevealerSets = [...revealersBySession.entries()]
    .filter(([registeredSessionId]) => registeredSessionId !== undefined)
    .map(([, revealers]) => revealers);
  if (scopedRevealerSets.length !== 1 || !runCodeMirrorRevealers(scopedRevealerSets[0], target)) {
    return false;
  }
  consumePendingCursorPosition(path, repo);
  return true;
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

export type EditorFileScope = { repo?: string } & CursorRevealOptions;

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
  const modelUriText = model.uri.toString();
  if (sessionId) {
    const modelDocumentUri = documentUriForModel(modelUriText, sessionId);
    return modelDocumentUri !== null && sessionModelMatchesTarget(modelDocumentUri, context);
  }

  if (isSessionModelUri(modelUriText)) return false;
  const modelUri = canonicalFileUri(modelUriText);
  if (targetUri && modelUri) {
    return fileUrisEqual(modelUri, targetUri);
  }
  return editorModelMatches(model.uri.path, monacoPath, path, repo);
}

function decodedDocumentPath(documentUri: string): string | null {
  try {
    return new URL(documentUri).pathname
      .split("/")
      .map((segment) => decodeURIComponent(segment))
      .join("/");
  } catch {
    return null;
  }
}

function sessionModelMatchesTarget(documentUri: string, context: ModelMatchContext): boolean {
  if (context.targetUri) return fileUrisEqual(documentUri, context.targetUri);
  const modelPath = decodedDocumentPath(documentUri);
  return (
    modelPath !== null &&
    editorModelMatches(modelPath, context.monacoPath, context.path, context.repo)
  );
}

export function clearMonacoCursorFlash(editor: MountedMonacoEditor): void {
  const flash = monacoCursorFlashes.get(editor);
  if (!flash) return;
  clearTimeout(flash.timeout);
  flash.collection.set([]);
  monacoCursorFlashes.delete(editor);
}

function flashMonacoEditorLine(editor: MountedMonacoEditor, line: number): void {
  clearMonacoCursorFlash(editor);
  const collection = editor.createDecorationsCollection([
    {
      range: { startLineNumber: line, startColumn: 1, endLineNumber: line, endColumn: 1 },
      options: {
        isWholeLine: true,
        className: EDITOR_CONTENT_SEARCH_FLASH_CLASS,
      },
    },
  ]);
  const flash: MonacoCursorFlash = {
    collection,
    timeout: setTimeout(() => {
      if (monacoCursorFlashes.get(editor) !== flash) return;
      collection.set([]);
      monacoCursorFlashes.delete(editor);
    }, EDITOR_CONTENT_SEARCH_FLASH_DURATION_MS),
  };
  monacoCursorFlashes.set(editor, flash);
}

export function revealMonacoEditor(editor: MountedMonacoEditor, target: CursorPosition): void {
  // A fresh Dockview panel can mount Monaco before automaticLayout has measured
  // the newly active container. Centering against that stale zero-size layout
  // updates the cursor but leaves the viewport at the top of the file.
  if (target.flashLine) editor.layout();
  editor.setPosition({ lineNumber: target.line, column: target.column });
  editor.revealLineInCenter(target.line);
  if (target.flashLine) flashMonacoEditorLine(editor, target.line);
  editor.focus();
}

function targetUriForPath(
  worktreePath: string | null,
  repo: string | undefined,
  path: string,
): string | null {
  if (!worktreePath) return null;
  try {
    const workspaceUri = canonicalFileUri(worktreePath) ?? filePathToUri(worktreePath);
    return joinFileUri(workspaceUri, repo, path);
  } catch {
    return null;
  }
}

function modelMatchContext(
  path: string,
  worktreePath: string | null,
  scope: EditorFileScope,
): ModelMatchContext {
  const { repo } = scope;
  return {
    targetUri: targetUriForPath(worktreePath, repo, path),
    monacoPath: worktreePath ? `${worktreePath}/${repo ? `${repo}/` : ""}${path}` : path,
    path,
    ...scope,
  };
}

export function monacoEditorMatchesFile(
  editor: MountedMonacoEditor,
  path: string,
  worktreePath: string | null,
  scope: EditorFileScope = {},
): boolean {
  const model = editor.getModel();
  return (
    model !== null && editorModelMatchesTarget(model, modelMatchContext(path, worktreePath, scope))
  );
}

function findMountedMonacoEditor(
  editors: MountedMonacoEditor[],
  context: ModelMatchContext,
): MountedMonacoEditor | null {
  const sessionCandidates = new Map<string, MountedMonacoEditor>();
  for (const editor of editors) {
    const model = editor.getModel();
    if (!model) continue;
    if (context.sessionId === undefined) {
      const identity = parseSessionModelUri(model.uri.toString());
      if (identity && sessionModelMatchesTarget(identity.documentUri, context)) {
        sessionCandidates.set(identity.sessionId, editor);
        continue;
      }
    }
    if (editorModelMatchesTarget(model, context)) return editor;
  }
  return sessionCandidates.size === 1 ? (sessionCandidates.values().next().value ?? null) : null;
}

function revealMountedMonaco(
  path: string,
  worktreePath: string | null,
  line: number,
  column: number,
  scope: EditorFileScope,
): boolean {
  const monaco = getMonacoInstance();
  if (!monaco) return false;
  const { repo } = scope;
  const context = modelMatchContext(path, worktreePath, scope);
  const editor = findMountedMonacoEditor(monaco.editor.getEditors(), context);
  if (!editor) return false;
  consumePendingCursorPosition(path, repo, scope.sessionId);
  revealMonacoEditor(editor, { line, column, flashLine: scope.flashLine });
  return true;
}

export function scrollEditorIfMounted(
  path: string,
  worktreePath: string | null,
  line: number,
  column: number,
  scope: EditorFileScope = {},
): boolean {
  if (revealMountedMonaco(path, worktreePath, line, column, scope)) return true;
  return revealMountedCodeMirror(path, scope.repo, scope.sessionId, {
    line,
    column,
    flashLine: scope.flashLine,
  });
}
