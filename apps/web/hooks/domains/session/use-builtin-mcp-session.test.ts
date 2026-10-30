import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import type { BuiltinMCPSessionView } from "@/lib/types/builtin-mcp";
const fixture = vi.hoisted(() => ({
  read: vi.fn(),
  base: "http://first.test",
  state: {
    auth: { mode: "local", authenticated: true, user: { id: "human" } },
    sessionMcpStatus: { bySessionId: {} },
  },
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (state: typeof fixture.state) => unknown) => select(fixture.state),
}));
vi.mock("@/lib/api/domains/builtin-mcp-api", () => ({ getBuiltinMCPSession: fixture.read }));
vi.mock("@/lib/config", () => ({ getBackendConfig: () => ({ apiBaseUrl: fixture.base }) }));
import { useBuiltinMCPSession } from "./use-builtin-mcp-session";
const view = (session = "session"): BuiltinMCPSessionView => ({
  workspace_id: "ws",
  task_id: "task",
  session_id: session,
  binding_state: "unknown",
  walkthrough_availability: "unknown",
});
beforeEach(() => {
  vi.resetAllMocks();
  fixture.base = "http://first.test";
  fixture.read.mockResolvedValue(view());
});
it("does not load a closed explorer and refreshes when reopened", async () => {
  const { result, rerender } = renderHook(
    ({ open }) => useBuiltinMCPSession("task", "session", "ws", open),
    { initialProps: { open: false } },
  );
  expect(fixture.read).not.toHaveBeenCalled();
  rerender({ open: true });
  await waitFor(() => expect(result.current.view).toEqual(view()));
  rerender({ open: false });
  expect(result.current.view).toBeUndefined();
  rerender({ open: true });
  await waitFor(() => expect(fixture.read).toHaveBeenCalledTimes(2));
});
it("does not accept a late response after the backend changes", async () => {
  let resolve!: (value: BuiltinMCPSessionView) => void;
  fixture.read.mockReturnValueOnce(
    new Promise((done) => {
      resolve = done;
    }),
  );
  const { result, rerender } = renderHook(() =>
    useBuiltinMCPSession("task", "session", "ws", true),
  );
  fixture.base = "http://second.test";
  fixture.read.mockResolvedValue({ ...view(), execution_id: "new" });
  rerender();
  await act(async () => resolve({ ...view(), execution_id: "old" }));
  await waitFor(() => expect(result.current.view?.execution_id).toBe("new"));
});
it("does not display another task's response", async () => {
  fixture.read.mockResolvedValue({ ...view(), task_id: "foreign" });
  const { result } = renderHook(() => useBuiltinMCPSession("task", "session", "ws", true));
  await act(async () => {});
  expect(result.current.view).toBeUndefined();
});
