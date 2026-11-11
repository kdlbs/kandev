"use client";

import { useCallback } from "react";
import type { Editor } from "@tiptap/core";
import { useClarificationEscapeGuard } from "@/hooks/use-clarification-escape-guard";
import { shouldClaimComposerEscape } from "./chat-escape-cancel";

type TipTapClarificationEscapeGuardOptions = {
  editor: Editor | null;
  isSuggestionMenuOpen: boolean;
  isReverseSearchOpen: boolean;
  isPromptSuggestionVisible: boolean;
  shouldHandleEscape?: (event: KeyboardEvent) => boolean;
};

export function useTipTapClarificationEscapeGuard({
  editor,
  isSuggestionMenuOpen,
  isReverseSearchOpen,
  isPromptSuggestionVisible,
  shouldHandleEscape,
}: TipTapClarificationEscapeGuardOptions) {
  const shouldClaimEscape = useCallback(
    (event: KeyboardEvent) => {
      const target = event.target;
      const editorDom = editor?.view.dom;
      return shouldClaimComposerEscape(
        Boolean(editorDom && target instanceof Node && editorDom.contains(target)),
        shouldHandleEscape?.(event) ?? false,
        { isSuggestionMenuOpen, isReverseSearchOpen, isPromptSuggestionVisible },
      );
    },
    [
      editor,
      isSuggestionMenuOpen,
      isPromptSuggestionVisible,
      isReverseSearchOpen,
      shouldHandleEscape,
    ],
  );

  useClarificationEscapeGuard(shouldHandleEscape ? shouldClaimEscape : null);
}
