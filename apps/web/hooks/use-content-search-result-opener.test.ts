import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { WorkspaceContentSearchResult } from "@/lib/types/backend";

const mockOpenContentSearchResult = vi.fn();

vi.mock("@/lib/commands/content-search-selection", () => ({
  openContentSearchResult: (...args: unknown[]) => mockOpenContentSearchResult(...args),
}));

import { useContentSearchResultOpener } from "./use-content-search-result-opener";

describe("useContentSearchResultOpener", () => {
  it("closes the palette and opens the selected workspace result", () => {
    const setOpen = vi.fn();
    const selected = {
      repository_name: "web",
      path: "src/app.tsx",
      line: 3,
      column: 2,
      preview: "needle",
      match_ranges: [],
    } satisfies WorkspaceContentSearchResult;
    const { result } = renderHook(() => useContentSearchResultOpener(setOpen, "/tasks/task-1"));

    act(() => result.current(selected));

    expect(setOpen).toHaveBeenCalledWith(false);
    expect(mockOpenContentSearchResult).toHaveBeenCalledWith(selected, "/tasks/task-1");
  });
});
