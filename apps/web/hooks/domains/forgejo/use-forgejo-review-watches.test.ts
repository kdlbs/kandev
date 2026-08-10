import { createElement, type ReactNode } from "react";
import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { useForgejoReviewWatches } from "./use-forgejo-review-watches";

const api = vi.hoisted(() => ({ list: vi.fn(), save: vi.fn(), remove: vi.fn(), poll: vi.fn() }));
vi.mock("@/lib/api/domains/forgejo-api", () => ({
  listForgejoReviewWatches: api.list,
  saveForgejoReviewWatch: api.save,
  deleteForgejoReviewWatch: api.remove,
  pollForgejoReviewWatch: api.poll,
}));
function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}
const watch = {
  id: "review-a",
  workspace_id: "ws-a",
  workflow_id: "workflow",
  workflow_step_id: "",
  repository_id: "",
  base_branch: "",
  prompt: "",
  agent_profile_id: "",
  owner: "owner",
  repo: "repo",
  enabled: true,
  poll_interval_seconds: 300,
};

describe("useForgejoReviewWatches", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.list.mockResolvedValue({ watches: [watch] });
  });
  it("uses a workspace-scoped cached list and refreshes after polling", async () => {
    api.poll.mockResolvedValue({ pull_requests: [] });
    const { result } = renderHook(() => useForgejoReviewWatches("ws-a"), { wrapper });
    await waitFor(() => expect(result.current.watches).toEqual([watch]));
    await result.current.poll(watch.id);
    expect(api.poll).toHaveBeenCalledWith(watch.id, { workspaceId: "ws-a" });
    expect(api.list).toHaveBeenCalledTimes(2);
  });
});
