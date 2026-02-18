import { useCallback, useEffect, useState, useRef, type RefObject } from 'react';
import type { editor as monacoEditor } from 'monaco-editor';
import { useAppStore } from '@/components/state-provider';
import { useLsp } from '@/hooks/use-lsp';
import { lspClientManager } from '@/lib/lsp/lsp-client-manager';
import { computeLineDiffStats } from '@/lib/diff';
import { useToast } from '@/components/toast-provider';
import { diffLines } from 'diff';

// ---------------------------------------------------------------------------
// Diff gutter decorations (pure function)
// ---------------------------------------------------------------------------

/** Compute gutter decorations for modified/added/deleted lines. */
export function computeDiffGutterDecorations(
  originalContent: string,
  currentContent: string,
): monacoEditor.IModelDeltaDecoration[] {
  const changes = diffLines(originalContent, currentContent);
  const decorations: monacoEditor.IModelDeltaDecoration[] = [];
  let currentLine = 1;

  for (let i = 0; i < changes.length; i++) {
    const change = changes[i];
    const lineCount = change.count ?? 0;

    if (change.removed) {
      const next = changes[i + 1];
      if (next?.added) {
        const addedLineCount = next.count ?? 0;
        for (let j = 0; j < addedLineCount; j++) {
          decorations.push({
            range: { startLineNumber: currentLine + j, startColumn: 1, endLineNumber: currentLine + j, endColumn: 1 },
            options: { isWholeLine: true, linesDecorationsClassName: 'monaco-diff-modified-gutter' },
          });
        }
        currentLine += addedLineCount;
        i++;
      } else {
        const indicatorLine = Math.max(1, currentLine - 1);
        decorations.push({
          range: { startLineNumber: indicatorLine, startColumn: 1, endLineNumber: indicatorLine, endColumn: 1 },
          options: { isWholeLine: true, linesDecorationsClassName: 'monaco-diff-deleted-gutter' },
        });
      }
    } else if (change.added) {
      for (let j = 0; j < lineCount; j++) {
        decorations.push({
          range: { startLineNumber: currentLine + j, startColumn: 1, endLineNumber: currentLine + j, endColumn: 1 },
          options: { isWholeLine: true, linesDecorationsClassName: 'monaco-diff-added-gutter' },
        });
      }
      currentLine += lineCount;
    } else {
      currentLine += lineCount;
    }
  }

  return decorations;
}

// ---------------------------------------------------------------------------
// useMonacoEditorLsp — LSP integration and toasts
// ---------------------------------------------------------------------------

type UseMonacoLspOpts = {
  sessionId?: string;
  worktreePath?: string;
  language: string;
  path: string;
  contentRef: RefObject<string>;
  editorRef: RefObject<monacoEditor.IStandaloneCodeEditor | null>;
};

export function useMonacoEditorLsp(opts: UseMonacoLspOpts) {
  const { sessionId, worktreePath, language, path, contentRef, editorRef } = opts;
  const { toast } = useToast();

  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const lspSessionId = sessionId ?? activeSessionId ?? null;
  const { status: lspStatus, lspLanguage, toggle: toggleLsp } = useLsp(lspSessionId, language);
  const hasLspActive = lspStatus.state === 'ready';
  const monacoPath = worktreePath ? `${worktreePath}/${path}` : path;
  const documentUri = `file://${monacoPath}`;

  // Open/close document
  useEffect(() => {
    if (!hasLspActive || !lspSessionId || !lspLanguage) return;
    lspClientManager.openDocument(lspSessionId, lspLanguage, documentUri, language, contentRef.current);
    return () => { lspClientManager.closeDocument(lspSessionId, lspLanguage, documentUri); };
  }, [hasLspActive, lspSessionId, lspLanguage, documentUri, language, contentRef]);

  // Document change sync
  const changeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => {
    const editor = editorRef.current;
    if (!editor || !hasLspActive || !lspSessionId || !lspLanguage) return;
    const model = editor.getModel();
    if (!model) return;
    const disposable = model.onDidChangeContent(() => {
      if (changeTimerRef.current) clearTimeout(changeTimerRef.current);
      changeTimerRef.current = setTimeout(() => {
        lspClientManager.changeDocument(lspSessionId, lspLanguage, documentUri, contentRef.current);
      }, 300);
    });
    return () => { if (changeTimerRef.current) clearTimeout(changeTimerRef.current); disposable.dispose(); };
  }, [hasLspActive, lspSessionId, lspLanguage, documentUri, contentRef, editorRef]);

  // LSP status toasts
  const lspStateForToast = lspStatus.state;
  const lspReasonForToast = 'reason' in lspStatus ? lspStatus.reason : null;
  useEffect(() => {
    if (lspStateForToast === 'installing') {
      toast({ title: 'Installing language server', description: 'This may take a moment...' });
    } else if (lspStateForToast === 'unavailable' && lspReasonForToast) {
      toast({ title: 'Language server not found', description: `${lspReasonForToast}. Enable auto-install in Settings \u2192 Editors.` });
    } else if (lspStateForToast === 'error' && lspReasonForToast) {
      toast({ title: 'LSP error', description: lspReasonForToast });
    }
  }, [lspStateForToast, lspReasonForToast, toast]);

  return { lspStatus, lspLanguage, toggleLsp, monacoPath };
}

