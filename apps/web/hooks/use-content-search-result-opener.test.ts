import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { WorkspaceContentSearchResult } from "@/lib/types/backend";

const mockOpenContentSearchResult = vi.fn();

vi.mock("@/lib/commands/content-search-selection", () => ({
  openContentSearchResult: (...args: unknown[]) => mockOpenContentSearchResult(...args),
}));

import { useContentSearchResultOpener } from "./use-content-search-result-opener";

const selected = {
  repository_name: "web",
  path: "src/app.tsx",
  line: 3,
  column: 2,
  preview: "needle",
  match_ranges: [],
} satisfies WorkspaceContentSearchResult;

describe("useContentSearchResultOpener", () => {
  beforeEach(() => {
    mockOpenContentSearchResult.mockReset();
  });

  it("closes the palette and opens the selected workspace result", () => {
    const setOpen = vi.fn();
    const { result } = renderHook(() =>
      useContentSearchResultOpener(setOpen, "/tasks/task-1", "session-1"),
    );

    act(() => result.current(selected));

    expect(setOpen).toHaveBeenCalledWith(false);
    expect(mockOpenContentSearchResult).toHaveBeenCalledWith(
      selected,
      "/tasks/task-1",
      "session-1",
    );
  });

  it("ignores a stale selection after the active session is cleared", () => {
    const setOpen = vi.fn();
    const { result } = renderHook(() =>
      useContentSearchResultOpener(setOpen, "/tasks/task-1", null),
    );

    act(() => result.current(selected));

    expect(setOpen).not.toHaveBeenCalled();
    expect(mockOpenContentSearchResult).not.toHaveBeenCalled();
  });
});
