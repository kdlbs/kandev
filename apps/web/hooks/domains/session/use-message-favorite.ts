import { useEffect } from "react";
import { useMessageFavoritesStore } from "@/lib/state/slices/message-favorites";

/**
 * Client-only "favorite" flag for a chat message, scoped to its session and
 * backed by sessionStorage (see `lib/state/slices/message-favorites`). Lets a
 * user star a message in a fast-moving transcript to find it again later.
 */
export function useMessageFavorite(
  sessionId: string,
  messageId: string,
): { isFavorite: boolean; toggleFavorite: () => void } {
  const hydrateSession = useMessageFavoritesStore((state) => state.hydrateSession);
  useEffect(() => {
    hydrateSession(sessionId);
  }, [sessionId, hydrateSession]);

  const isFavorite = useMessageFavoritesStore((state) =>
    Boolean(state.bySession[sessionId]?.[messageId]),
  );
  const toggleFavoriteInStore = useMessageFavoritesStore((state) => state.toggleFavorite);

  return {
    isFavorite,
    toggleFavorite: () => toggleFavoriteInStore(sessionId, messageId),
  };
}
