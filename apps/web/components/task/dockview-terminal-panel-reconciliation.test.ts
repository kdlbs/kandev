import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { userShellScopeKey } from "@/lib/state/user-shell-scope";

const { mockPark, mockStop, mockStopVscode, dockviewState } = vi.hoisted(() => ({
  mockPark: vi.fn(() => Promise.resolve()),
  mockStop: vi.fn(() => Promise.resolve()),
  mockStopVscode: vi.fn(),
  dockviewState: { isRestoringLayout: false, preMaximizeLayout: null },
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: Object.assign(() => ({}), {
    getState: () => ({
      isRestoringLayout: dockviewState.isRestoringLayout,
      preMaximizeLayout: dockviewState.preMaximizeLayout,
      clearScrollTargetForOwner: vi.fn(),
      exitMaximizedLayout: vi.fn(),
    }),
  }),
}));

vi.mock("@/lib/api/domains/user-shell-api", () => ({
  parkUserShell: mockPark,
  stopUserShell: mockStop,
}));

vi.mock("@/lib/api/domains/vscode-api", () => ({ stopVscode: mockStopVscode }));

vi.mock("@/lib/layout/panel-portal-manager", () => ({
  panelPortalManager: { get: vi.fn(() => undefined), release: vi.fn() },
}));

import { removeInvalidOrdinaryTerminalPanels, setupPortalCleanup } from "./dockview-layout-setup";

const SCOPE = { environmentId: "env-1", taskId: "task-1" };
const OWNED_ID = "shell-owned";
const FOREIGN_ID = "shell-foreign";

type FakePanel = {
  id: string;
  params: Record<string, unknown>;
  api: { id: string; close: () => void; updateParameters: () => void };
};

function makeApi(panels: FakePanel[]) {
  const handlers: Array<(panel: { id: string }) => void> = [];
  return {
    panels,
    onDidRemovePanel: (handler: (panel: { id: string }) => void) => {
      handlers.push(handler);
      return { dispose: () => {} };
    },
    fireRemoval(panel: { id: string }) {
      for (const handler of handlers) handler(panel);
    },
    hasMaximizedGroup: () => false,
    getPanel: (id: string) => panels.find((panel) => panel.id === id),
  };
}

function makePanel(id: string, params: Record<string, unknown>): FakePanel {
  return {
    id,
    params,
    api: { id, close: vi.fn(), updateParameters: vi.fn() },
  };
}

function makeAppStore(): StoreApi<AppState> {
  const state = {
    tasks: { activeSessionId: "session-1", activeTaskId: "task-1" },
    environmentIdBySessionId: { "session-1": "env-1" },
    userShells: {
      byScope: {
        [userShellScopeKey(SCOPE)]: [
          { terminalId: OWNED_ID, kind: "ordinary", state: "open" },
        ] as Array<{ terminalId: string; kind?: string; state?: string }>,
      },
      byEnvironmentId: {} as Record<
        string,
        Array<{ terminalId: string; kind?: string; state?: string }>
      >,
    },
  };
  return { getState: () => state as unknown as AppState } as StoreApi<AppState>;
}

beforeEach(() => {
  vi.clearAllMocks();
  dockviewState.isRestoringLayout = false;
});

describe("removeInvalidOrdinaryTerminalPanels", () => {
  it("closes only ordinary panels outside the active task scope", () => {
    const owned = makePanel(OWNED_ID, {
      terminalId: OWNED_ID,
      environmentId: "env-1",
      taskID: "task-1",
    });
    const foreign = makePanel(FOREIGN_ID, {
      terminalId: FOREIGN_ID,
      environmentId: "env-1",
      taskID: "task-1",
    });
    const script = makePanel("script-1", {
      terminalId: "script-1",
      environmentId: "env-1",
    });
    const api = makeApi([owned, foreign, script]);

    removeInvalidOrdinaryTerminalPanels(api as never, makeAppStore(), SCOPE);

    expect(owned.api.close).not.toHaveBeenCalled();
    expect(script.api.close).not.toHaveBeenCalled();
    expect(foreign.api.close).toHaveBeenCalledTimes(1);
  });

  it("removes panels stamped for another task even when their cache would own them", () => {
    const otherTask = makePanel(FOREIGN_ID, {
      terminalId: FOREIGN_ID,
      environmentId: "env-1",
      taskID: "task-2",
    });
    const api = makeApi([otherTask]);

    removeInvalidOrdinaryTerminalPanels(api as never, makeAppStore(), SCOPE);

    expect(otherTask.api.close).toHaveBeenCalledTimes(1);
  });

  it("removes panels stamped for another environment", () => {
    const otherEnvironment = makePanel(FOREIGN_ID, {
      terminalId: FOREIGN_ID,
      environmentId: "env-2",
      taskID: "task-1",
    });
    const api = makeApi([otherEnvironment]);

    removeInvalidOrdinaryTerminalPanels(api as never, makeAppStore(), SCOPE);

    expect(otherEnvironment.api.close).toHaveBeenCalledTimes(1);
  });

  it("does not mutate terminal lifecycle state for a removed foreign panel", () => {
    const foreign = makePanel(FOREIGN_ID, {
      terminalId: FOREIGN_ID,
      environmentId: "env-1",
      taskID: "task-2",
    });
    const api = makeApi([foreign]);
    const store = makeAppStore();
    setupPortalCleanup(api as never, store);

    removeInvalidOrdinaryTerminalPanels(api as never, store, SCOPE);
    api.fireRemoval(foreign);

    expect(foreign.api.close).toHaveBeenCalledTimes(1);
    expect(mockPark).not.toHaveBeenCalled();
    expect(mockStop).not.toHaveBeenCalled();
  });

  it("removes extra panels that reference parked terminals", () => {
    const parked = makePanel(FOREIGN_ID, {
      terminalId: FOREIGN_ID,
      environmentId: "env-1",
      taskID: "task-1",
    });
    const api = makeApi([parked]);
    const store = makeAppStore();
    store.getState().userShells.byScope[userShellScopeKey(SCOPE)] = [
      { terminalId: FOREIGN_ID, kind: "ordinary", state: "parked" },
    ];

    removeInvalidOrdinaryTerminalPanels(api as never, store, SCOPE);

    expect(parked.api.close).toHaveBeenCalledTimes(1);
  });

  it("keeps legacy environment-scoped terminal panels", () => {
    const legacy = makePanel(FOREIGN_ID, {
      terminalId: FOREIGN_ID,
      environmentId: "env-1",
      taskID: "task-2",
    });
    const api = makeApi([legacy]);
    const store = makeAppStore();
    store.getState().userShells.byEnvironmentId["env-1"] = [
      { terminalId: FOREIGN_ID, label: "Legacy" },
    ];

    removeInvalidOrdinaryTerminalPanels(api as never, store, SCOPE);

    expect(legacy.api.close).not.toHaveBeenCalled();
  });

  it("does nothing while a layout is being restored", () => {
    dockviewState.isRestoringLayout = true;
    const foreign = makePanel(FOREIGN_ID, {
      terminalId: FOREIGN_ID,
      environmentId: "env-1",
      taskID: "task-1",
    });
    const api = makeApi([foreign]);

    removeInvalidOrdinaryTerminalPanels(api as never, makeAppStore(), SCOPE);

    expect(foreign.api.close).not.toHaveBeenCalled();
  });
});
