import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Command, CommandList } from "@kandev/ui/command";
import { StateProvider } from "@/components/state-provider";
import { WorkspaceContentSearch } from "@/components/workspace-content-search";
import { useWorkspaceContentSearch } from "./use-workspace-content-search";

const { request } = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request }),
}));

function SearchConsumer() {
  const state = useWorkspaceContentSearch({
    enabled: true,
    query: "needle",
    sessionId: "session-1",
  });
  return (
    <StateProvider>
      <Command shouldFilter={false}>
        <CommandList>
          <WorkspaceContentSearch
            {...state}
            search="needle"
            sessionId="session-1"
            onSelect={() => {}}
          />
        </CommandList>
      </Command>
    </StateProvider>
  );
}

beforeEach(() => {
  vi.useFakeTimers();
  request.mockReset();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

// @covers AC-UI-TASK-WORKSPACE-CONTENT-SEARCH-001.10
// Integration boundary: actual hook, WS wrapper and translated consumer rows.
describe("workspace content-search recovery presentation", () => {
  it("shows no matches after the transport recovers from a temporary failure", async () => {
    request
      .mockRejectedValueOnce(new Error("temporary disconnect"))
      .mockResolvedValue({ results: [] });
    render(<SearchConsumer />);
    expect(screen.getByText("Searching task workspace…")).toBeTruthy();

    await act(async () => vi.runAllTimersAsync());

    expect(request).toHaveBeenCalledTimes(8);
    expect(screen.getByText("No content matches found.")).toBeTruthy();
    expect(
      screen.queryByText("Search failed. Edit the query or reopen search to retry."),
    ).toBeNull();
    expect(screen.queryByText("Searching task workspace…")).toBeNull();
  });

  it.each(["all", "terminal"] as const)(
    "keeps %s failures visible without matches",
    async (failure) => {
      if (failure === "terminal") {
        for (let attempt = 0; attempt < 7; attempt += 1) {
          request.mockResolvedValueOnce({ results: [] });
        }
      }
      request.mockRejectedValue(new Error("disconnected"));
      render(<SearchConsumer />);

      await act(async () => vi.runAllTimersAsync());

      expect(request).toHaveBeenCalledTimes(8);
      expect(
        screen.getByText("Search failed. Edit the query or reopen search to retry."),
      ).toBeTruthy();
      expect(screen.queryByText("No content matches found.")).toBeNull();
    },
  );
});
