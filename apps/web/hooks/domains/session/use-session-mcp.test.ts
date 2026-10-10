import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  state: {
    settingsAgents: {
      items: [{ supports_mcp: true, profiles: [{ id: "profile-1" }] }],
    },
    sessionMcpStatus: { bySessionId: {} as Record<string, unknown> },
  },
  getAgentProfileMcpConfigAction: vi.fn(),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => store,
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
}));

vi.mock("@/app/actions/agents", () => ({
  getAgentProfileMcpConfigAction: (...args: unknown[]) =>
    mocks.getAgentProfileMcpConfigAction(...args),
}));

import { useSessionMcp } from "./use-session-mcp";

const store = { getState: () => mocks.state };

beforeEach(() => {
  vi.clearAllMocks();
  mocks.getAgentProfileMcpConfigAction.mockResolvedValue({ enabled: true, servers: { docs: {} } });
  mocks.state.sessionMcpStatus.bySessionId = {};
});

afterEach(() => cleanup());

describe("useSessionMcp detail demand", () => {
  it("shares configuration between two visible panels", async () => {
    let resolve!: (value: unknown) => void;
    mocks.getAgentProfileMcpConfigAction.mockReturnValue(
      new Promise((next) => {
        resolve = next;
      }),
    );
    const first = renderHook(() => useSessionMcp("profile-1", "session-1"));
    const second = renderHook(() => useSessionMcp("profile-1", "session-2"));
    expect(mocks.getAgentProfileMcpConfigAction).toHaveBeenCalledTimes(1);
    first.unmount();
    expect(mocks.getAgentProfileMcpConfigAction.mock.calls[0][1].signal.aborted).toBe(false);
    resolve({ enabled: true, servers: { docs: {} } });
    await waitFor(() => expect(second.result.current.mcpServers).toEqual(["kandev", "docs"]));
  });
  it("defers profile configuration reads while hidden and loads once when revealed", async () => {
    const { result, rerender } = renderHook(
      ({ detailActive }: { detailActive: boolean }) =>
        useSessionMcp("profile-1", "session-1", detailActive),
      { initialProps: { detailActive: false } },
    );

    expect(result.current.supportsMcp).toBe(true);
    expect(result.current.mcpServers).toEqual(["kandev"]);
    expect(mocks.getAgentProfileMcpConfigAction).not.toHaveBeenCalled();

    rerender({ detailActive: true });
    await waitFor(() => expect(mocks.getAgentProfileMcpConfigAction).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(result.current.mcpServers).toEqual(["kandev", "docs"]));

    rerender({ detailActive: false });
    expect(result.current.mcpServers).toEqual(["kandev", "docs"]);
    expect(mocks.getAgentProfileMcpConfigAction).toHaveBeenCalledTimes(1);
  });
});
