import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useMessageFavoritesStore } from "./favorites-store";
import { MESSAGE_FAVORITES_STORAGE_PREFIX, loadSessionFavorites } from "./persistence";

const SESSION_ID = "session-favorites-1";
const OTHER_SESSION_ID = "session-favorites-2";

describe("message favorites store", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
    useMessageFavoritesStore.setState({ bySession: {} });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("starts with no favorites for an unhydrated session", () => {
    const state = useMessageFavoritesStore.getState();
    expect(state.bySession[SESSION_ID]).toBeUndefined();
  });

  it("toggling a message on marks it favorited and persists it to sessionStorage", () => {
    useMessageFavoritesStore.getState().toggleFavorite(SESSION_ID, "msg-1");

    const state = useMessageFavoritesStore.getState();
    expect(state.bySession[SESSION_ID]?.["msg-1"]).toBe(true);
    expect(loadSessionFavorites(SESSION_ID)).toEqual(["msg-1"]);
    expect(
      window.sessionStorage.getItem(`${MESSAGE_FAVORITES_STORAGE_PREFIX}${SESSION_ID}`),
    ).not.toBeNull();
  });

  it("toggling a favorited message off removes it and clears storage once empty", () => {
    useMessageFavoritesStore.getState().toggleFavorite(SESSION_ID, "msg-1");
    useMessageFavoritesStore.getState().toggleFavorite(SESSION_ID, "msg-1");

    const state = useMessageFavoritesStore.getState();
    expect(state.bySession[SESSION_ID]?.["msg-1"]).toBeUndefined();
    expect(loadSessionFavorites(SESSION_ID)).toEqual([]);
    expect(
      window.sessionStorage.getItem(`${MESSAGE_FAVORITES_STORAGE_PREFIX}${SESSION_ID}`),
    ).toBeNull();
  });

  it("scopes favorites per session so unrelated sessions never share a namespace", () => {
    useMessageFavoritesStore.getState().toggleFavorite(SESSION_ID, "msg-1");
    useMessageFavoritesStore.getState().toggleFavorite(OTHER_SESSION_ID, "msg-2");

    const state = useMessageFavoritesStore.getState();
    expect(state.bySession[SESSION_ID]).toEqual({ "msg-1": true });
    expect(state.bySession[OTHER_SESSION_ID]).toEqual({ "msg-2": true });
  });

  it("hydrates a session's favorites from sessionStorage after a remount", () => {
    window.sessionStorage.setItem(
      `${MESSAGE_FAVORITES_STORAGE_PREFIX}${SESSION_ID}`,
      JSON.stringify(["msg-1", "msg-2"]),
    );

    useMessageFavoritesStore.getState().hydrateSession(SESSION_ID);

    const state = useMessageFavoritesStore.getState();
    expect(state.bySession[SESSION_ID]).toEqual({ "msg-1": true, "msg-2": true });
  });

  it("hydrates an empty session from storage only once", () => {
    const getItem = vi.spyOn(window.sessionStorage, "getItem");

    useMessageFavoritesStore.getState().hydrateSession(SESSION_ID);
    useMessageFavoritesStore.getState().hydrateSession(SESSION_ID);

    expect(getItem).toHaveBeenCalledTimes(1);
  });

  it("ignores malformed persisted favorites", () => {
    window.sessionStorage.setItem(
      `${MESSAGE_FAVORITES_STORAGE_PREFIX}${SESSION_ID}`,
      JSON.stringify("stale"),
    );

    expect(loadSessionFavorites(SESSION_ID)).toEqual([]);
  });

  it("hydrating is a no-op once a session already has favorites in state", () => {
    useMessageFavoritesStore.getState().toggleFavorite(SESSION_ID, "msg-1");
    window.sessionStorage.setItem(
      `${MESSAGE_FAVORITES_STORAGE_PREFIX}${SESSION_ID}`,
      JSON.stringify(["msg-stale"]),
    );

    useMessageFavoritesStore.getState().hydrateSession(SESSION_ID);

    expect(useMessageFavoritesStore.getState().bySession[SESSION_ID]).toEqual({ "msg-1": true });
  });

  it("ignores a malformed persisted value instead of crashing on hydrate", () => {
    window.sessionStorage.setItem(
      `${MESSAGE_FAVORITES_STORAGE_PREFIX}${SESSION_ID}`,
      JSON.stringify("stale"),
    );

    expect(() => useMessageFavoritesStore.getState().hydrateSession(SESSION_ID)).not.toThrow();
    expect(useMessageFavoritesStore.getState().bySession[SESSION_ID]).toEqual({});
  });

  it("ignores a persisted array containing non-string entries", () => {
    window.sessionStorage.setItem(
      `${MESSAGE_FAVORITES_STORAGE_PREFIX}${SESSION_ID}`,
      JSON.stringify(["msg-1", 42]),
    );

    expect(() => useMessageFavoritesStore.getState().hydrateSession(SESSION_ID)).not.toThrow();
    expect(useMessageFavoritesStore.getState().bySession[SESSION_ID]).toEqual({});
  });
});
