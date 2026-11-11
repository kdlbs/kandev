import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";
import { useMobileTerminals, __resetAutoCreatedEnvironmentsForTest } from "./use-mobile-terminals";
import { userShellScopeKey } from "@/lib/state/user-shell-scope";

const SESSION_ID = "sess-1";

const addTerminalMock = vi.fn();
let mockEnvironmentId: string | null = null;
let mockShells: Array<{
  terminalId: string;
  label: string;
  closable: boolean;
  state?: string;
}> = [];
let mockShellsLoaded = false;

let mockTaskID: string | null = "task-1";
let mockHasSuccessfulList = true;
let mockIsLoading = false;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      environmentIdBySessionId: mockEnvironmentId ? { [SESSION_ID]: mockEnvironmentId } : {},
      tasks: { activeTaskId: mockTaskID },
    }),
  useAppStoreApi: () => ({
    getState: () => {
      const key = userShellScopeKey({
        environmentId: mockEnvironmentId ?? "",
        taskId: mockTaskID,
      });
      return {
        userShells: {
          listed: { [key]: mockHasSuccessfulList },
          loading: { [key]: mockIsLoading },
        },
      };
    },
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
    isLoading: mockIsLoading,
    isLoaded: mockShellsLoaded,
    hasSuccessfulList: mockHasSuccessfulList,
    scope: null,
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
  mockHasSuccessfulList = true;
  mockIsLoading = false;
  mockTaskID = "task-1";
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

  it("does not create while a fresh read is pending over a cached success", async () => {
    mockIsLoading = true;
    renderHook(() => useMobileTerminals(SESSION_ID));
    await new Promise((r) => setTimeout(r, 0));
    expect(addTerminalMock).not.toHaveBeenCalled();
  });

  it("does not create from a settled but unsuccessful read", async () => {
    mockShellsLoaded = true;
    mockHasSuccessfulList = false;
    renderHook(() => useMobileTerminals(SESSION_ID));
    await new Promise((r) => setTimeout(r, 0));
    expect(addTerminalMock).not.toHaveBeenCalled();
  });

  it("creates a shell when the environment cache only has bottom-panel or parked rows", async () => {
    addTerminalMock.mockResolvedValue(undefined);
    mockShells = [
      { terminalId: "bottom-panel", label: "Terminal", closable: true },
      { terminalId: "shell-parked", label: "Parked", closable: true, state: "parked" },
    ];
    renderHook(() => useMobileTerminals(SESSION_ID));
    await waitFor(() => expect(addTerminalMock).toHaveBeenCalledTimes(1));
  });

  it("multiple instances for the same env trigger only one creation (module-level guard)", async () => {
    addTerminalMock.mockResolvedValue(undefined);
    renderHook(() => useMobileTerminals(SESSION_ID));
    renderHook(() => useMobileTerminals(SESSION_ID));
    renderHook(() => useMobileTerminals(SESSION_ID));
    await waitFor(() => expect(addTerminalMock).toHaveBeenCalledTimes(1));
  });

  it("waits for taskID before auto-creating — null taskID is a no-op", async () => {
    addTerminalMock.mockResolvedValue(undefined);
    mockTaskID = null;
    const { rerender } = renderHook(() => useMobileTerminals(SESSION_ID));
    await new Promise((r) => setTimeout(r, 0));
    expect(addTerminalMock).not.toHaveBeenCalled();
    // Once taskID hydrates the next render fires the auto-create.
    mockTaskID = "task-late";
    rerender();
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
