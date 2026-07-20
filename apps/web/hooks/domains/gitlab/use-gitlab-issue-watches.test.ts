import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useGitLabIssueWatches } from "./use-gitlab-issue-watches";
import type { IssueWatch } from "@/lib/types/gitlab";

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
  gitlabIssueWatches: { items: IssueWatch[]; loaded: boolean; loading: boolean };
  setGitLabIssueWatches: ReturnType<typeof vi.fn>;
  setGitLabIssueWatchesLoading: ReturnType<typeof vi.fn>;
  addGitLabIssueWatch: ReturnType<typeof vi.fn>;
  updateGitLabIssueWatchInStore: ReturnType<typeof vi.fn>;
  removeGitLabIssueWatch: ReturnType<typeof vi.fn>;
} = {
  gitlabIssueWatches: { items: [], loaded: true, loading: false },
  setGitLabIssueWatches: vi.fn(),
  setGitLabIssueWatchesLoading: vi.fn(),
  addGitLabIssueWatch: vi.fn(),
  updateGitLabIssueWatchInStore: vi.fn(),
  removeGitLabIssueWatch: vi.fn(),
};

function issueWatch(id: string, workspaceId: string): IssueWatch {
  return {
    id,
    workspace_id: workspaceId,
    workflow_id: "workflow",
    workflow_step_id: "step",
    projects: [],
    agent_profile_id: "",
    executor_profile_id: "",
    prompt: "fix",
    labels: [],
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
  listIssueWatches: api.list,
  createIssueWatch: api.create,
  updateIssueWatch: api.update,
  deleteIssueWatch: api.remove,
  triggerIssueWatch: api.trigger,
  triggerAllIssueWatches: api.triggerAll,
  previewResetIssueWatch: api.previewReset,
  resetIssueWatch: api.reset,
}));

describe("useGitLabIssueWatches", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    state.gitlabIssueWatches.items = [];
    api.list.mockResolvedValue({ watches: [] });
    state.updateGitLabIssueWatchInStore.mockImplementation((watch: IssueWatch) => {
      state.gitlabIssueWatches.items = [watch];
    });
  });

  it("accepts the row workspace when used outside a bound workspace", async () => {
    state.gitlabIssueWatches.items = [issueWatch("hidden", "ws-other")];
    api.remove.mockResolvedValue({ deleted: true });
    api.trigger.mockResolvedValue({ count: 0, issues: [] });
    const { result } = renderHook(() => useGitLabIssueWatches(null));

    expect(result.current.items).toEqual([]);

    await act(() => result.current.remove("watch-2", "ws-row"));
    await act(() => result.current.trigger("watch-2", "ws-row"));

    expect(api.remove).toHaveBeenCalledWith("watch-2", "ws-row");
    expect(api.trigger).toHaveBeenCalledWith("watch-2", "ws-row");
  });

  it("hides workspace A rows synchronously when switching to workspace B", () => {
    state.gitlabIssueWatches.items = [issueWatch("a", "ws-a")];
    const { result, rerender } = renderHook(
      ({ workspaceId }) => useGitLabIssueWatches(workspaceId),
      { initialProps: { workspaceId: "ws-a" } },
    );

    rerender({ workspaceId: "ws-b" });
    expect(result.current.items).toEqual([]);
  });

  it("ignores delayed workspace A list and mutation responses after switching to B", async () => {
    const listA = deferred<{ watches: IssueWatch[] }>();
    const listB = deferred<{ watches: IssueWatch[] }>();
    const updateA = deferred<IssueWatch>();
    api.list.mockImplementation((workspaceId: string) =>
      workspaceId === "ws-a" ? listA.promise : listB.promise,
    );
    api.update.mockReturnValue(updateA.promise);
    const { result, rerender } = renderHook(
      ({ workspaceId }) => useGitLabIssueWatches(workspaceId),
      { initialProps: { workspaceId: "ws-a" } },
    );
    let mutation!: Promise<IssueWatch>;
    act(() => {
      mutation = result.current.update("a", { enabled: false });
    });
    rerender({ workspaceId: "ws-b" });

    await act(async () => {
      listA.resolve({ watches: [issueWatch("a", "ws-a")] });
      updateA.resolve(issueWatch("a", "ws-a"));
      listB.resolve({ watches: [issueWatch("b", "ws-b")] });
      await mutation;
    });
    state.gitlabIssueWatches.items = [issueWatch("a", "ws-a")];
    rerender({ workspaceId: "ws-b" });

    expect(result.current.items).toEqual([]);
    expect(state.setGitLabIssueWatches).not.toHaveBeenCalledWith([
      expect.objectContaining({ workspace_id: "ws-a" }),
    ]);
  });
});
