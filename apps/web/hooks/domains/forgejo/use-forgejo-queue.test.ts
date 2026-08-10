import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useForgejoQueue } from "./use-forgejo-queue";

const api = vi.hoisted(() => ({ list: vi.fn() }));
vi.mock("@/lib/api/domains/forgejo-api", () => ({ listForgejoQueue: api.list }));

describe("useForgejoQueue", () => {
  beforeEach(() => { vi.clearAllMocks(); });

  it("does not fetch until a workspace is available", () => {
    const { result } = renderHook(() => useForgejoQueue(undefined));
    expect(api.list).not.toHaveBeenCalled();
    expect(result.current.queue).toBeNull();
  });

  it("loads the workspace-scoped queue", async () => {
    api.list.mockResolvedValue({ issues: [{ repository: { full_name: "owner/repo" }, issue: { number: 1, title: "Issue" } }], pull_requests: [] });
    const { result } = renderHook(() => useForgejoQueue("workspace-a"));
    await waitFor(() => expect(result.current.queue?.issues).toHaveLength(1));
    expect(api.list).toHaveBeenCalledWith({ workspaceId: "workspace-a" });
    expect(result.current.error).toBeNull();
  });

  it("surfaces a load error", async () => {
    api.list.mockRejectedValue(new Error("offline"));
    const { result } = renderHook(() => useForgejoQueue("workspace-a"));
    await waitFor(() => expect(result.current.error).toBe("offline"));
  });
});
