import { createElement, type ReactNode } from "react";
import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { useForgejoIssueWatches } from "./use-forgejo-issue-watches";

const api = vi.hoisted(() => ({ list: vi.fn(), save: vi.fn(), remove: vi.fn(), poll: vi.fn() }));
vi.mock("@/lib/api/domains/forgejo-api", () => ({
  listForgejoIssueWatches: api.list,
  saveForgejoIssueWatch: api.save,
  deleteForgejoIssueWatch: api.remove,
  pollForgejoIssueWatch: api.poll,
}));

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}

const watch = {
  id: "watch-a", workspace_id: "ws-a", workflow_id: "workflow", workflow_step_id: "",
  repository_id: "", base_branch: "", prompt: "", agent_profile_id: "", executor_profile_id: "",
  cleanup_policy: "auto", inflight_limit: 0, owner: "owner", repo: "repo", labels: "",
  enabled: true, poll_interval_seconds: 300,
};

describe("useForgejoIssueWatches", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.list.mockResolvedValue({ watches: [watch] });
  });

  it("loads watches into the workspace-scoped store", async () => {
    const { result } = renderHook(() => useForgejoIssueWatches("ws-a"), { wrapper });
    await waitFor(() => expect(result.current.watches).toEqual([watch]));
    expect(api.list).toHaveBeenCalledWith({ workspaceId: "ws-a" });
  });

  it("refreshes the cached list after polling and deletion", async () => {
    api.poll.mockResolvedValue({ issues: [] });
    api.remove.mockResolvedValue({ deleted: true });
    const { result } = renderHook(() => useForgejoIssueWatches("ws-a"), { wrapper });
    await waitFor(() => expect(result.current.watches).toEqual([watch]));

    await result.current.poll(watch.id);
    await result.current.remove(watch.id);

    expect(api.poll).toHaveBeenCalledWith(watch.id, { workspaceId: "ws-a" });
    expect(api.remove).toHaveBeenCalledWith(watch.id, { workspaceId: "ws-a" });
    expect(api.list).toHaveBeenCalledTimes(3);
  });
});
