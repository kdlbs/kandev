import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { getMCPProfileOwner } from "@/lib/api/domains/mcp-api";
import { useMCPProfileOwner } from "./use-mcp-profile-owner";
import type { MCPProfileOwner } from "@/lib/types/http-mcp";
vi.mock("@/lib/api/domains/mcp-api", () => ({ getMCPProfileOwner: vi.fn() }));
const read = vi.mocked(getMCPProfileOwner);
const owner = (workspace_id = "w", id = "p"): MCPProfileOwner => ({
  id,
  workspace_id,
  agent_id: "a",
  agent_name: "mock",
  name: "Workspace profile",
  kind: "concrete",
  enabled: false,
  supports_mcp: true,
});
beforeEach(() => vi.resetAllMocks());
afterEach(cleanup);
// @covers AC-AGENTS-MCP-MANAGEMENT-001.2, AC-AGENTS-MCP-MANAGEMENT-002.1
it("admits authorized workspace metadata absent from the global inventory, including disabled owners", async () => {
  read.mockResolvedValue(owner());
  const { result } = renderHook(() => useMCPProfileOwner("w", "a", "p"));
  await waitFor(() => expect(result.current.owner).toEqual(owner()));
  expect(read).toHaveBeenCalledWith("w", "p");
});
it.each([owner("other"), { ...owner(), agent_id: "other" }, owner("w", "other")])(
  "rejects response context mismatch without a fallback",
  async (value) => {
    read.mockResolvedValue(value);
    const { result } = renderHook(() => useMCPProfileOwner("w", "a", "p"));
    await waitFor(() => expect(result.current.error).toBeTruthy());
    expect(result.current.owner).toBeNull();
    expect(result.current.canRetry).toBe(false);
  },
);
it("masks the old context immediately and rejects late completions or denied reads", async () => {
  const pending = Promise.withResolvers<MCPProfileOwner>();
  read.mockReturnValueOnce(pending.promise).mockResolvedValueOnce(owner("b"));
  const { result, rerender } = renderHook(({ w }) => useMCPProfileOwner(w, "a", "p"), {
    initialProps: { w: "w" },
  });
  rerender({ w: "b" });
  expect(result.current.owner).toBeNull();
  await waitFor(() => expect(result.current.owner?.workspace_id).toBe("b"));
  await act(async () => pending.resolve(owner()));
  expect(result.current.owner?.workspace_id).toBe("b");
  read.mockRejectedValue(new Error("denied"));
  act(() => result.current.reload());
  await waitFor(() => expect(result.current.error).toBeTruthy());
  expect(result.current.owner).toBeNull();
});
it("never reads without explicit valid context", () => {
  const { result } = renderHook(() => useMCPProfileOwner(null, "a", "p"));
  expect(read).not.toHaveBeenCalled();
  expect(result.current.owner).toBeNull();
});

it("offers retry only for a recoverable owner read failure", async () => {
  read.mockRejectedValueOnce(new Error("offline"));
  const { result } = renderHook(() => useMCPProfileOwner("w", "a", "p"));
  await waitFor(() => expect(result.current.canRetry).toBe(true));
});
