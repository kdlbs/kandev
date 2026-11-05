type ComposerEscapeControls = {
  isSuggestionMenuOpen: boolean;
  isReverseSearchOpen: boolean;
  isPromptSuggestionVisible: boolean;
};

export function shouldDeferComposerEscapeToExistingControls(
  input: ComposerEscapeControls,
): boolean {
  return input.isSuggestionMenuOpen || input.isReverseSearchOpen || input.isPromptSuggestionVisible;
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
