import { useEffect, useRef } from "react";
import { EditorView } from "@codemirror/view";
import {
  clampSourcePosition,
  sourceLineAtOffset,
  sourceOffsetAtLine,
} from "@/components/task/markdown-source-position";
import type { SourcePositionProps } from "./source-position";

type CodeMirrorSourcePositionOptions = SourcePositionProps & {
  view: EditorView | null;
  content: string;
  identity?: string;
};

export function useCodeMirrorSourcePosition({
  view,
  content,
  identity,
  isActive = true,
  onSourcePositionHandle,
}: CodeMirrorSourcePositionOptions) {
  const contentRef = useRef(content);
  contentRef.current = content;
  useEffect(() => {
    if (!view || !isActive || !onSourcePositionHandle) return;
    let live = true;
    const sourceOffset = (offset: number) => {
      const line = view.state.doc.lineAt(offset);
      return sourceOffsetAtLine(contentRef.current, line.number, offset - line.from + 1);
    };
    const editorOffset = (offset: number) => {
      const lineNumber = sourceLineAtOffset(contentRef.current, offset);
      const line = view.state.doc.line(Math.min(view.state.doc.lines, lineNumber));
      return Math.min(
        line.to,
        line.from + offset - sourceOffsetAtLine(contentRef.current, lineNumber),
      );
    };
    onSourcePositionHandle({
      capture: () => {
        if (!live) return null;
        const { anchor, head } = view.state.selection.main;
        const active = sourceOffset(head);
        const visibleOffset = view.lineBlockAtHeight(
          view.scrollDOM.getBoundingClientRect().top - view.documentTop,
        ).from;
        const nearestLine = sourceLineAtOffset(
          contentRef.current,
          anchor === head ? sourceOffset(visibleOffset) : active,
        );
        return { selection: { anchor: sourceOffset(anchor), active }, nearestLine };
      },
      restore: (position) => {
        if (!live) return;
        const target = clampSourcePosition(contentRef.current, position);
        const anchor = target.selection
          ? editorOffset(target.selection.anchor)
          : view.state.doc.line(Math.min(view.state.doc.lines, target.nearestLine)).from;
        const head = target.selection ? editorOffset(target.selection.active) : anchor;
        const reveal =
          anchor === head
            ? view.state.doc.line(Math.min(view.state.doc.lines, target.nearestLine)).from
            : head;
        view.dispatch({
          selection: { anchor, head },
          effects: EditorView.scrollIntoView(reveal, { y: "center" }),
        });
        view.focus();
      },
    });
    return () => {
      live = false;
      onSourcePositionHandle(null);
    };
  }, [view, identity, isActive, onSourcePositionHandle]);
}
