import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useRemoveTerminal, useTerminals } from "./use-terminals";
import type { SetStateAction } from "react";
import type { Terminal } from "./use-terminals";

const mockDestroyUserShell = vi.fn();
const mockRemoveUserShell = vi.fn();
const mockSetRightPanelActiveTab = vi.fn();
const mockToastError = vi.fn();

const storeState = {
  processes: {
    devProcessBySessionId: {},
    outputsByProcessId: {},
  },
  rightPanel: {
    activeTabBySessionId: { "session-1": "shell-1" },
  },
  previewPanel: {
    openBySessionId: {},
  },
  tasks: { activeTaskId: "task-1" },
  setRightPanelActiveTab: mockSetRightPanelActiveTab,
  setPreviewOpen: vi.fn(),
  setPreviewStage: vi.fn(),
  updateUserShell: vi.fn(),
  removeUserShell: mockRemoveUserShell,
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof storeState) => unknown) => selector(storeState),
  useAppStoreApi: () => ({ getState: () => storeState }),
}));

vi.mock("@/lib/local-storage", () => ({
  getSessionStorage: () => null,
}));

vi.mock("@/lib/api", () => ({
  stopProcess: vi.fn(),
}));

vi.mock("@/lib/api/domains/user-shell-api", () => ({
  createUserShell: vi.fn(),
  destroyUserShell: (...args: unknown[]) => mockDestroyUserShell(...args),
  renameUserShell: vi.fn(),
  resumeUserShell: vi.fn(),
}));

vi.mock("./use-user-shells", () => ({
  useUserShells: () => ({ shells: [], isLoaded: false }),
}));

vi.mock("@/lib/toast/sonner", () => ({
  toast: { error: (...args: unknown[]) => mockToastError(...args) },
}));

const initialTerminals = [
  {
    id: "shell-1",
    type: "shell" as const,
    label: "Terminal",
    closable: true,
    kind: "ordinary" as const,
  },
  {
    id: "shell-2",
    type: "shell" as const,
    label: "Terminal",
    closable: true,
    kind: "ordinary" as const,
  },
];

function deferredPromise() {
  let resolve!: () => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<void>((done, fail) => {
    resolve = done;
    reject = fail;
  });
  return { promise, resolve, reject };
}

function renderTerminals() {
  return renderHook(() =>
    useTerminals({
      sessionId: "session-1",
      environmentId: "env-1",
      initialTerminals,
    }),
  );
}

describe("task terminal destroy lifecycle", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    storeState.rightPanel.activeTabBySessionId["session-1"] = "shell-1";
  });

  afterEach(cleanup);

  it("removes immediately while teardown continues and suppresses duplicates", async () => {
    const request = deferredPromise();
    mockDestroyUserShell.mockReturnValue(request.promise);
    const { result } = renderTerminals();

    let first!: Promise<boolean>;
    let duplicate!: Promise<boolean>;
    act(() => {
      first = result.current.destroyTerminal("shell-1");
      duplicate = result.current.destroyTerminal("shell-1");
    });

    expect(mockDestroyUserShell).toHaveBeenCalledTimes(1);
    expect(result.current.terminals.map((terminal) => terminal.id)).toEqual(["shell-2"]);
    expect(mockRemoveUserShell).toHaveBeenCalledWith("env-1", "shell-1");

    await expect(duplicate).resolves.toBe(false);
    request.resolve();
    await act(async () => {
      await expect(first).resolves.toBe(true);
    });

    expect(result.current.terminals.map((terminal) => terminal.id)).toEqual(["shell-2"]);
    expect(mockRemoveUserShell).toHaveBeenCalledWith("env-1", "shell-1");
    expect(mockToastError).not.toHaveBeenCalled();
  });

  it("keeps the terminal dismissed and reports one localized error after failure", async () => {
    const request = deferredPromise();
    mockDestroyUserShell.mockReturnValue(request.promise);
    const { result } = renderTerminals();

    let close!: Promise<boolean>;
    act(() => {
      close = result.current.destroyTerminal("shell-1");
    });
    request.reject(new Error("network down"));

    await act(async () => {
      await expect(close).resolves.toBe(false);
    });

    expect(result.current.terminals.map((terminal) => terminal.id)).toEqual(["shell-2"]);
    expect(mockRemoveUserShell).toHaveBeenCalledWith("env-1", "shell-1");
    expect(mockToastError).toHaveBeenCalledTimes(1);
    expect(mockToastError).toHaveBeenCalledWith("Failed to close terminal");
  });

  it("updates the active fallback outside the terminal state updater", () => {
    let currentTerminals: Terminal[] = initialTerminals;
    let updatingTerminals = false;
    const setTerminals = vi.fn((update: SetStateAction<Terminal[]>) => {
      updatingTerminals = true;
      currentTerminals = typeof update === "function" ? update(currentTerminals) : update;
      updatingTerminals = false;
    });
    const setActiveTab = vi.fn(() => expect(updatingTerminals).toBe(false));
    const { result } = renderHook(() =>
      useRemoveTerminal({
        getActiveTab: () => "shell-1",
        sessionId: "session-1",
        terminals: currentTerminals,
        setTerminals,
        setRightPanelActiveTab: setActiveTab,
      }),
    );

    act(() => result.current("shell-1"));

    expect(currentTerminals.map((terminal) => terminal.id)).toEqual(["shell-2"]);
    expect(setActiveTab).toHaveBeenCalledWith("session-1", "shell-2");
  });
});
