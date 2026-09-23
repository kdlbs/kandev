"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import CodeMirror from "@uiw/react-codemirror";
import { vscodeDark } from "@uiw/codemirror-theme-vscode";
import { EditorView } from "@codemirror/view";
import type { Extension } from "@codemirror/state";
import { getCodeMirrorExtensionFromPath } from "@/lib/languages";
import { useCodeMirrorWalkthroughRange } from "@/components/editors/codemirror/use-codemirror-walkthrough-range";
import {
  clearCodeMirrorCursorFlash,
  codeMirrorCursorFlashExtension,
  revealPendingCodeMirrorCursor,
} from "@/components/editors/codemirror/codemirror-cursor-navigation";
import { cn } from "@/lib/utils";

type FileViewerContentProps = {
  path: string;
  content: string;
  repo?: string;
  sessionId?: string;
  className?: string;
  editable?: boolean;
  onChange?: (newContent: string) => void;
};

export function FileViewerContent({
  path,
  content,
  repo,
  sessionId,
  className,
  editable = false,
  onChange,
}: FileViewerContentProps) {
  const langExt = getCodeMirrorExtensionFromPath(path);
  const extensions: Extension[] = [
    EditorView.lineWrapping,
    EditorView.editable.of(editable),
    codeMirrorCursorFlashExtension,
  ];
  if (langExt) {
    extensions.push(langExt);
  }

  const viewRef = useRef<EditorView | null>(null);
  const fileIdentityRef = useRef({ path, repo, sessionId });
  const [editorView, setEditorView] = useState<EditorView | null>(null);
  const areaRef = useRef<HTMLDivElement>(null);
  const walkthroughRange = useCodeMirrorWalkthroughRange({
    view: editorView,
    editorAreaRef: areaRef,
    path,
    repo,
  });

  const handleCreateEditor = useCallback(
    (view: EditorView) => {
      viewRef.current = view;
      setEditorView(view);
      revealPendingCodeMirrorCursor(view, path, repo, sessionId);
    },
    [path, repo, sessionId],
  );

  // The same FileViewerContent instance is reused when switching files (the
  // path prop changes without a remount), and a pending entry may be set after
  // the editor already mounted. Re-consume + scroll whenever path changes and
  // the EditorView exists, so the second open of a different file still jumps.
  useEffect(() => {
    const view = viewRef.current;
    const previous = fileIdentityRef.current;
    const fileChanged =
      previous.path !== path || previous.repo !== repo || previous.sessionId !== sessionId;
    fileIdentityRef.current = { path, repo, sessionId };
    if (!view || !fileChanged) return;
    clearCodeMirrorCursorFlash(view);
    revealPendingCodeMirrorCursor(view, path, repo, sessionId);
  }, [path, repo, sessionId]);

  useEffect(() => {
    if (!editorView) return;
    return () => clearCodeMirrorCursorFlash(editorView);
  }, [editorView]);

  return (
    <div ref={areaRef} className={cn("relative h-full overflow-hidden", className)}>
      <CodeMirror
        value={content}
        height="100%"
        theme={vscodeDark}
        extensions={extensions}
        readOnly={!editable}
        onChange={editable ? onChange : undefined}
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
