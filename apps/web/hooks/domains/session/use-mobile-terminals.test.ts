import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";
import { useMobileTerminals, __resetAutoCreatedEnvironmentsForTest } from "./use-mobile-terminals";

const SESSION_ID = "sess-1";

const addTerminalMock = vi.fn();
let mockEnvironmentId: string | null = null;
let mockShells: Array<{ terminalId: string; label: string; closable: boolean }> = [];
let mockShellsLoaded = false;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      environmentIdBySessionId: mockEnvironmentId ? { [SESSION_ID]: mockEnvironmentId } : {},
    }),
}));

vi.mock("./use-terminals", () => ({
  useTerminals: () => ({
    terminals: [],
    activeTab: undefined,
    terminalTabValue: "commands",
    addTerminal: addTerminalMock,
    removeTerminal: vi.fn(),
    handleCloseDevTab: vi.fn(),
    handleCloseTab: vi.fn(),
    handleRunCommand: vi.fn(),
    isStoppingDev: false,
    devProcessId: undefined,
    devOutput: "",
  }),
}));

vi.mock("./use-user-shells", () => ({
  useUserShells: () => ({
    shells: mockShells,
    isLoading: false,
    isLoaded: mockShellsLoaded,
    addShell: vi.fn(),
    removeShell: vi.fn(),
  }),
}));

beforeEach(() => {
  addTerminalMock.mockReset();
  __resetAutoCreatedEnvironmentsForTest();
  mockEnvironmentId = "env-A";
  mockShells = [];
  mockShellsLoaded = true;
});

describe("useMobileTerminals auto-create", () => {
  it("creates a first shell when shells loaded but list is empty", async () => {
    addTerminalMock.mockResolvedValue(undefined);
    renderHook(() => useMobileTerminals(SESSION_ID));
    await waitFor(() => expect(addTerminalMock).toHaveBeenCalledTimes(1));
  });

  it("does not create a shell when one already exists", async () => {
    mockShells = [{ terminalId: "shell-1", label: "Terminal", closable: true }];
    renderHook(() => useMobileTerminals(SESSION_ID));
    // Wait a tick so any effects had a chance to fire.
    await new Promise((r) => setTimeout(r, 0));
    expect(addTerminalMock).not.toHaveBeenCalled();
  });

  it("does not create when shells are still loading", async () => {
    mockShellsLoaded = false;
    renderHook(() => useMobileTerminals(SESSION_ID));
    await new Promise((r) => setTimeout(r, 0));
    expect(addTerminalMock).not.toHaveBeenCalled();
  });

  it("multiple instances for the same env trigger only one creation (module-level guard)", async () => {
    addTerminalMock.mockResolvedValue(undefined);
    renderHook(() => useMobileTerminals(SESSION_ID));
    renderHook(() => useMobileTerminals(SESSION_ID));
    renderHook(() => useMobileTerminals(SESSION_ID));
    await waitFor(() => expect(addTerminalMock).toHaveBeenCalledTimes(1));
  });

  it("clears the guard when addTerminal rejects so a fresh hook retries", async () => {
    addTerminalMock.mockRejectedValueOnce(new Error("network down"));
    const first = renderHook(() => useMobileTerminals(SESSION_ID));
    await waitFor(() => expect(addTerminalMock).toHaveBeenCalledTimes(1));
    // Let the rejection settle so the .catch handler clears the guard.
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    first.unmount();
    // A new instance for the same env should now retry creation.
    addTerminalMock.mockResolvedValueOnce(undefined);
    renderHook(() => useMobileTerminals(SESSION_ID));
    await waitFor(() => expect(addTerminalMock).toHaveBeenCalledTimes(2));
  });
});
