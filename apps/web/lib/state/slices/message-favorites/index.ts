export { useMessageFavoritesStore } from "./favorites-store";
export type {
  MessageFavoritesState,
  MessageFavoritesActions,
  MessageFavoritesSlice,
} from "./types";
export {
  persistSessionFavorites,
  loadSessionFavorites,
  clearPersistedSessionFavorites,
  MESSAGE_FAVORITES_STORAGE_PREFIX,
} from "./persistence";
