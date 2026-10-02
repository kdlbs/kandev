import { useEffect, useRef } from "react";
import type { ChatInputContainerHandle } from "@/components/task/chat/chat-input-container";

type InitialDraftDelivery = {
  draft?: string;
  initialPrompt?: string;
  chatInputRef: React.RefObject<ChatInputContainerHandle | null>;
};

/**
 * Inserts a caller-supplied draft into the composer once, without submitting.
 * Runs after every commit (not keyed on the draft prop alone) so a draft that
 * arrives before the composer mounts is held, not dropped. `undefined` or
 * `""` resets the applied-value bookkeeping without touching the composer,
 * so a caller re-applies an identical draft by passing `undefined` first.
 */
export function useQuickChatInitialDraft({
  draft,
  initialPrompt,
  chatInputRef,
}: InitialDraftDelivery) {
  const appliedRef = useRef<string | undefined>(undefined);

  useEffect(() => {
    if (!draft) {
      appliedRef.current = undefined;
      return;
    }
    if (initialPrompt) return;
    if (appliedRef.current === draft) return;
    const input = chatInputRef.current;
    if (!input) return;
    // The composer ref can exist before its underlying editor does (e.g.
    // TipTap's `immediatelyRender: false`); insertText/clear silently no-op
    // until then, so wait for it rather than marking the draft applied.
    if (!input.getTextareaElement()) return;
    input.clear();
    input.insertText(draft, 0, 0);
    input.focusInput();
    appliedRef.current = draft;
  });
}
