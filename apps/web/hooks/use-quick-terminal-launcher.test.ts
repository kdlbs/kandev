import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const reuseOrCreateQuickTerminal = vi.fn();
let state = { reuseOrCreateQuickTerminal };

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: typeof state) => unknown) => selector(state),
}));

import { useQuickTerminalLauncher } from "./use-quick-terminal-launcher";

describe("useQuickTerminalLauncher", () => {
  beforeEach(() => {
    reuseOrCreateQuickTerminal.mockReset();
    state = { reuseOrCreateQuickTerminal };
  });

  it("reuses or creates the terminal for the requested workspace", () => {
    const { result } = renderHook(() => useQuickTerminalLauncher("workspace-1"));

    act(() => result.current());

    expect(reuseOrCreateQuickTerminal).toHaveBeenCalledWith("workspace-1");
  });

  it("does not launch without a workspace", () => {
    const { result } = renderHook(() => useQuickTerminalLauncher(null));

    act(() => result.current());

    expect(reuseOrCreateQuickTerminal).not.toHaveBeenCalled();
  });
});
