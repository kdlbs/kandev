type ComposerEscapeControls = {
  isSuggestionMenuOpen: boolean;
  isReverseSearchOpen: boolean;
  isPromptSuggestionVisible: boolean;
};

type AgentEscapeState = {
  canCancelAgent: boolean;
  escapeCancelsAgent: boolean;
  hasPendingClarification: boolean;
};

export function shouldDeferComposerEscapeToExistingControls(
  input: ComposerEscapeControls,
): boolean {
  return input.isSuggestionMenuOpen || input.isReverseSearchOpen || input.isPromptSuggestionVisible;
}

export function shouldCancelAgentWithEscape(
  event: Pick<KeyboardEvent, "key" | "isComposing" | "keyCode" | "repeat" | "defaultPrevented">,
  state: AgentEscapeState,
): boolean {
  return (
    event.key === "Escape" &&
    state.canCancelAgent &&
    state.escapeCancelsAgent &&
    !state.hasPendingClarification &&
    !event.isComposing &&
    event.keyCode !== 229 &&
    !event.repeat &&
    !event.defaultPrevented
  );
}

export function shouldClaimComposerEscape(
  targetIsInsideEditor: boolean,
  shouldHandleEscape: boolean,
  controls: ComposerEscapeControls,
): boolean {
  return (
    targetIsInsideEditor &&
    shouldHandleEscape &&
    !shouldDeferComposerEscapeToExistingControls(controls)
  );
}

export function createComposerEscapeCaptureHandler(
  onEscape: ((event: React.KeyboardEvent<HTMLElement>) => void) | undefined,
  isSuggestionMenuOpen: boolean,
  isReverseSearchOpen: boolean,
  isPromptSuggestionVisible: boolean,
): (event: React.KeyboardEvent<HTMLElement>) => void {
  return (event) => {
    if (
      !shouldDeferComposerEscapeToExistingControls({
        isSuggestionMenuOpen,
        isReverseSearchOpen,
        isPromptSuggestionVisible,
      })
    ) {
      onEscape?.(event);
    }
  };
}
