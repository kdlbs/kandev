import type { Editor } from "@tiptap/core";
import { EditorContent } from "@tiptap/react";
import type { RefObject } from "react";
import { EditorContextProvider } from "./editor-context";

export function TipTapEditorArea(p: {
  editor: Editor | null;
  editorWrapperRef: RefObject<HTMLDivElement | null>;
  sessionId: string | null;
  taskId: string | null;
  detailActive: boolean;
  onKeyDownCapture: (event: React.KeyboardEvent<HTMLElement>) => void;
}) {
  return (
    <EditorContextProvider
      value={{ sessionId: p.sessionId, taskId: p.taskId, detailActive: p.detailActive }}
    >
      <div ref={p.editorWrapperRef} className="h-full">
        <EditorContent
          editor={p.editor}
          onKeyDownCapture={p.onKeyDownCapture}
          className="h-full [&_.tiptap]:h-full [&_.tiptap]:outline-none"
        />
      </div>
    </EditorContextProvider>
  );
}
