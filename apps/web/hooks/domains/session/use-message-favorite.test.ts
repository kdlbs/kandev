import { beforeEach, describe, expect, it } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useMessageFavorite } from "./use-message-favorite";
import {
  useMessageFavoritesStore,
  persistSessionFavorites,
} from "@/lib/state/slices/message-favorites";

const SESSION_ID = "session-hook-1";

describe("useMessageFavorite", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
    useMessageFavoritesStore.setState({ bySession: {} });
  });

  it("starts false and flips to true when toggled, then back to false", () => {
    const { result } = renderHook(() => useMessageFavorite(SESSION_ID, "msg-1"));

    expect(result.current.isFavorite).toBe(false);

    act(() => result.current.toggleFavorite());
    expect(result.current.isFavorite).toBe(true);

    act(() => result.current.toggleFavorite());
    expect(result.current.isFavorite).toBe(false);
  });

  it("hydrates a previously-favorited message from sessionStorage", () => {
    persistSessionFavorites(SESSION_ID, ["msg-2"]);
    useMessageFavoritesStore.setState({ bySession: {} });

    const { result } = renderHook(() => useMessageFavorite(SESSION_ID, "msg-2"));

    expect(result.current.isFavorite).toBe(true);
  });

  it("keeps favorites for different messages independent", () => {
    const { result: resultA } = renderHook(() => useMessageFavorite(SESSION_ID, "msg-a"));
    const { result: resultB } = renderHook(() => useMessageFavorite(SESSION_ID, "msg-b"));

    act(() => resultA.current.toggleFavorite());

    expect(resultA.current.isFavorite).toBe(true);
    expect(resultB.current.isFavorite).toBe(false);
  });
});
