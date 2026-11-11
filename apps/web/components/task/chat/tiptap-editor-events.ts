import { useLayoutEffect, type RefObject } from "react";
import type { Editor } from "@tiptap/core";
import { useReverseSearchSelectHandler } from "./use-reverse-search-select-handler";
import { useEditorPromptSuggestion } from "./use-prompt-suggestion-escape";

function useEditorRefSync(editorRef: RefObject<Editor | null>, editor: Editor | null) {
  useLayoutEffect(() => {
    editorRef.current = editor;
  });
}

export function useTipTapEditorEventHandlers(input: {
  editor: Editor | null;
  editorRef: RefObject<Editor | null>;
  editorWrapperRef: RefObject<HTMLDivElement | null>;
  suggestion: string | null;
  dismissSuggestion?: () => void;
  closeReverseSearch: () => void;
  applyHistoryEntry: (index: number) => void;
}) {
  useEditorRefSync(input.editorRef, input.editor);
  useEditorPromptSuggestion(
    input.editor,
    input.editorRef,
    input.editorWrapperRef,
    input.suggestion,
    input.dismissSuggestion,
  );
  return useReverseSearchSelectHandler(
    input.applyHistoryEntry,
    input.closeReverseSearch,
    input.editor,
  );
}
