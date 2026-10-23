import { useEffect, useRef } from "react";
import type { editor, IPosition, ISelection, IRange } from "monaco-editor";
import { monacoEditorMatchesFile } from "@/hooks/file-editor-cursor";
import {
  clampSourcePosition,
  sourceLineAtOffset,
  sourceOffsetAtLine,
} from "@/components/task/markdown-source-position";
import type { SourcePositionHandle, SourcePositionProps } from "../source-position";

const IMMEDIATE_SCROLL: editor.ScrollType = 1;

type MonacoSourcePositionOptions = SourcePositionProps & {
  editor: editor.IStandaloneCodeEditor | null;
  content: string;
  identity?: string;
  path?: string;
  worktreePath?: string;
  repo?: string;
  sessionId?: string;
};

function selectionRange(anchor: IPosition, active: IPosition): ISelection & IRange {
  const forward =
    anchor.lineNumber < active.lineNumber ||
    (anchor.lineNumber === active.lineNumber && anchor.column <= active.column);
  const start = forward ? anchor : active;
  const end = forward ? active : anchor;
  return {
    selectionStartLineNumber: anchor.lineNumber,
    selectionStartColumn: anchor.column,
    positionLineNumber: active.lineNumber,
    positionColumn: active.column,
    startLineNumber: start.lineNumber,
    startColumn: start.column,
    endLineNumber: end.lineNumber,
    endColumn: end.column,
  };
}

function createHandle(
  instance: editor.IStandaloneCodeEditor,
  model: editor.ITextModel,
  readContent: () => string,
  isLive: () => boolean,
): SourcePositionHandle {
  const sourceOffset = (position: IPosition) => {
    const native = model.getPositionAt(model.getOffsetAt(position));
    return sourceOffsetAtLine(readContent(), native.lineNumber, native.column);
  };
  const nativePosition = (offset: number) => {
    const lineNumber = sourceLineAtOffset(readContent(), offset);
    const column = offset - sourceOffsetAtLine(readContent(), lineNumber) + 1;
    return model.getPositionAt(model.getOffsetAt({ lineNumber, column }));
  };
  return {
    capture: () => {
      if (!isLive()) return null;
      const selection = instance.getSelection();
      if (!selection) return { nearestLine: instance.getVisibleRanges()[0]?.startLineNumber ?? 1 };
      const anchor = sourceOffset({
        lineNumber: selection.selectionStartLineNumber,
        column: selection.selectionStartColumn,
      });
      const active = sourceOffset({
        lineNumber: selection.positionLineNumber,
        column: selection.positionColumn,
      });
      const nearestLine =
        anchor === active
          ? (instance.getVisibleRanges()[0]?.startLineNumber ?? 1)
          : sourceLineAtOffset(readContent(), active);
      return { selection: { anchor, active }, nearestLine };
    },
    restore: (position) => {
      if (!isLive()) return;
      const target = clampSourcePosition(readContent(), position);
      if (target.selection) {
        const range = selectionRange(
          nativePosition(target.selection.anchor),
          nativePosition(target.selection.active),
        );
        instance.setSelection(range);
        if (target.selection.anchor === target.selection.active)
          instance.revealLineInCenter(target.nearestLine, IMMEDIATE_SCROLL);
        else instance.revealRangeInCenterIfOutsideViewport(range, IMMEDIATE_SCROLL);
      } else {
        instance.revealLineInCenter(target.nearestLine, IMMEDIATE_SCROLL);
      }
      instance.focus();
    },
  };
}

export function useMonacoSourcePosition({
  editor: instance,
  content,
  identity,
  path,
  worktreePath,
  repo,
  sessionId,
  isActive = true,
  onSourcePositionHandle,
}: MonacoSourcePositionOptions) {
  const contentRef = useRef(content);
  contentRef.current = content;
  useEffect(() => {
    if (!instance || !isActive || !onSourcePositionHandle) return;
    let invalidate: (() => void) | undefined;
    const register = () => {
      invalidate?.();
      invalidate = undefined;
      const model = instance.getModel();
      if (
        !model ||
        (path &&
          !monacoEditorMatchesFile(instance, path, worktreePath ?? null, { repo, sessionId }))
      )
        return;
      let live = true;
      onSourcePositionHandle(
        createHandle(
          instance,
          model,
          () => contentRef.current,
          () => live && instance.getModel() === model,
        ),
      );
      invalidate = () => {
        live = false;
        onSourcePositionHandle(null);
      };
    };
    const subscription = instance.onDidChangeModel(register);
    register();
    return () => {
      subscription.dispose();
      invalidate?.();
    };
  }, [instance, identity, path, worktreePath, repo, sessionId, isActive, onSourcePositionHandle]);
}
