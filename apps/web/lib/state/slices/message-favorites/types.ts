/**
 * Client-only "favorite" flag for chat messages. Lets a user star a message
 * in the session transcript to find it again quickly. Not synced to the
 * backend — persisted per-session in sessionStorage only (see persistence.ts).
 */

export type MessageFavoritesState = {
  /** sessionId -> set of favorited message ids (value is always `true`). */
  bySession: Record<string, Record<string, true>>;
};

export type MessageFavoritesActions = {
  /** Load persisted favorites for a session into state (no-op once hydrated). */
  hydrateSession: (sessionId: string) => void;
  /** Flip the favorite flag for a message and persist the session's set. */
  toggleFavorite: (sessionId: string, messageId: string) => void;
};

export type MessageFavoritesSlice = MessageFavoritesState & MessageFavoritesActions;
