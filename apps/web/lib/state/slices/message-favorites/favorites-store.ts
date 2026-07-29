import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import type { MessageFavoritesSlice, MessageFavoritesState } from "./types";
import { persistSessionFavorites, loadSessionFavorites } from "./persistence";

const defaultState: MessageFavoritesState = {
  bySession: {},
};

/** Persist the current favorite set for a session to sessionStorage. */
function persistSession(state: MessageFavoritesState, sessionId: string): void {
  const ids = Object.keys(state.bySession[sessionId] ?? {});
  persistSessionFavorites(sessionId, ids);
}

/** Session-scoped favorite state with hydration and persistence actions. */
export const useMessageFavoritesStore = create<MessageFavoritesSlice>()(
  immer<MessageFavoritesSlice>((set) => ({
    ...defaultState,

    hydrateSession: (sessionId: string) =>
      set((state) => {
        const existing = state.bySession[sessionId];
        if (existing) return;
        const ids = loadSessionFavorites(sessionId);
        state.bySession[sessionId] = Object.fromEntries(ids.map((id) => [id, true as const]));
      }),

    toggleFavorite: (sessionId: string, messageId: string) =>
      set((state) => {
        const current = { ...(state.bySession[sessionId] ?? {}) };
        if (current[messageId]) {
          delete current[messageId];
        } else {
          current[messageId] = true;
        }
        state.bySession[sessionId] = current;
        persistSession(state, sessionId);
      }),
  })),
);
