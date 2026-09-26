export function getSuggestionMenuOpenState({
  mentionIsOpen,
  slashIsOpen,
  slashItemCount,
  entityReferenceMenuOpen,
}: {
  mentionIsOpen: boolean;
  slashIsOpen: boolean;
  slashItemCount: number;
  entityReferenceMenuOpen: boolean;
}) {
  // MentionMenu renders an empty state while open, so it must stay armed even
  // when the current query has no results. SlashCommandMenu returns null when
  // it has no commands, so it still requires at least one item here.
  const mentionMenuOpen = mentionIsOpen;
  const slashMenuOpen = slashIsOpen && slashItemCount > 0;
  return {
    mentionMenuOpen,
    slashMenuOpen,
    isSuggestionMenuOpen: mentionMenuOpen || slashMenuOpen || entityReferenceMenuOpen,
  };
}
