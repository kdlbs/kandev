import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const reuseOrCreateQuickTerminal = vi.fn();
const hydrate = vi.fn();
const listQuickTerminalTabs = vi.fn();
let state = {
  reuseOrCreateQuickTerminal,
  hydrate,
  quickChat: { terminalTabs: [] as Array<{ workspaceId: string }> },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: typeof state) => unknown) => selector(state),
}));

vi.mock("@/lib/api/domains/quick-terminal-api", () => ({
  listQuickTerminalTabs: (...args: unknown[]) => listQuickTerminalTabs(...args),
  toQuickTerminalTab: (tab: unknown) => tab,
}));

import { useQuickTerminalLauncher } from "./use-quick-terminal-launcher";

const WORKSPACE_ID = "workspace-1";

describe("useQuickTerminalLauncher", () => {
  beforeEach(() => {
    reuseOrCreateQuickTerminal.mockReset();
    hydrate.mockReset();
    listQuickTerminalTabs.mockReset();
    listQuickTerminalTabs.mockResolvedValue({ tabs: [] });
    state = {
      reuseOrCreateQuickTerminal,
      hydrate,
      quickChat: { terminalTabs: [] },
    };
  });

  it("reuses or creates the terminal for the requested workspace", async () => {
    const { result } = renderHook(() => useQuickTerminalLauncher(WORKSPACE_ID));

    await act(async () => {
      await result.current();
    });

    expect(listQuickTerminalTabs).toHaveBeenCalledWith(WORKSPACE_ID);
    expect(reuseOrCreateQuickTerminal).toHaveBeenCalledWith(WORKSPACE_ID);
  });

  it("hydrates shared terminal tabs before reuse when the local store is empty", async () => {
    listQuickTerminalTabs.mockResolvedValue({
      tabs: [
        {
          tabId: "tab-1",
          workspaceId: WORKSPACE_ID,
          sessionId: "session-1",
          sequence: 1,
          status: "running",
        },
      ],
    });
    const { result } = renderHook(() => useQuickTerminalLauncher(WORKSPACE_ID));

    await act(async () => {
      await result.current();
    });

    expect(hydrate).toHaveBeenCalledWith({
      quickChat: {
        terminalTabs: [
          {
            tabId: "tab-1",
            workspaceId: WORKSPACE_ID,
            sessionId: "session-1",
            sequence: 1,
            status: "running",
          },
        ],
      },
    });
    expect(reuseOrCreateQuickTerminal).toHaveBeenCalledWith(WORKSPACE_ID);
  });

  it("does not relist server terminals when the workspace already has one locally", async () => {
    state = {
      reuseOrCreateQuickTerminal,
      hydrate,
      quickChat: { terminalTabs: [{ workspaceId: WORKSPACE_ID }] },
    };
    const { result } = renderHook(() => useQuickTerminalLauncher(WORKSPACE_ID));

    await act(async () => {
      await result.current();
    });

    expect(listQuickTerminalTabs).not.toHaveBeenCalled();
    expect(hydrate).not.toHaveBeenCalled();
    expect(reuseOrCreateQuickTerminal).toHaveBeenCalledWith(WORKSPACE_ID);
  });

  it("does not launch without a workspace", async () => {
    const { result } = renderHook(() => useQuickTerminalLauncher(null));

    await act(async () => {
      await result.current();
    });

    expect(listQuickTerminalTabs).not.toHaveBeenCalled();
    expect(reuseOrCreateQuickTerminal).not.toHaveBeenCalled();
  });
});
