import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, waitFor } from "@testing-library/react";
import { createElement, StrictMode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReviewWatch } from "@/lib/types/github";
import { useReviewWatches } from "./use-review-watches";

const mocks = vi.hoisted(() => ({ listReviewWatches: vi.fn() }));

vi.mock("@/lib/api/domains/github-api", () => ({
  listReviewWatches: mocks.listReviewWatches,
  createReviewWatch: vi.fn(),
  updateReviewWatch: vi.fn(),
  deleteReviewWatch: vi.fn(),
  triggerReviewWatch: vi.fn(),
  triggerAllReviewWatches: vi.fn(),
  previewResetReviewWatch: vi.fn(),
  resetReviewWatch: vi.fn(),
}));

afterEach(() => {
  cleanup();
  mocks.listReviewWatches.mockReset();
});

function watch(id: string): ReviewWatch {
  return {
    id,
    workspace_id: "ws-1",
    workflow_id: "wf-1",
    workflow_step_id: "step-1",
    repos: [{ owner: "acme", name: "" }],
    agent_profile_id: "agent-1",
    executor_profile_id: "exec-1",
    prompt: "",
    review_scope: "user_and_teams",
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
  const watches = useReviewWatches("ws-1");
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
  useReviewWatches(null);
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

describe("useReviewWatches", () => {
  it("reuses and publishes the in-flight query across a StrictMode remount", async () => {
    let resolveRequest!: (response: { watches: ReviewWatch[] }) => void;
    mocks.listReviewWatches.mockImplementationOnce(
      () =>
        new Promise<{ watches: ReviewWatch[] }>((resolve) => {
          resolveRequest = resolve;
        }),
    );

    const { getByTestId } = renderWithQuery(createElement(Probe));

    await waitFor(() => expect(mocks.listReviewWatches).toHaveBeenCalledTimes(1));
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
    expect(mocks.listReviewWatches).not.toHaveBeenCalled();
  });
});
