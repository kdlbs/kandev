import { createElement, type ReactNode } from "react";
import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useForgejoQueue } from "./use-forgejo-queue";
import { StateProvider } from "@/components/state-provider";

const api = vi.hoisted(() => ({ list: vi.fn() }));
vi.mock("@/lib/api/domains/forgejo-api", () => ({ listForgejoQueue: api.list }));

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}

describe("useForgejoQueue", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("does not fetch until a workspace is available", () => {
    const { result } = renderHook(() => useForgejoQueue(undefined), { wrapper });
    expect(api.list).not.toHaveBeenCalled();
    expect(result.current.queue).toBeNull();
  });

  it("loads the workspace-scoped queue", async () => {
    api.list.mockResolvedValue({ issues: [{ repository: { full_name: "owner/repo" }, issue: { number: 1, title: "Issue" } }], pull_requests: [] });
    const { result } = renderHook(() => useForgejoQueue("workspace-a"), { wrapper });
    await waitFor(() => expect(result.current.queue?.issues).toHaveLength(1));
    expect(api.list).toHaveBeenCalledWith({ workspaceId: "workspace-a" });
    expect(result.current.error).toBeNull();
  });

  it("surfaces a load error", async () => {
    api.list.mockRejectedValue(new Error("offline"));
    const { result } = renderHook(() => useForgejoQueue("workspace-a"), { wrapper });
    await waitFor(() => expect(result.current.error).toBe("offline"));
  });
});
