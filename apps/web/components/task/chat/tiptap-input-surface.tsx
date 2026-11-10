import type { ComponentProps, RefObject } from "react";
import type { Editor } from "@tiptap/core";
import { TipTapEditorArea } from "./tiptap-editor-area";
import { TipTapPopups } from "./tiptap-popups";
import { useTipTapEditorEventHandlers } from "./tiptap-editor-events";
import { useTipTapClarificationEscapeGuard } from "./use-tiptap-clarification-escape-guard";

type TipTapEscapeGuardProps = {
  shouldHandleEscape?: (event: KeyboardEvent) => boolean;
  isPromptSuggestionVisible: boolean;
};

type TipTapInputSurfaceProps = Omit<
  ComponentProps<typeof TipTapPopups>,
  "onReverseSearchSelect"
> & {
  editor: Editor | null;
  editorWrapperRef: RefObject<HTMLDivElement | null>;
  editorRef: RefObject<Editor | null>;
  isSuggestionMenuOpen: boolean;
  escapeGuard: TipTapEscapeGuardProps;
  promptSuggestion: string | null;
  onPromptSuggestionDismiss?: () => void;
  applyHistoryEntry: (index: number) => void;
  sessionId: string | null;
  taskId: string | null;
  detailActive: boolean;
  onKeyDownCapture: (event: React.KeyboardEvent<HTMLElement>) => void;
};

export function TipTapInputSurface(p: TipTapInputSurfaceProps) {
  const {
    editor,
    editorWrapperRef,
    editorRef,
    isSuggestionMenuOpen,
    escapeGuard,
    promptSuggestion,
    onPromptSuggestionDismiss,
    applyHistoryEntry,
    sessionId,
    taskId,
    detailActive,
    onKeyDownCapture,
    ...popupProps
  } = p;
  useTipTapClarificationEscapeGuard({
    editor,
    isSuggestionMenuOpen,
    isReverseSearchOpen: popupProps.overlay.isReverseSearchOpen,
    isPromptSuggestionVisible: escapeGuard.isPromptSuggestionVisible,
    shouldHandleEscape: escapeGuard.shouldHandleEscape,
  });
  const handleReverseSearchSelect = useTipTapEditorEventHandlers({
    editor,
    editorRef,
    editorWrapperRef,
    suggestion: promptSuggestion,
    dismissSuggestion: onPromptSuggestionDismiss,
    closeReverseSearch: popupProps.overlay.closeReverseSearch,
    applyHistoryEntry,
  });
  return (
    <>
      <TipTapPopups {...popupProps} onReverseSearchSelect={handleReverseSearchSelect} />
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
