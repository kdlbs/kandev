import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, waitFor } from "@testing-library/react";
import { createElement, StrictMode } from "react";
import { StateProvider, useAppStore } from "@/components/state-provider";
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

// Probe renders the hook and mirrors the resulting store state into the DOM so
// assertions can read it after StrictMode's mount -> cleanup -> mount cycle.
function Probe() {
  useReviewWatches("ws-1");
  const rw = useAppStore((state) => state.reviewWatches);
  return createElement(
    "div",
    { "data-testid": "probe" },
    JSON.stringify({ loaded: rw.loaded, loading: rw.loading, ids: rw.items.map((w) => w.id) }),
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

describe("useReviewWatches", () => {
  it("loads watches into the store when the effect double-mounts (StrictMode)", async () => {
    // The first mount's fetch never resolves; StrictMode cancels it. Only the
    // re-mount's fetch resolves, so the watch reaches the store ONLY if cleanup
    // cleared the scope guard and let the second effect re-issue the fetch.
    mocks.listReviewWatches
      .mockImplementationOnce(() => new Promise<{ watches: ReviewWatch[] }>(() => {}))
      .mockResolvedValue({ watches: [watch("w-1")] });

    const { getByTestId } = render(
      createElement(StrictMode, null, createElement(StateProvider, null, createElement(Probe))),
    );

    // StrictMode double-invokes the effect in dev, exercising the cleanup path.
    await waitFor(() => expect(mocks.listReviewWatches.mock.calls.length).toBeGreaterThan(1));

    await waitFor(() => {
      const state = readProbe(getByTestId("probe"));
      expect(state.loaded).toBe(true);
      expect(state.ids).toEqual(["w-1"]);
      expect(state.loading).toBe(false);
    });
  });

  it("does not fetch when workspaceId is null", () => {
    render(createElement(StateProvider, null, createElement(NullProbe)));
    expect(mocks.listReviewWatches).not.toHaveBeenCalled();
  });
});
