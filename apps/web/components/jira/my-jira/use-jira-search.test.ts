import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { useJiraSearch } from "./use-jira-search";

const mocks = vi.hoisted(() => ({ searchJiraTickets: vi.fn() }));

vi.mock("@/lib/api/domains/jira-api", () => ({
  searchJiraTickets: (...args: unknown[]) => mocks.searchJiraTickets(...args),
}));

describe("useJiraSearch", () => {
  beforeEach(() => {
    mocks.searchJiraTickets.mockReset().mockResolvedValue({ tickets: [], isLast: true });
  });

  it("waits until the first view selection is resolved before searching", async () => {
    const { rerender } = renderHook(
      ({ enabled }: { enabled: boolean }) =>
        useJiraSearch("workspace-1", "project = CLIP", enabled),
      { initialProps: { enabled: false } },
    );

    expect(mocks.searchJiraTickets).not.toHaveBeenCalled();
    rerender({ enabled: true });

    await waitFor(() => expect(mocks.searchJiraTickets).toHaveBeenCalledOnce());
    expect(mocks.searchJiraTickets).toHaveBeenCalledWith(
      { jql: "project = CLIP", pageToken: "", maxResults: 25 },
      { workspaceId: "workspace-1" },
    );
  });
});
