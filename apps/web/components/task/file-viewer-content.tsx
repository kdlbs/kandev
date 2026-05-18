"use client";

import CodeMirror from "@uiw/react-codemirror";
import { vscodeDark } from "@uiw/codemirror-theme-vscode";
import { EditorView } from "@codemirror/view";
import type { Extension } from "@codemirror/state";
import { getCodeMirrorExtensionFromPath } from "@/lib/languages";
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

  return (
    <CodeMirror
      value={content}
      height="100%"
      theme={vscodeDark}
      extensions={extensions}
      readOnly
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
        className,
      )}
    />
  );
}
