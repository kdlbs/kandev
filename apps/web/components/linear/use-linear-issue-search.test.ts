import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement, type PropsWithChildren } from "react";
import type { LinearIssue, LinearSearchResult } from "@/lib/types/linear";

const searchLinearIssuesMock =
  vi.fn<(params: unknown, options?: unknown) => Promise<LinearSearchResult>>();

vi.mock("@/lib/api/domains/linear-api", () => ({
  searchLinearIssues: (params: unknown, options?: unknown) =>
    searchLinearIssuesMock(params, options),
}));

import { useLinearIssueSearch } from "./use-linear-issue-search";

afterEach(() => cleanup());

const WORKSPACE = "ws-1";

// Reset inline at the top of each test rather than in a beforeEach: a beforeEach
// hook shifts Vitest's post-test unhandled-rejection check so the debounced
// reject in the error case is mis-flagged.
function resetSearchMock() {
  searchLinearIssuesMock.mockReset();
}

function fakeIssue(overrides: Partial<LinearIssue> = {}): LinearIssue {
  return {
    id: "1",
    identifier: "ENG-1",
    title: "Issue",
    description: "",
    stateId: "s1",
    stateName: "Todo",
    stateType: "unstarted",
    stateCategory: "new",
    teamId: "t1",
    teamKey: "ENG",
    priority: 0,
    url: "https://linear.app/eng-1",
    states: [],
    ...overrides,
  };
}

function emptyResult(): LinearSearchResult {
  return { issues: [], maxResults: 25, isLast: true };
}

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return function Wrapper({ children }: PropsWithChildren) {
    return createElement(QueryClientProvider, { client: queryClient }, children);
  };
}

describe("useLinearIssueSearch — gating", () => {
  it("does not fetch while disabled (integration not configured)", async () => {
    resetSearchMock();
    searchLinearIssuesMock.mockResolvedValue(emptyResult());
    const { result } = renderHook(() => useLinearIssueSearch(WORKSPACE, "", "", "me", false), {
      wrapper: createWrapper(),
    });
    await new Promise((r) => setTimeout(r, 300));
    expect(searchLinearIssuesMock).not.toHaveBeenCalled();
    expect(result.current.loading).toBe(false);
    expect(result.current.items).toEqual([]);
  });

  it("does not fetch when workspaceId is missing even if enabled", async () => {
    resetSearchMock();
    searchLinearIssuesMock.mockResolvedValue(emptyResult());
    renderHook(() => useLinearIssueSearch(undefined, "", "", "me", true), {
      wrapper: createWrapper(),
    });
    await new Promise((r) => setTimeout(r, 300));
    expect(searchLinearIssuesMock).not.toHaveBeenCalled();
  });

  it("fetches once enabled flips from false to true", async () => {
    resetSearchMock();
    searchLinearIssuesMock.mockResolvedValue(emptyResult());
    const { rerender } = renderHook(
      ({ enabled }: { enabled: boolean }) => useLinearIssueSearch(WORKSPACE, "", "", "me", enabled),
      { initialProps: { enabled: false }, wrapper: createWrapper() },
    );
    await new Promise((r) => setTimeout(r, 300));
    expect(searchLinearIssuesMock).not.toHaveBeenCalled();
    rerender({ enabled: true });
    await waitFor(() => expect(searchLinearIssuesMock).toHaveBeenCalled());
  });
});

describe("useLinearIssueSearch — fetch wiring", () => {
  it("loads issues when enabled and forwards workspace + filters", async () => {
    resetSearchMock();
    const issue = fakeIssue();
    searchLinearIssuesMock.mockResolvedValue({ issues: [issue], maxResults: 25, isLast: true });
    const { result } = renderHook(() => useLinearIssueSearch(WORKSPACE, "bug", "ENG", "me", true), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.items).toEqual([issue]));
    const [params, options] = searchLinearIssuesMock.mock.calls[0] as [
      Record<string, unknown>,
      Record<string, unknown>,
    ];
    expect(params.query).toBe("bug");
    expect(params.teamKey).toBe("ENG");
    expect(params.assigned).toBe("me");
    expect(options.workspaceId).toBe(WORKSPACE);
    expect(result.current.loading).toBe(false);
  });

  it("surfaces error message without items", async () => {
    resetSearchMock();
    searchLinearIssuesMock.mockRejectedValue(new Error("boom"));
    const { result } = renderHook(() => useLinearIssueSearch(WORKSPACE, "", "", "me", true), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.error).toBe("boom"));
    expect(result.current.items).toEqual([]);
  });

  it("uses the returned cursor to fetch the next page", async () => {
    resetSearchMock();
    const first = fakeIssue({ id: "1", identifier: "ENG-1" });
    const second = fakeIssue({ id: "2", identifier: "ENG-2" });
    searchLinearIssuesMock
      .mockResolvedValueOnce({
        issues: [first],
        maxResults: 25,
        isLast: false,
        nextPageToken: "cursor-2",
      })
      .mockResolvedValueOnce({ issues: [second], maxResults: 25, isLast: true });

    const { result } = renderHook(() => useLinearIssueSearch(WORKSPACE, "", "", "me", true), {
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.items).toEqual([first]));

    act(() => result.current.goNext());

    await waitFor(() => expect(result.current.items).toEqual([second]));
    expect(result.current.page).toBe(2);
    const [params] = searchLinearIssuesMock.mock.calls[1] as [Record<string, unknown>];
    expect(params.pageToken).toBe("cursor-2");
  });
});
