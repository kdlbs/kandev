import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { SidebarView } from "@/lib/state/slices/ui/sidebar-view-types";

const routing = vi.hoisted(() => {
  const state = {
    params: "",
    replace: vi.fn((href: string) => {
      state.params = href.split("?")[1] ?? "";
    }),
  };
  return state;
});

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ replace: routing.replace }),
  useSearchParams: () => new URLSearchParams(routing.params),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: "en", resolvedLanguage: "en" },
  }),
}));

import { useMobileListResults } from "./use-mobile-list-results";

const VIEW: SidebarView = {
  id: "view-a",
  name: "All tasks",
  filters: [{ id: "saved-state", dimension: "state", op: "is", value: "backlog" }],
  sort: {
    key: "running",
    direction: "desc",
    thenBy: [{ key: "lastActivityAt", direction: "desc" }],
  },
  group: "none",
  groupIndent: true,
  collapsedGroups: [],
};

beforeEach(() => {
  routing.params = "";
  routing.replace.mockClear();
});

afterEach(cleanup);

describe("useMobileListResults", () => {
  it("keeps applied temporary filters and literal search separate from the saved view", () => {
    const { result } = renderHook(() => useMobileListResults("ws-a", VIEW));
    const originalFilters = VIEW.filters;
    const originalSort = VIEW.sort;

    act(() => {
      result.current.beginFilterEdit();
      result.current.setFilterDraft([
        { id: "temporary-state", dimension: "state", op: "is", value: "review" },
      ]);
      result.current.applyFilterEdit();
      result.current.setSearchQuery("literal%_query");
    });

    expect(result.current.queryOverrides).toMatchObject({
      filters: [
        { dimension: "state", op: "is", value: "backlog" },
        { dimension: "state", op: "is", value: "review" },
      ],
      query: "literal%_query",
      sort: {
        key: "running",
        direction: "desc",
        then_by: [{ key: "lastActivityAt", direction: "desc" }],
      },
    });
    expect(result.current.activeFilterCount).toBe(1);
    expect(VIEW.filters).toBe(originalFilters);
    expect(VIEW.sort).toBe(originalSort);
  });

  it("discards un-applied filter edits and restores the last applied draft on cancel", () => {
    const { result } = renderHook(() => useMobileListResults("ws-a", VIEW));
    const applied = [
      {
        id: "temporary-workflow",
        dimension: "workflow" as const,
        op: "is" as const,
        value: "wf-a",
      },
    ];
    const unApplied = [
      {
        id: "temporary-repository",
        dimension: "repository" as const,
        op: "is" as const,
        value: "repo-a",
      },
    ];

    act(() => {
      result.current.beginFilterEdit();
      result.current.setFilterDraft(applied);
      result.current.applyFilterEdit();
      result.current.beginFilterEdit();
      result.current.setFilterDraft(unApplied);
      result.current.cancelFilterEdit();
    });

    expect(result.current.resultFilters).toEqual(applied);
    expect(result.current.filterDraft).toEqual(applied);
    expect(result.current.queryOverrides?.filters).toEqual([
      { dimension: "state", op: "is", value: "backlog" },
      { dimension: "workflow", op: "is", value: "wf-a" },
    ]);
  });

  it("clears route-owned results without persisting them and accepts the same link again", () => {
    window.history.replaceState({}, "", "/tasks");
    routing.params = "workspace=ws-a&archive=archived&workflow=all&repository=all&q=literal%25_";
    const { result, rerender } = renderHook(() => useMobileListResults("ws-a", VIEW));

    expect(result.current.hasLinkScope).toBe(true);
    expect(result.current.queryOverrides?.query).toBe("literal%_");
    expect(result.current.queryOverrides?.filters).toEqual([
      { dimension: "state", op: "is", value: "backlog" },
      { dimension: "archived", op: "is", value: true },
    ]);

    act(() => result.current.clearSearchAndFilters());
    expect(result.current.queryOverrides).toBeUndefined();
    expect(routing.replace).toHaveBeenCalledWith("/tasks?workspace=ws-a", { scroll: false });

    rerender();
    expect(result.current.hasLinkScope).toBe(false);
    routing.params = "workspace=ws-a&archive=archived&workflow=all&repository=all&q=literal%25_";
    rerender();
    expect(result.current.hasLinkScope).toBe(true);
  });
});
