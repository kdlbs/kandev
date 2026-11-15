import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useTerminals } from "./use-terminals";

const mockCreateUserShell = vi.fn();
const mockAddUserShell = vi.fn();
const mockSetRightPanelActiveTab = vi.fn();

const storeState = {
  processes: {
    devProcessBySessionId: {} as Record<string, string>,
    outputsByProcessId: {} as Record<string, string>,
  },
  rightPanel: { activeTabBySessionId: {} as Record<string, string> },
  previewPanel: { openBySessionId: {} as Record<string, boolean> },
  tasks: {
    activeTaskId: "task-1" as string | null,
    activeSessionId: "session-1" as string | null,
  },
  environmentIdBySessionId: { "session-1": "env-1" } as Record<string, string>,
  workspaces: { activeId: "workspace-1" } as { activeId: string | null },
  workspaceContextGeneration: 0,
  setRightPanelActiveTab: mockSetRightPanelActiveTab,
  setPreviewOpen: vi.fn(),
  setPreviewStage: vi.fn(),
  addUserShell: mockAddUserShell,
  updateUserShell: vi.fn(),
  removeUserShell: vi.fn(),
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof storeState) => unknown) => selector(storeState),
  useAppStoreApi: () => ({ getState: () => storeState }),
}));

vi.mock("@/lib/local-storage", () => ({ getSessionStorage: () => null }));

vi.mock("@/lib/api", () => ({ stopProcess: vi.fn() }));

vi.mock("@/lib/api/domains/user-shell-api", () => ({
  createUserShell: (...args: unknown[]) => mockCreateUserShell(...args),
  destroyUserShell: vi.fn(),
  renameUserShell: vi.fn(),
  resumeUserShell: vi.fn(),
}));

vi.mock("./use-user-shells", () => ({
  useUserShells: () => ({
    shells: [],
    isLoading: false,
    isLoaded: false,
    hasSuccessfulList: false,
    scope: null,
    addShell: vi.fn(),
    removeShell: vi.fn(),
  }),
}));

vi.mock("@/lib/toast/sonner", () => ({ toast: { error: vi.fn() } }));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

const CREATED = {
  terminalId: "shell-new",
  kind: "ordinary" as const,
  seq: 1,
  displayName: "Terminal 1",
  label: "Terminal 1",
  state: "open" as const,
  ptyStatus: "running" as const,
  running: true,
  closable: true,
};

describe("useTerminals late create publication", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    storeState.tasks.activeTaskId = "task-1";
    storeState.tasks.activeSessionId = "session-1";
    storeState.workspaces.activeId = "workspace-1";
    storeState.workspaceContextGeneration = 0;
  });

  afterEach(cleanup);

  it("keeps a late create in its origin cache without publishing into the replacement task", async () => {
    const response = deferred<typeof CREATED>();
    mockCreateUserShell.mockReturnValue(response.promise);
    const { result } = renderHook(() =>
      useTerminals({ sessionId: "session-1", environmentId: "env-1" }),
    );

    act(() => {
      void result.current.addTerminal();
    });
    storeState.tasks.activeTaskId = "task-2";
    response.resolve(CREATED);
    await act(async () => {
      await response.promise;
      await Promise.resolve();
    });

    expect(mockAddUserShell).toHaveBeenCalledWith(
      { environmentId: "env-1", taskId: "task-1" },
      expect.objectContaining({ terminalId: "shell-new" }),
    );
    expect(result.current.terminals).toEqual([]);
    expect(mockSetRightPanelActiveTab).not.toHaveBeenCalled();
  });

  it("does not publish after a workspace context replacement", async () => {
    const response = deferred<typeof CREATED>();
    mockCreateUserShell.mockReturnValue(response.promise);
    const { result } = renderHook(() =>
      useTerminals({ sessionId: "session-1", environmentId: "env-1" }),
    );

    act(() => {
      void result.current.addTerminal();
    });
    storeState.workspaceContextGeneration = 1;
    response.resolve(CREATED);
    await act(async () => {
      await response.promise;
      await Promise.resolve();
    });

    expect(mockAddUserShell).toHaveBeenCalledWith(
      { environmentId: "env-1", taskId: "task-1" },
      expect.objectContaining({ terminalId: "shell-new" }),
    );
    expect(result.current.terminals).toEqual([]);
    expect(mockSetRightPanelActiveTab).not.toHaveBeenCalled();
  });

  it("publishes when the user returns to the origin task before the create settles", async () => {
    const response = deferred<typeof CREATED>();
    mockCreateUserShell.mockReturnValue(response.promise);
    const { result } = renderHook(() =>
      useTerminals({ sessionId: "session-1", environmentId: "env-1" }),
    );

    act(() => {
      void result.current.addTerminal();
    });
    storeState.tasks.activeTaskId = "task-2";
    storeState.tasks.activeTaskId = "task-1";
    response.resolve(CREATED);
    await act(async () => {
      await response.promise;
      await Promise.resolve();
    });

    expect(mockAddUserShell).toHaveBeenCalledWith(
      { environmentId: "env-1", taskId: "task-1" },
      expect.objectContaining({ terminalId: "shell-new" }),
    );
    expect(result.current.terminals.map((terminal) => terminal.id)).toEqual(["shell-new"]);
    expect(mockSetRightPanelActiveTab).toHaveBeenCalledWith("session-1", "shell-new");
  });
});
