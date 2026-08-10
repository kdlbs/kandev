import { createElement, type ReactNode } from "react";
import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { useForgejoConfig } from "./use-forgejo-config";

const api = vi.hoisted(() => ({ get: vi.fn(), save: vi.fn(), refresh: vi.fn(), remove: vi.fn() }));
vi.mock("@/lib/api/domains/forgejo-api", () => ({
  getForgejoConfig: api.get,
  setForgejoConfig: api.save,
  refreshForgejoConnection: api.refresh,
  deleteForgejoConfig: api.remove,
}));

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}

const config = {
  workspace_id: "ws-a", origin: "https://forgejo.example", username: "alice", has_secret: true,
  has_webhook_secret: false, last_ok: true, created_at: "", updated_at: "",
};

describe("useForgejoConfig", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.get.mockResolvedValue(config);
  });

  it("loads and saves config in the workspace cache", async () => {
    api.save.mockResolvedValue({ ...config, username: "bob" });
    const { result } = renderHook(() => useForgejoConfig("ws-a"), { wrapper });
    await waitFor(() => expect(result.current.config?.username).toBe("alice"));

    await result.current.save({ origin: config.origin, token: "new-token" });

    expect(api.save).toHaveBeenCalledWith(
      { origin: config.origin, token: "new-token" },
      { workspaceId: "ws-a" },
    );
    expect(result.current.config?.username).toBe("bob");
  });

  it("resets the complete workspace cache after disconnect", async () => {
    api.remove.mockResolvedValue({ deleted: true });
    const { result } = renderHook(() => useForgejoConfig("ws-a"), { wrapper });
    await waitFor(() => expect(result.current.config).toEqual(config));

    await result.current.disconnect();

    expect(api.remove).toHaveBeenCalledWith({ workspaceId: "ws-a" });
    expect(result.current.config).toBeNull();
  });
});
