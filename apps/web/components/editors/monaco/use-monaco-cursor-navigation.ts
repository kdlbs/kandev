import { useCallback, useEffect, useSyncExternalStore } from "react";
import type { editor as monacoEditor } from "monaco-editor";
import {
  clearMonacoCursorFlash,
  consumePendingCursorPosition,
  monacoEditorMatchesFile,
  revealMonacoEditor,
} from "@/hooks/file-editor-cursor";

type UseMonacoCursorNavigationOptions = {
  editor: monacoEditor.IStandaloneCodeEditor | null;
  path: string;
  worktreePath?: string;
  repo?: string;
  sessionId?: string;
};

function useMonacoModelSnapshot(editor: monacoEditor.IStandaloneCodeEditor | null): string {
  const subscribe = useCallback(
    (onStoreChange: () => void) => {
      if (!editor) return () => {};
      const subscription = editor.onDidChangeModel(() => {
        clearMonacoCursorFlash(editor);
        onStoreChange();
      });
      return () => subscription.dispose();
    },
    [editor],
  );
  const getSnapshot = useCallback(() => {
    const model = editor?.getModel();
    return model ? `${model.id}:${model.uri.toString()}` : "";
  }, [editor]);
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}

export function useMonacoCursorNavigation({
  editor,
  path,
  worktreePath,
  repo,
  sessionId,
}: UseMonacoCursorNavigationOptions): void {
  const modelSnapshot = useMonacoModelSnapshot(editor);

  useEffect(() => {
    if (
      !editor ||
      !monacoEditorMatchesFile(editor, path, worktreePath ?? null, { repo, sessionId })
    ) {
      return;
    }
    const pending = consumePendingCursorPosition(path, repo, sessionId);
    if (pending) revealMonacoEditor(editor, pending);
  }, [editor, modelSnapshot, path, repo, sessionId, worktreePath]);

  useEffect(() => {
    if (!editor) return;
    return () => clearMonacoCursorFlash(editor);
  }, [editor]);
}
