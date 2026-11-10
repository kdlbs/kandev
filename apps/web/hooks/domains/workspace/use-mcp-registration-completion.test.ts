import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import {
  createMCPServer,
  installMCPMarketplaceEntry,
  listMCPServers,
  searchMCPMarketplace,
} from "@/lib/api/domains/mcp-api";
import { ApiError } from "@/lib/api/client";
import type { MCPDefinitionInput, MCPServerDefinition } from "@/lib/types/http-mcp";
import { useMCPWorkspaceSettings } from "./use-mcp-workspace-settings";

vi.mock("@/lib/api/domains/mcp-api", () => ({
  createMCPServer: vi.fn(),
  installMCPMarketplaceEntry: vi.fn(),
  listMCPServers: vi.fn(),
  searchMCPMarketplace: vi.fn(),
  updateMCPServer: vi.fn(),
  deleteMCPServer: vi.fn(),
  refreshMCPMarketplace: vi.fn(),
}));
const WORKSPACE_A = "workspace-a";
const input: MCPDefinitionInput = {
  runtime_name: "docs",
  display_name: "Docs",
  execution_mode: "remote",
  transport: "streamable_http",
  configuration: { url: "https://example.test" },
};
const saved = {
  ...input,
  id: "definition-1",
  workspace_id: WORKSPACE_A,
  revision: 1,
  enabled: true,
} as MCPServerDefinition;
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(listMCPServers).mockResolvedValue([]);
  vi.mocked(searchMCPMarketplace).mockResolvedValue({ entries: [], stale: false, degraded: false });
  vi.mocked(createMCPServer).mockResolvedValue(saved);
  vi.mocked(installMCPMarketplaceEntry).mockResolvedValue(saved);
});
it.each(["custom", "marketplace"])(
  "keeps %s saved-only completion and deduplicates its catalog identity",
  async (source) => {
    vi.mocked(listMCPServers).mockResolvedValue([saved]);
    const { result } = renderHook(() => useMCPWorkspaceSettings(WORKSPACE_A));
    await waitFor(() => expect(result.current.loading).toBe(false));
    await act(async () => {
      if (source === "custom") await result.current.create(input);
      else
        await result.current.install({
          identity: "example@1",
          expected_revision: 1,
          choice_id: "remote",
          runtime_name: "docs",
        });
    });
    expect(result.current.servers).toEqual([saved]);
    expect(result.current.registration.receipt).toEqual({
      workspaceId: WORKSPACE_A,
      definitionId: saved.id,
      displayName: "Docs",
    });
    expect(JSON.stringify(result.current.registration.receipt)).not.toContain("https://");
    act(() => result.current.registration.dismiss());
    expect(result.current.registration.receipt).toBeNull();
    expect(result.current.servers).toEqual([saved]);
  },
);
it("blocks same-tick duplicate creation", async () => {
  const pending = deferred<MCPServerDefinition>();
  vi.mocked(createMCPServer).mockReturnValue(pending.promise);
  const { result } = renderHook(() => useMCPWorkspaceSettings(WORKSPACE_A));
  await waitFor(() => expect(result.current.loading).toBe(false));
  let first!: Promise<unknown>;
  act(() => {
    first = result.current.create(input);
    void result.current.create(input);
  });
  expect(createMCPServer).toHaveBeenCalledTimes(1);
  await act(async () => {
    pending.resolve(saved);
    await first;
  });
});
it("ignores a late create and old catalog load after leaving and returning to a workspace", async () => {
  const create = deferred<MCPServerDefinition>();
  const load = deferred<MCPServerDefinition[]>();
  vi.mocked(createMCPServer).mockReturnValue(create.promise);
  const { result, rerender } = renderHook(({ id }) => useMCPWorkspaceSettings(id), {
    initialProps: { id: WORKSPACE_A },
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  vi.mocked(listMCPServers).mockReturnValueOnce(load.promise);
  let first!: Promise<unknown>;
  let reload!: Promise<unknown>;
  act(() => {
    first = result.current.create(input);
    reload = result.current.reload();
  });
  rerender({ id: "workspace-b" });
  rerender({ id: WORKSPACE_A });
  await act(async () => {
    create.resolve(saved);
    load.resolve([saved]);
    await Promise.all([first, reload]);
  });
  expect(result.current.servers).toEqual([]);
  expect(result.current.registration.receipt).toBeNull();
});
it("requires a successful catalog read after uncertain ACK and never replays the create", async () => {
  vi.mocked(createMCPServer).mockRejectedValue(new TypeError("network lost"));
  const { result } = renderHook(() => useMCPWorkspaceSettings(WORKSPACE_A));
  await waitFor(() => expect(result.current.loading).toBe(false));
  await act(async () => {
    await result.current.create(input);
  });
  expect(result.current.registration.uncertain).toBe(true);
  expect(result.current.registration.receipt).toBeNull();
  await act(async () => {
    await result.current.create(input);
  });
  expect(createMCPServer).toHaveBeenCalledTimes(1);
  vi.mocked(listMCPServers).mockRejectedValueOnce(new Error("offline"));
  await act(async () => {
    await result.current.reviewRegistration();
  });
  expect(result.current.registration.uncertain).toBe(true);
  vi.mocked(listMCPServers).mockResolvedValue([saved]);
  await act(async () => {
    await result.current.reviewRegistration();
  });
  expect(result.current.registration.uncertain).toBe(false);
  expect(result.current.servers).toEqual([saved]);
  expect(result.current.registration.receipt).toBeNull();
  expect(createMCPServer).toHaveBeenCalledTimes(1);
});
it("keeps explicit validation rejection separate from uncertain success", async () => {
  const error = new ApiError("invalid", 400, {});
  vi.mocked(createMCPServer).mockRejectedValue(error);
  const { result } = renderHook(() => useMCPWorkspaceSettings(WORKSPACE_A));
  await waitFor(() => expect(result.current.loading).toBe(false));
  await act(async () => {
    await expect(result.current.create(input)).rejects.toBe(error);
  });
  expect(result.current.registration.uncertain).toBe(false);
});
it("does not expose an old workspace catalog-review failure in the new workspace", async () => {
  const pending = deferred<MCPServerDefinition[]>();
  const { result, rerender } = renderHook(({ id }) => useMCPWorkspaceSettings(id), {
    initialProps: { id: WORKSPACE_A },
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  vi.mocked(listMCPServers).mockImplementationOnce(() =>
    pending.promise.then(() => {
      throw new Error("old context");
    }),
  );
  let review!: Promise<unknown>;
  act(() => {
    review = result.current.reviewRegistration();
  });
  rerender({ id: "workspace-b" });
  await waitFor(() => expect(result.current.loading).toBe(false));
  await act(async () => {
    pending.resolve([]);
    await review;
  });
  expect(result.current.error).toBeNull();
});
it("does not let a catalog read started before registration erase its acknowledged definition", async () => {
  const pending = deferred<MCPServerDefinition[]>();
  const { result } = renderHook(() => useMCPWorkspaceSettings(WORKSPACE_A));
  await waitFor(() => expect(result.current.loading).toBe(false));
  vi.mocked(listMCPServers).mockReturnValueOnce(pending.promise);
  let reload!: Promise<unknown>;
  act(() => {
    reload = result.current.reload();
  });
  await act(async () => {
    await result.current.create(input);
  });
  await act(async () => {
    pending.resolve([]);
    await reload;
  });
  expect(result.current.servers).toEqual([saved]);
});
