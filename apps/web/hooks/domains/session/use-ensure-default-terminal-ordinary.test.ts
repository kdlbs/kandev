import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { appState, createUserShell, dockviewApi } = vi.hoisted(() => ({
  appState: {
    tasks: { activeSessionId: "session-one", activeTaskId: "task-one" },
    environmentIdBySessionId: { "session-one": "env-one" },
    userShells: {
      loaded: { "env-one": true },
      byEnvironmentId: { "env-one": [] },
    },
    addUserShell: vi.fn(),
  },
  createUserShell: vi.fn(),
  dockviewApi: { getPanel: vi.fn() },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof appState) => unknown) => selector(appState),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: { api: typeof dockviewApi }) => unknown) =>
    selector({ api: dockviewApi }),
}));

vi.mock("@/lib/api/domains/user-shell-api", () => ({ createUserShell }));

import { useEnsureDefaultTerminalOrdinary } from "./use-ensure-default-terminal-ordinary";

describe("useEnsureDefaultTerminalOrdinary", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    dockviewApi.getPanel.mockReturnValue(undefined);
  });

  it("does not create a shell when the effective layout omits terminal-default", () => {
    renderHook(() => useEnsureDefaultTerminalOrdinary());

    expect(dockviewApi.getPanel).toHaveBeenCalledWith("terminal-default");
    expect(createUserShell).not.toHaveBeenCalled();
    expect(appState.addUserShell).not.toHaveBeenCalled();
  });

  it("still migrates the default terminal panel to an ordinary shell", async () => {
    const updateParameters = vi.fn();
    dockviewApi.getPanel.mockReturnValue({
      params: { terminalId: "shell-default" },
      api: { updateParameters },
    });
    createUserShell.mockResolvedValue({
      terminalId: "terminal-one",
      kind: "ordinary",
      seq: 1,
      displayName: "Terminal 1",
      state: "open",
      ptyStatus: "running",
    });

    renderHook(() => useEnsureDefaultTerminalOrdinary());

    await waitFor(() => expect(createUserShell).toHaveBeenCalledTimes(1));
    expect(createUserShell).toHaveBeenCalledWith("env-one", { taskId: "task-one" });
    expect(updateParameters).toHaveBeenCalledWith({
      terminalId: "terminal-one",
      environmentId: "env-one",
      taskID: "task-one",
    });
  });
});
