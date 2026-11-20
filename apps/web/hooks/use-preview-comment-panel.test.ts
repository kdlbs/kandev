import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const touch = vi.hoisted(() => ({ enabled: false }));
vi.mock("@/hooks/use-compact-task-chrome", () => ({ useTouchDrawer: () => touch.enabled }));

import { PREVIEW_COMMENT_PANEL_KEY, usePreviewCommentPanel } from "./use-preview-comment-panel";

afterEach(() => {
  touch.enabled = false;
  window.localStorage.clear();
});

describe("usePreviewCommentPanel", () => {
  it("is open by default on fine pointers and remembers being hidden", () => {
    const { result, unmount } = renderHook(() => usePreviewCommentPanel());
    expect(result.current.open).toBe(true);
    expect(result.current.docked).toBe(true);

    act(() => result.current.toggle());
    expect(result.current.open).toBe(false);
    expect(window.localStorage.getItem(PREVIEW_COMMENT_PANEL_KEY)).toBe("false");
    unmount();

    const next = renderHook(() => usePreviewCommentPanel());
    expect(next.result.current.open).toBe(false);
  });

  it("starts hidden on coarse pointers without touching the stored choice", () => {
    touch.enabled = true;
    const { result } = renderHook(() => usePreviewCommentPanel());
    expect(result.current.open).toBe(false);

    act(() => result.current.toggle());
    expect(result.current.open).toBe(true);
    expect(result.current.docked).toBe(false);
    expect(window.localStorage.getItem(PREVIEW_COMMENT_PANEL_KEY)).toBeNull();
  });
});
