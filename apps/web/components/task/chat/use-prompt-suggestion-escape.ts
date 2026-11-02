"use client";

import { useCallback, useEffect, type RefObject } from "react";
import type { Editor } from "@tiptap/core";
import { useClarificationEscapeGuard } from "@/hooks/use-clarification-escape-guard";
import { updatePromptSuggestion, visiblePromptSuggestion } from "./tiptap-prompt-suggestion";

type PromptSuggestionEscapeArgs = {
  /** True while the composer holds a suggestion for the current turn. */
  active: boolean;
  /** Whether the ghost text is visible right now (empty, editable draft). */
  isVisible: () => boolean;
  containerRef: RefObject<HTMLElement | null>;
  onDismiss: () => void;
};

function owns(container: HTMLElement | null, node: Node | null): boolean {
  return !!container && !!node && container.contains(node);
}

/**
 * Escape dismisses a visible suggestion. Quick Chat's dialog intercepts Escape
 * in the capture phase, before ProseMirror sees it, so this follows the
 * suggestion-menu pattern: an ownership-aware guard predicate tells the dialog
 * the key is claimed, and a document listener dismisses and stops propagation.
 */
export function usePromptSuggestionEscape({
  active,
  isVisible,
  containerRef,
  onDismiss,
}: PromptSuggestionEscapeArgs) {
  const claims = useCallback(
    (event: KeyboardEvent) => {
      const container = containerRef.current;
      const owned =
        owns(container, event.target as Node | null) || owns(container, document.activeElement);
      return owned && isVisible();
    },
    [containerRef, isVisible],
  );
  useClarificationEscapeGuard(active ? claims : null);
  useEffect(() => {
    if (!active) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || !claims(event)) return;
      onDismiss();
      event.preventDefault();
      event.stopPropagation();
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [active, claims, onDismiss]);
}

/** Syncs the composer's suggestion into the editor and wires Escape dismissal. */
export function useEditorPromptSuggestion(
  editor: Editor | null,
  editorRef: RefObject<Editor | null>,
  containerRef: RefObject<HTMLElement | null>,
  suggestion: string | null,
  onDismiss: (() => void) | undefined,
) {
  useEffect(() => {
    updatePromptSuggestion(editor, suggestion);
  }, [editor, suggestion]);
  const isVisible = useCallback(
    () => visiblePromptSuggestion(editorRef.current) !== null,
    [editorRef],
  );
  const dismiss = useCallback(() => onDismiss?.(), [onDismiss]);
  usePromptSuggestionEscape({
    active: Boolean(suggestion && onDismiss),
    isVisible,
    containerRef,
    onDismiss: dismiss,
  });
}
