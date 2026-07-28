/** Maps each session to its favorited message IDs. */
export type MessageFavoritesState = {
  /** sessionId -> set of favorited message ids (value is always `true`). */
  bySession: Record<string, Record<string, true>>;
};

/** Actions for hydrating and toggling session-scoped message favorites. */
export type MessageFavoritesActions = {
  /** Load persisted favorites for a session into state (no-op once hydrated). */
  hydrateSession: (sessionId: string) => void;
  /** Flip the favorite flag for a message and persist the session's set. */
  toggleFavorite: (sessionId: string, messageId: string) => void;
};

/** Complete message-favorites store contract. */
export type MessageFavoritesSlice = MessageFavoritesState & MessageFavoritesActions;
