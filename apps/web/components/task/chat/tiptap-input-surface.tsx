import type { ComponentProps, RefObject } from "react";
import type { Editor } from "@tiptap/core";
import { TipTapEditorArea } from "./tiptap-editor-area";
import { TipTapPopups } from "./tiptap-popups";

type TipTapInputSurfaceProps = ComponentProps<typeof TipTapPopups> & {
  editor: Editor | null;
  editorWrapperRef: RefObject<HTMLDivElement | null>;
  sessionId: string | null;
  taskId: string | null;
  detailActive: boolean;
  onKeyDownCapture: (event: React.KeyboardEvent<HTMLElement>) => void;
};

export function TipTapInputSurface(p: TipTapInputSurfaceProps) {
  const {
    editor,
    editorWrapperRef,
    sessionId,
    taskId,
    detailActive,
    onKeyDownCapture,
    ...popupProps
  } = p;
  return (
    <>
      <TipTapPopups {...popupProps} />
      <TipTapEditorArea
        editor={editor}
        editorWrapperRef={editorWrapperRef}
        sessionId={sessionId}
        taskId={taskId}
        detailActive={detailActive}
        onKeyDownCapture={onKeyDownCapture}
      />
    </>
  );
}
