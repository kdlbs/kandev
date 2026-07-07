"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import CodeMirror from "@uiw/react-codemirror";
import { vscodeDark } from "@uiw/codemirror-theme-vscode";
import { EditorView } from "@codemirror/view";
import type { Extension } from "@codemirror/state";
import { getCodeMirrorExtensionFromPath } from "@/lib/languages";
import { consumePendingCursorPosition } from "@/hooks/use-file-editors";
import { useCodeMirrorWalkthroughRange } from "@/components/editors/codemirror/use-codemirror-walkthrough-range";
import { cn } from "@/lib/utils";

type FileViewerContentProps = {
  path: string;
  content: string;
  className?: string;
};

export function FileViewerContent({ path, content, className }: FileViewerContentProps) {
  const langExt = getCodeMirrorExtensionFromPath(path);
  const extensions: Extension[] = [EditorView.lineWrapping, EditorView.editable.of(false)];
  if (langExt) {
    extensions.push(langExt);
  }

  const viewRef = useRef<EditorView | null>(null);
  const [editorView, setEditorView] = useState<EditorView | null>(null);
  const areaRef = useRef<HTMLDivElement>(null);
  const walkthroughRange = useCodeMirrorWalkthroughRange({
    view: editorView,
    editorAreaRef: areaRef,
    path,
  });

  // Consume a pending cursor position (set by chat read/edit file links before
  // they open the file) and scroll CodeMirror so the target 1-based line is
  // centered. No pending entry → no scroll, so normal file browsing stays at
  // the top. Desktop's Monaco consumes the same entry; on mobile there is no
  // Monaco, so this CodeMirror viewer must consume it instead.
  const scrollToPendingLine = useCallback((view: EditorView, filePath: string) => {
    const pending = consumePendingCursorPosition(filePath);
    if (!pending) return;
    const totalLines = view.state.doc.lines;
    const clampedLine = Math.min(Math.max(pending.line, 1), totalLines);
    const pos = view.state.doc.line(clampedLine).from;
    view.dispatch({
      selection: { anchor: pos },
      effects: EditorView.scrollIntoView(pos, { y: "center" }),
    });
  }, []);

  const handleCreateEditor = useCallback(
    (view: EditorView) => {
      viewRef.current = view;
      setEditorView(view);
      scrollToPendingLine(view, path);
    },
    [path, scrollToPendingLine],
  );

  // The same FileViewerContent instance is reused when switching files (the
  // path prop changes without a remount), and a pending entry may be set after
  // the editor already mounted. Re-consume + scroll whenever path changes and
  // the EditorView exists, so the second open of a different file still jumps.
  useEffect(() => {
    if (viewRef.current) {
      scrollToPendingLine(viewRef.current, path);
    }
  }, [path, scrollToPendingLine]);

  return (
    <div ref={areaRef} className={cn("relative h-full overflow-hidden", className)}>
      <CodeMirror
        value={content}
        height="100%"
        theme={vscodeDark}
        extensions={extensions}
        readOnly
        onCreateEditor={handleCreateEditor}
        basicSetup={{
          lineNumbers: true,
          foldGutter: true,
          highlightActiveLine: false,
          highlightSelectionMatches: true,
        }}
        className={cn(
          // Wrapper must have a bounded height so CodeMirror's internal .cm-scroller
          // scrolls instead of expanding to content height. touch-pan-y on
          // .cm-scroller is what makes vertical touch scroll work on mobile —
          // without it, .cm-content captures the gesture for selection.
          "h-full overflow-hidden text-xs",
          "[&_.cm-scroller]:touch-pan-y [&_.cm-scroller]:overscroll-contain",
        )}
      />
      {walkthroughRange ? (
        <div
          aria-hidden="true"
          data-testid="walkthrough-editor-range"
          data-line-range={`${walkthroughRange.startLine}-${walkthroughRange.endLine}`}
          className="pointer-events-none absolute z-20 rounded-sm border-l-2 border-primary/70 bg-primary/10 shadow-[inset_0_0_0_1px_hsl(var(--primary)/0.18)]"
          style={{
            top: walkthroughRange.top,
            left: walkthroughRange.left,
            width: walkthroughRange.width,
            height: walkthroughRange.height,
          }}
        />
      ) : null}
    </div>
  );
}
