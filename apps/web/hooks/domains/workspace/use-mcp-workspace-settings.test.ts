import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { listMCPServers } from "@/lib/api/domains/mcp-api";
import type { MCPServerDefinition } from "@/lib/types/http-mcp";
import { useMCPWorkspaceDefinitions } from "./use-mcp-workspace-settings";

vi.mock("@/lib/api/domains/mcp-api", () => ({ listMCPServers: vi.fn() }));
beforeEach(() => vi.resetAllMocks());
const server: MCPServerDefinition = {
  id: "server-a",
  workspace_id: "workspace-a",
  runtime_name: "docs",
  normalized_runtime_name: "docs",
  display_name: "Documentation",
  enabled: true,
  execution_mode: "remote",
  transport: "streamable_http",
  configuration: { url: "https://example.test/mcp" },
  source: "custom",
  revision: 1,
  created_at: "2026-10-05",
  updated_at: "2026-10-05",
};

it("masks old catalog while a new workspace loads and ignores old completion", async () => {
  vi.mocked(listMCPServers).mockResolvedValueOnce([server]);
  const { result, rerender } = renderHook(
    ({ workspace }) => useMCPWorkspaceDefinitions(workspace),
    { initialProps: { workspace: "workspace-a" } },
  );
  await waitFor(() => expect(result.current.definitions).toEqual([server]));
  let resolveOld!: (value: MCPServerDefinition[]) => void;
  let resolveNew!: (value: MCPServerDefinition[]) => void;
  vi.mocked(listMCPServers)
    .mockReturnValueOnce(
      new Promise((resolve) => {
        resolveOld = resolve;
      }),
    )
    .mockReturnValueOnce(
      new Promise((resolve) => {
        resolveNew = resolve;
      }),
    );
  let old!: Promise<unknown>;
  act(() => {
    old = result.current.reload();
  });
  rerender({ workspace: "workspace-b" });
  expect(result.current.definitions).toEqual([]);
  await act(async () => {
    resolveOld([server]);
    await old;
  });
  expect(result.current.definitions).toEqual([]);
  await act(async () => {
    resolveNew([]);
  });
  expect(result.current.loading).toBe(false);
});

it("retains a successful empty inventory during refresh and recovers after failure", async () => {
  vi.mocked(listMCPServers).mockResolvedValueOnce([]);
  const { result } = renderHook(() => useMCPWorkspaceDefinitions("workspace-a"));
  await waitFor(() => expect(result.current.loading).toBe(false));
  let reject!: (cause: Error) => void;
  vi.mocked(listMCPServers).mockReturnValueOnce(
    new Promise((_resolve, no) => {
      reject = no;
    }),
  );
  let pending!: Promise<unknown>;
  act(() => {
    pending = result.current.reload();
  });
  expect(result.current.loading).toBe(false);
  const error = new Error("refresh failed");
  await act(async () => {
    reject(error);
    await pending;
  });
  expect(result.current.error).toBe(error);
  vi.mocked(listMCPServers).mockResolvedValueOnce([server]);
  await act(async () => {
    await result.current.reload();
  });
  expect(result.current.definitions).toEqual([server]);
  expect(result.current.error).toBeNull();
});