// ---------------------------------------------------------------------------
// useMonacoDiffDecorations — diff gutter decorations + diff stats
// ---------------------------------------------------------------------------

type UseMonacoDiffDecorationsOpts = {
  originalContent: string;
  isDirty: boolean;
  showDiffIndicators: boolean;
  contentRef: RefObject<string>;
  editorRef: RefObject<monacoEditor.IStandaloneCodeEditor | null>;
  diffDecorationsRef: RefObject<monacoEditor.IEditorDecorationsCollection | null>;
};

export function useMonacoDiffDecorations(opts: UseMonacoDiffDecorationsOpts) {
  const { originalContent, isDirty, showDiffIndicators, contentRef, editorRef, diffDecorationsRef } = opts;

  const updateDiffDecorations = useCallback(() => {
    if (!diffDecorationsRef.current || !editorRef.current) return;
    if (!showDiffIndicators || !isDirty || !originalContent) {
      diffDecorationsRef.current.set([]);
      return;
    }
    diffDecorationsRef.current.set(computeDiffGutterDecorations(originalContent, contentRef.current));
  }, [originalContent, showDiffIndicators, isDirty, contentRef, editorRef, diffDecorationsRef]);

  const diffTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => {
    updateDiffDecorations();
    const editor = editorRef.current;
    if (!editor) return;
    const model = editor.getModel();
    if (!model) return;
    const disposable = model.onDidChangeContent(() => {
      if (diffTimerRef.current) clearTimeout(diffTimerRef.current);
      diffTimerRef.current = setTimeout(updateDiffDecorations, 150);
    });
    return () => { if (diffTimerRef.current) clearTimeout(diffTimerRef.current); disposable.dispose(); };
  }, [updateDiffDecorations, editorRef]);

  // Diff stats
  const [diffStats, setDiffStats] = useState<{ additions: number; deletions: number } | null>(null);
  const computeDiffStats = useCallback(() => {
    if (!isDirty) { setDiffStats(null); return; }
    setDiffStats(computeLineDiffStats(originalContent, contentRef.current));
  }, [isDirty, originalContent, contentRef]);

  const statsTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Compute on mount and when deps change (without calling setState directly in effect)
  useEffect(() => {
    // Schedule diff stats computation via microtask to avoid synchronous setState in effect
    const timer = setTimeout(computeDiffStats, 0);
    const editor = editorRef.current;
    if (!editor) return () => clearTimeout(timer);
    const model = editor.getModel();
    if (!model) return () => clearTimeout(timer);
    const disposable = model.onDidChangeContent(() => {
      if (statsTimerRef.current) clearTimeout(statsTimerRef.current);
      statsTimerRef.current = setTimeout(computeDiffStats, 300);
    });
    return () => {
      clearTimeout(timer);
      if (statsTimerRef.current) clearTimeout(statsTimerRef.current);
      disposable.dispose();
    };
  }, [computeDiffStats, editorRef]);

  return { diffStats };
}
