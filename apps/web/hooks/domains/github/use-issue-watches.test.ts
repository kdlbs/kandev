import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, waitFor } from "@testing-library/react";
import { createElement, StrictMode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { IssueWatch } from "@/lib/types/github";
import { useIssueWatches } from "./use-issue-watches";

const mocks = vi.hoisted(() => ({ listIssueWatches: vi.fn() }));

vi.mock("@/lib/api/domains/github-api", () => ({
  listIssueWatches: mocks.listIssueWatches,
  createIssueWatch: vi.fn(),
  updateIssueWatch: vi.fn(),
  deleteIssueWatch: vi.fn(),
  triggerIssueWatch: vi.fn(),
  triggerAllIssueWatches: vi.fn(),
  previewResetIssueWatch: vi.fn(),
  resetIssueWatch: vi.fn(),
}));

afterEach(() => {
  cleanup();
  mocks.listIssueWatches.mockReset();
});

function watch(id: string): IssueWatch {
  return {
    id,
    workspace_id: "ws-1",
    workflow_id: "wf-1",
    workflow_step_id: "step-1",
    repos: [{ owner: "acme", name: "" }],
    agent_profile_id: "agent-1",
    executor_profile_id: "exec-1",
    prompt: "",
    labels: [],
    custom_query: "",
    enabled: true,
    poll_interval_seconds: 300,
    cleanup_policy: "auto",
    last_polled_at: null,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

// Probe renders the hook and mirrors the resulting query state into the DOM so
// assertions can read it after StrictMode's mount -> cleanup -> mount cycle.
function Probe() {
  const watches = useIssueWatches("ws-1");
  return createElement(
    "div",
    { "data-testid": "probe" },
    JSON.stringify({
      loaded: watches.loaded,
      loading: watches.loading,
      ids: watches.items.map((watch) => watch.id),
    }),
  );
}

function NullProbe() {
  useIssueWatches(null);
  return null;
}

function readProbe(el: HTMLElement) {
  return JSON.parse(el.textContent ?? "{}") as {
    loaded: boolean;
    loading: boolean;
    ids: string[];
  };
}

function createQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
}

function renderWithQuery(child: ReturnType<typeof createElement>) {
  return render(
    createElement(
      QueryClientProvider,
      { client: createQueryClient() },
      createElement(StrictMode, null, child),
    ),
  );
}

describe("useIssueWatches", () => {
  it("reuses and publishes the in-flight query across a StrictMode remount", async () => {
    let resolveRequest!: (response: { watches: IssueWatch[] }) => void;
    mocks.listIssueWatches.mockImplementationOnce(
      () =>
        new Promise<{ watches: IssueWatch[] }>((resolve) => {
          resolveRequest = resolve;
        }),
    );

    const { getByTestId } = renderWithQuery(createElement(Probe));

    await waitFor(() => expect(mocks.listIssueWatches).toHaveBeenCalledTimes(1));
    await act(async () => resolveRequest({ watches: [watch("w-1")] }));

    await waitFor(() => {
      const state = readProbe(getByTestId("probe"));
      expect(state.loaded).toBe(true);
      expect(state.ids).toEqual(["w-1"]);
      expect(state.loading).toBe(false);
    });
  });

  it("does not fetch when workspaceId is null", () => {
    renderWithQuery(createElement(NullProbe));
    expect(mocks.listIssueWatches).not.toHaveBeenCalled();
  });
});
