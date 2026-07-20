import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useGitLabReviewWatches } from "./use-gitlab-review-watches";
import type { ReviewWatch } from "@/lib/types/gitlab";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
  trigger: vi.fn(),
  triggerAll: vi.fn(),
  previewReset: vi.fn(),
  reset: vi.fn(),
}));

const state: {
  gitlabReviewWatches: { items: ReviewWatch[]; loaded: boolean; loading: boolean };
  setGitLabReviewWatches: ReturnType<typeof vi.fn>;
  setGitLabReviewWatchesLoading: ReturnType<typeof vi.fn>;
  addGitLabReviewWatch: ReturnType<typeof vi.fn>;
  updateGitLabReviewWatchInStore: ReturnType<typeof vi.fn>;
  removeGitLabReviewWatch: ReturnType<typeof vi.fn>;
} = {
  gitlabReviewWatches: { items: [], loaded: true, loading: false },
  setGitLabReviewWatches: vi.fn(),
  setGitLabReviewWatchesLoading: vi.fn(),
  addGitLabReviewWatch: vi.fn(),
  updateGitLabReviewWatchInStore: vi.fn(),
  removeGitLabReviewWatch: vi.fn(),
};

function reviewWatch(id: string, workspaceId: string): ReviewWatch {
  return {
    id,
    workspace_id: workspaceId,
    workflow_id: "workflow",
    workflow_step_id: "step",
    projects: [],
    agent_profile_id: "",
    executor_profile_id: "",
    prompt: "review",
    review_scope: "user",
    custom_query: "",
    enabled: true,
    poll_interval_seconds: 300,
    cleanup_policy: "auto",
    created_at: "2026-01-01",
    updated_at: "2026-01-01",
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: typeof state) => unknown) => selector(state),
}));
vi.mock("@/lib/api/domains/gitlab-api", () => ({
  listReviewWatches: api.list,
  createReviewWatch: api.create,
  updateReviewWatch: api.update,
  deleteReviewWatch: api.remove,
  triggerReviewWatch: api.trigger,
  triggerAllReviewWatches: api.triggerAll,
  previewResetReviewWatch: api.previewReset,
  resetReviewWatch: api.reset,
}));

describe("useGitLabReviewWatches", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    state.gitlabReviewWatches.items = [];
    api.list.mockResolvedValue({ watches: [] });
    state.updateGitLabReviewWatchInStore.mockImplementation((watch: ReviewWatch) => {
      state.gitlabReviewWatches.items = [watch];
    });
  });

  it("forwards the bound workspace to row actions and reset", async () => {
    api.update.mockResolvedValue({ id: "watch-1" });
    api.reset.mockResolvedValue({ tasksDeleted: 2 });
    const { result } = renderHook(() => useGitLabReviewWatches("ws-1"));

    await act(() => result.current.update("watch-1", { enabled: false }));
    await act(() => result.current.previewReset("watch-1"));
    await act(() => result.current.reset("watch-1"));

    expect(api.update).toHaveBeenCalledWith("watch-1", "ws-1", { enabled: false });
    expect(api.previewReset).toHaveBeenCalledWith("watch-1", "ws-1");
    expect(api.reset).toHaveBeenCalledWith("watch-1", "ws-1");
  });

  it("hides workspace A rows synchronously when switching to workspace B", () => {
    state.gitlabReviewWatches.items = [reviewWatch("a", "ws-a")];
    const { result, rerender } = renderHook(
      ({ workspaceId }) => useGitLabReviewWatches(workspaceId),
      { initialProps: { workspaceId: "ws-a" } },
    );
    expect(result.current.items.map((watch) => watch.id)).toEqual(["a"]);

    rerender({ workspaceId: "ws-b" });
    expect(result.current.items).toEqual([]);
  });

  it("ignores delayed workspace A list and mutation responses after switching to B", async () => {
    const listA = deferred<{ watches: ReviewWatch[] }>();
    const listB = deferred<{ watches: ReviewWatch[] }>();
    const updateA = deferred<ReviewWatch>();
    api.list.mockImplementation((workspaceId: string) =>
      workspaceId === "ws-a" ? listA.promise : listB.promise,
    );
    api.update.mockReturnValue(updateA.promise);
    const { result, rerender } = renderHook(
      ({ workspaceId }) => useGitLabReviewWatches(workspaceId),
      { initialProps: { workspaceId: "ws-a" } },
    );
    let mutation!: Promise<ReviewWatch>;
    act(() => {
      mutation = result.current.update("a", { enabled: false });
    });
    rerender({ workspaceId: "ws-b" });

    await act(async () => {
      listA.resolve({ watches: [reviewWatch("a", "ws-a")] });
      updateA.resolve(reviewWatch("a", "ws-a"));
      listB.resolve({ watches: [reviewWatch("b", "ws-b")] });
      await mutation;
    });
    state.gitlabReviewWatches.items = [reviewWatch("a", "ws-a")];
    rerender({ workspaceId: "ws-b" });

    expect(result.current.items).toEqual([]);
    expect(state.setGitLabReviewWatches).not.toHaveBeenCalledWith([
      expect.objectContaining({ workspace_id: "ws-a" }),
    ]);
  });
});
