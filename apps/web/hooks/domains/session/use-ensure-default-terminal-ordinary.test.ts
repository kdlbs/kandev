import { waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import { userShellScopeKey } from "@/lib/state/user-shell-scope";
import type { AppState } from "@/lib/state/store";
import type { UserShellInfo } from "@/lib/state/slices/session-runtime/types";
import { deferred, renderSessionRead } from "./session-read-test-helpers";

const { createUserShell, dockviewApi, dockviewState } = vi.hoisted(() => ({
  createUserShell: vi.fn(),
  dockviewState: { isRestoringLayout: false },
  dockviewApi: {
    getPanel: vi.fn(),
    panels: [] as Array<{
      id: string;
      params?: Record<string, unknown>;
      api: { id: string; close: () => void };
    }>,
  },
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: Object.assign(
    (selector: (state: { api: typeof dockviewApi; isRestoringLayout: boolean }) => unknown) =>
      selector({ api: dockviewApi, isRestoringLayout: dockviewState.isRestoringLayout }),
    {
      getState: () => ({
        api: dockviewApi,
        isRestoringLayout: dockviewState.isRestoringLayout,
      }),
    },
  ),
}));

vi.mock("@/lib/api/domains/user-shell-api", () => ({ createUserShell }));

import { useEnsureDefaultTerminalOrdinary } from "./use-ensure-default-terminal-ordinary";

const TASK_ID = "task-one";
const ENV_ID = "environment";
const PANEL_ID = "terminal-default";
const SHELL_DEFAULT = "shell-default";
const SHELL_CREATED = "shell-created";
const SHELL_OPEN = "shell-open";
const SHELL_PARKED = "shell-parked";
const SHELL_FOREIGN = "shell-foreign";
const SCOPE_KEY = userShellScopeKey({ environmentId: ENV_ID, taskId: TASK_ID });

type SeedOptions = {
  shells?: UserShellInfo[];
  listed?: boolean;
  loading?: boolean;
  panelTerminalId?: string;
  panelTaskId?: string;
  panelEnvironmentId?: string;
  omitPanelStamps?: boolean;
  activeTaskId?: string | null;
};

function ordinary(id: string, overrides: Partial<UserShellInfo> = {}): UserShellInfo {
  return {
    terminalId: id,
    kind: "ordinary",
    seq: 1,
    state: "open",
    ptyStatus: "running",
    customName: null,
    displayName: "Terminal 1",
    ...overrides,
  };
}

type PanelMock = {
  params: Record<string, unknown>;
  api: { updateParameters: ReturnType<typeof vi.fn> };
};

function seed(store: StoreApi<AppState>, options: SeedOptions): PanelMock | null {
  const activeTaskId = "activeTaskId" in options ? (options.activeTaskId ?? null) : TASK_ID;
  const key = userShellScopeKey({ environmentId: ENV_ID, taskId: activeTaskId ?? null });
  store.setState((state) => ({
    tasks: {
      ...state.tasks,
      activeSessionId: "session",
      activeTaskId,
    },
    userShells: {
      ...state.userShells,
      byScope: { [key]: options.shells ?? [] },
      listed: { [key]: options.listed ?? true },
      loaded: { [key]: true },
      loading: { [key]: options.loading ?? false },
    },
  }));
  if (!options.panelTerminalId) {
    dockviewApi.getPanel.mockReturnValue(undefined);
    return null;
  }
  const panel: PanelMock = {
    params: options.omitPanelStamps
      ? { terminalId: options.panelTerminalId }
      : {
          terminalId: options.panelTerminalId,
          environmentId: options.panelEnvironmentId ?? ENV_ID,
          taskID: options.panelTaskId ?? TASK_ID,
        },
    api: { updateParameters: vi.fn() },
  };
  dockviewApi.getPanel.mockReturnValue({ id: PANEL_ID, ...panel });
  return panel;
}

function renderWithSeed(options: SeedOptions) {
  let panel: PanelMock | null = null;
  const rendered = renderSessionRead(
    () => useEnsureDefaultTerminalOrdinary(),
    undefined,
    (store) => {
      panel = seed(store, options);
    },
  );
  return { ...rendered, panel: panel as PanelMock | null };
}

const CREATED = {
  terminalId: SHELL_CREATED,
  kind: "ordinary" as const,
  seq: 1,
  displayName: "Terminal 1",
  state: "open" as const,
  ptyStatus: "running" as const,
};

beforeEach(() => {
  vi.clearAllMocks();
  dockviewApi.panels = [];
  createUserShell.mockResolvedValue(CREATED);
});

describe("useEnsureDefaultTerminalOrdinary selection", () => {
  it("does not create a shell when the effective layout omits the panel", () => {
    renderWithSeed({});

    expect(dockviewApi.getPanel).toHaveBeenCalledWith(PANEL_ID);
    expect(createUserShell).not.toHaveBeenCalled();
  });

  it("migrates shell-default to a newly created ordinary shell", async () => {
    const { panel } = renderWithSeed({ panelTerminalId: SHELL_DEFAULT });

    await waitFor(() => expect(createUserShell).toHaveBeenCalledTimes(1));
    expect(createUserShell).toHaveBeenCalledWith(ENV_ID, { taskId: TASK_ID });
    await waitFor(() =>
      expect(panel?.api.updateParameters).toHaveBeenCalledWith({
        terminalId: SHELL_CREATED,
        environmentId: ENV_ID,
        taskID: TASK_ID,
      }),
    );
  });

  it("repairs a restored default panel pointing at another task", () => {
    const { panel } = renderWithSeed({
      panelTerminalId: SHELL_FOREIGN,
      shells: [ordinary(SHELL_OPEN, { seq: 3 }), ordinary("shell-other", { seq: 2 })],
    });

    expect(panel?.api.updateParameters).toHaveBeenCalledWith({
      terminalId: "shell-other",
      environmentId: ENV_ID,
      taskID: TASK_ID,
    });
    expect(createUserShell).not.toHaveBeenCalled();
    expect(panel?.api.updateParameters).not.toHaveBeenCalledWith(
      expect.objectContaining({ terminalId: SHELL_FOREIGN }),
    );
  });

  it("restores missing stamps on an eligible terminal id", () => {
    const { panel } = renderWithSeed({
      panelTerminalId: SHELL_OPEN,
      omitPanelStamps: true,
      shells: [ordinary(SHELL_OPEN)],
    });

    expect(panel?.api.updateParameters).toHaveBeenCalledWith({
      terminalId: SHELL_OPEN,
      environmentId: ENV_ID,
      taskID: TASK_ID,
    });
  });

  it("repairs conflicting stamps even when the terminal id is eligible", () => {
    const { panel } = renderWithSeed({
      panelTerminalId: SHELL_OPEN,
      panelTaskId: "task-two",
      panelEnvironmentId: "env-other",
      shells: [ordinary(SHELL_OPEN)],
    });

    expect(panel?.api.updateParameters).toHaveBeenCalledWith({
      terminalId: SHELL_OPEN,
      environmentId: ENV_ID,
      taskID: TASK_ID,
    });
  });

  it("replaces a selection that became parked with the lowest open sequence", () => {
    const { panel } = renderWithSeed({
      panelTerminalId: SHELL_PARKED,
      shells: [
        ordinary(SHELL_PARKED, { seq: 1, state: "parked" }),
        ordinary("shell-later", { seq: 4 }),
        ordinary(SHELL_OPEN, { seq: 2 }),
      ],
    });

    expect(panel?.api.updateParameters).toHaveBeenCalledWith({
      terminalId: SHELL_OPEN,
      environmentId: ENV_ID,
      taskID: TASK_ID,
    });
  });
});

describe("useEnsureDefaultTerminalOrdinary creation ownership", () => {
  it("keeps a valid open selection and ignores parked rows", () => {
    const { panel } = renderWithSeed({
      panelTerminalId: SHELL_OPEN,
      shells: [
        ordinary(SHELL_OPEN, { seq: 3 }),
        ordinary(SHELL_PARKED, { seq: 1, state: "parked" }),
      ],
    });

    expect(panel?.api.updateParameters).not.toHaveBeenCalled();
    expect(createUserShell).not.toHaveBeenCalled();
  });

  it("creates nothing while the current scope has no successful list", () => {
    renderWithSeed({ panelTerminalId: SHELL_FOREIGN, listed: false });

    expect(createUserShell).not.toHaveBeenCalled();
  });

  it("creates nothing while a fresh read is pending over a cached success", () => {
    renderWithSeed({ panelTerminalId: SHELL_FOREIGN, listed: true, loading: true });

    expect(createUserShell).not.toHaveBeenCalled();
  });

  it("creates nothing for a taskless successful list", () => {
    renderWithSeed({
      activeTaskId: null,
      panelTerminalId: SHELL_FOREIGN,
      shells: [],
    });

    expect(createUserShell).not.toHaveBeenCalled();
  });

  it("reconciles again when layout restoration completes", () => {
    dockviewState.isRestoringLayout = true;
    const { panel, rerender } = renderWithSeed({
      panelTerminalId: SHELL_FOREIGN,
      shells: [ordinary(SHELL_OPEN)],
    });

    expect(panel?.api.updateParameters).not.toHaveBeenCalled();
    dockviewState.isRestoringLayout = false;
    rerender();
    expect(panel?.api.updateParameters).toHaveBeenCalledWith({
      terminalId: SHELL_OPEN,
      environmentId: ENV_ID,
      taskID: TASK_ID,
    });
  });

  it("creates once for concurrent consumers of the same scope", async () => {
    const pending = deferred<typeof CREATED>();
    createUserShell.mockReturnValue(pending.promise);
    renderSessionRead(
      () => [useEnsureDefaultTerminalOrdinary(), useEnsureDefaultTerminalOrdinary()],
      undefined,
      (store) => {
        seed(store, { panelTerminalId: SHELL_DEFAULT });
      },
    );

    expect(createUserShell).toHaveBeenCalledTimes(1);
    pending.resolve(CREATED);
    await waitFor(() => expect(dockviewApi.getPanel).toHaveBeenCalled());
  });

  it("does not rewrite a reused panel from a late create", async () => {
    const pending = deferred<typeof CREATED>();
    createUserShell.mockReturnValue(pending.promise);
    const { panel, result } = renderWithSeed({ panelTerminalId: SHELL_DEFAULT });

    await waitFor(() => expect(createUserShell).toHaveBeenCalledTimes(1));
    // The panel is reused for another task before creation settles.
    if (panel) {
      panel.params = {
        terminalId: "shell-someone-else",
        environmentId: ENV_ID,
        taskID: "task-two",
      };
    }
    result.current.store.setState((state) => ({
      tasks: { ...state.tasks, activeTaskId: "task-two" },
    }));
    pending.resolve(CREATED);

    await waitFor(() =>
      expect(result.current.store.getState().userShells.byScope[SCOPE_KEY]).toContainEqual(
        expect.objectContaining({ terminalId: SHELL_CREATED }),
      ),
    );
    expect(panel?.api.updateParameters).not.toHaveBeenCalled();
  });
});
