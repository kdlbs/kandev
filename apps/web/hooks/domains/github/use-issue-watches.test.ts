import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, waitFor } from "@testing-library/react";
import { createElement, StrictMode } from "react";
import { StateProvider, useAppStore } from "@/components/state-provider";
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

// Probe renders the hook and mirrors the resulting store state into the DOM so
// assertions can read it after StrictMode's mount -> cleanup -> mount cycle.
function Probe() {
  useIssueWatches("ws-1");
  const iw = useAppStore((state) => state.issueWatches);
  return createElement(
    "div",
    { "data-testid": "probe" },
    JSON.stringify({ loaded: iw.loaded, loading: iw.loading, ids: iw.items.map((w) => w.id) }),
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

describe("useIssueWatches", () => {
  it("loads watches into the store when the effect double-mounts (StrictMode)", async () => {
    // The first mount's fetch never resolves; StrictMode cancels it. Only the
    // re-mount's fetch resolves, so the watch reaches the store ONLY if cleanup
    // cleared the scope guard and let the second effect re-issue the fetch.
    mocks.listIssueWatches
      .mockImplementationOnce(() => new Promise<{ watches: IssueWatch[] }>(() => {}))
      .mockResolvedValue({ watches: [watch("w-1")] });

    const { getByTestId } = render(
      createElement(StrictMode, null, createElement(StateProvider, null, createElement(Probe))),
    );

    // StrictMode double-invokes the effect in dev, exercising the cleanup path.
    await waitFor(() => expect(mocks.listIssueWatches.mock.calls.length).toBeGreaterThan(1));

    await waitFor(() => {
      const state = readProbe(getByTestId("probe"));
      expect(state.loaded).toBe(true);
      expect(state.ids).toEqual(["w-1"]);
      expect(state.loading).toBe(false);
    });
  });

  it("does not fetch when workspaceId is null", () => {
    render(createElement(StateProvider, null, createElement(NullProbe)));
    expect(mocks.listIssueWatches).not.toHaveBeenCalled();
  });
});
