import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { DockviewReadyEvent } from "dockview-react";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { useDockviewStore } from "@/lib/state/dockview-store";
import {
  clearSessionTabUserActivationIntentsForTest,
  markSessionTabUserActivationIntent,
} from "./session-tab-activation-intent";
import { setupSessionTabSync } from "./dockview-session-tab-sync";

const TASK_ID = "task-A";
const ACTIVE_SESSION_ID = "s-active";
const OTHER_SESSION_ID = "s-other";
const OTHER_SESSION_PANEL_ID = `session:${OTHER_SESSION_ID}`;

type SessionTabSyncPanel = {
  id: string;
  api: { setActive: ReturnType<typeof vi.fn<() => void>> };
};

type SessionTabSyncApi = {
  panels: SessionTabSyncPanel[];
  groups: Array<{ activePanel: { id: string } | null }>;
  getPanel: (id: string) => SessionTabSyncPanel | null;
  onDidActivePanelChange: (callback: (panel: { id: string } | null) => void) => {
    dispose: ReturnType<typeof vi.fn<() => void>>;
  };
};

type SessionTabSyncStore = {
  getState: () => {
    tasks: { activeTaskId: string; activeSessionId: string };
    taskSessions: {
      items: Record<string, { id: string; task_id: string }>;
    };
    taskSessionsByTask: {
      itemsByTaskId: Record<string, Array<{ id: string }>>;
    };
    environmentIdBySessionId: Record<string, string>;
    setActiveSession: ReturnType<typeof vi.fn<(taskId: string, sessionId: string) => void>>;
    setActiveSessionAuto: ReturnType<typeof vi.fn<(taskId: string, sessionId: string) => void>>;
  };
};

type SessionTabSyncHarness = ReturnType<typeof makeSessionTabSyncHarness>;
type SessionTabSyncDisposable = { dispose: () => void };

const activeDisposables: SessionTabSyncDisposable[] = [];

function makeSessionTabSyncHarness(args: {
  activeTaskId: string;
  activeSessionId: string;
  otherSessionId: string;
  includeOtherEnv?: boolean;
  includeOtherInTaskList?: boolean;
  otherEnvironmentId?: string;
  otherSessionTaskId?: string;
  restoredSessionId?: string;
  restoredSessionIds?: string[];
  restoredPanelIds?: string[];
}) {
  let activePanelChange: ((panel: { id: string } | null) => void) | null = null;
  const activePanelSetActive = vi.fn(() => {
    activePanelChange?.({ id: `session:${args.activeSessionId}` });
  });
  const otherPanelSetActive = vi.fn();
  const panels: SessionTabSyncPanel[] = [
    { id: `session:${args.activeSessionId}`, api: { setActive: activePanelSetActive } },
    { id: `session:${args.otherSessionId}`, api: { setActive: otherPanelSetActive } },
  ];
  const api: SessionTabSyncApi = {
    panels,
    groups: (
      args.restoredPanelIds ??
      (args.restoredSessionIds ?? [args.restoredSessionId ?? args.activeSessionId]).map(
        (sessionId) => `session:${sessionId}`,
      )
    ).map((panelId) => ({ activePanel: { id: panelId } })),
    getPanel: (id: string) => panels.find((panel) => panel.id === id) ?? null,
    onDidActivePanelChange: (callback: (panel: { id: string } | null) => void) => {
      activePanelChange = callback;
      return { dispose: vi.fn() };
    },
  };
  const setActiveSession = vi.fn();
  const setActiveSessionAuto = vi.fn();
  const environmentIdBySessionId = {
    [args.activeSessionId]: "env-A",
    ...(args.includeOtherEnv === false
      ? {}
      : { [args.otherSessionId]: args.otherEnvironmentId ?? "env-A" }),
  };
  const appStore: SessionTabSyncStore = {
    getState: () => ({
      tasks: {
        activeTaskId: args.activeTaskId,
        activeSessionId: args.activeSessionId,
      },
      taskSessions: {
        items: {
          [args.activeSessionId]: { id: args.activeSessionId, task_id: args.activeTaskId },
          [args.otherSessionId]: {
            id: args.otherSessionId,
            task_id: args.otherSessionTaskId ?? args.activeTaskId,
          },
        },
      },
      taskSessionsByTask: {
        itemsByTaskId: {
          [args.activeTaskId]: [
            { id: args.activeSessionId },
            ...(args.includeOtherInTaskList === false ? [] : [{ id: args.otherSessionId }]),
          ],
        },
      },
      environmentIdBySessionId,
      setActiveSession,
      setActiveSessionAuto,
    }),
  };

  return {
    api,
    appStore,
    setActiveSession,
    setActiveSessionAuto,
    activePanelSetActive,
    otherPanelSetActive,
    fireActivePanelChange: (panelId: string | null) => {
      activePanelChange?.(panelId ? { id: panelId } : null);
    },
  };
}

function makeDefaultSessionTabSyncHarness(args?: {
  includeOtherEnv?: boolean;
  includeOtherInTaskList?: boolean;
  otherEnvironmentId?: string;
  otherSessionTaskId?: string;
  restoredSessionId?: string;
  restoredSessionIds?: string[];
  restoredPanelIds?: string[];
}) {
  return makeSessionTabSyncHarness({
    activeTaskId: TASK_ID,
    activeSessionId: ACTIVE_SESSION_ID,
    otherSessionId: OTHER_SESSION_ID,
    includeOtherEnv: args?.includeOtherEnv,
    includeOtherInTaskList: args?.includeOtherInTaskList,
    otherEnvironmentId: args?.otherEnvironmentId,
    otherSessionTaskId: args?.otherSessionTaskId,
    restoredSessionId: args?.restoredSessionId,
    restoredSessionIds: args?.restoredSessionIds,
    restoredPanelIds: args?.restoredPanelIds,
  });
}

function startSessionTabSync(harness: SessionTabSyncHarness) {
  activeDisposables.push(
    setupSessionTabSync(
      harness.api as unknown as DockviewReadyEvent["api"],
      harness.appStore as unknown as StoreApi<AppState>,
    ),
  );
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-01-01T00:00:00Z"));
  clearSessionTabUserActivationIntentsForTest();
  useDockviewStore.setState({ isRestoringLayout: false, currentLayoutEnvId: "env-A" });
});

afterEach(() => {
  for (const disposable of activeDisposables.splice(0)) disposable.dispose();
  clearSessionTabUserActivationIntentsForTest();
  useDockviewStore.setState({ isRestoringLayout: false });
  vi.useRealTimers();
});

describe("setupSessionTabSync automatic activation", () => {
  it("does not pin a session when Dockview activates another session panel without user intent", () => {
    const harness = makeDefaultSessionTabSyncHarness();

    startSessionTabSync(harness);
    harness.fireActivePanelChange(OTHER_SESSION_PANEL_ID);

    expect(harness.setActiveSession).not.toHaveBeenCalled();
    expect(harness.activePanelSetActive).toHaveBeenCalledTimes(1);
  });
});

describe("setupSessionTabSync restored selection", () => {
  // @covers AC-UI-TASK-AGENT-TAB-RECONCILIATION-001.6
  it("adopts the restored secondary Agent tab without creating a user pin", () => {
    const harness = makeDefaultSessionTabSyncHarness({ restoredSessionId: OTHER_SESSION_ID });

    startSessionTabSync(harness);

    expect(harness.setActiveSessionAuto).toHaveBeenCalledWith(TASK_ID, OTHER_SESSION_ID);
    expect(harness.setActiveSession).not.toHaveBeenCalled();
  });

  it("uses the Agent-group selection when another group has global focus", () => {
    const harness = makeDefaultSessionTabSyncHarness({
      restoredPanelIds: ["files", OTHER_SESSION_PANEL_ID],
    });

    startSessionTabSync(harness);

    expect(harness.setActiveSessionAuto).toHaveBeenCalledWith(TASK_ID, OTHER_SESSION_ID);
  });

  it("keeps the boot fallback when the restored session has no environment", () => {
    const harness = makeDefaultSessionTabSyncHarness({
      includeOtherEnv: false,
      restoredSessionId: OTHER_SESSION_ID,
    });

    startSessionTabSync(harness);

    expect(harness.setActiveSessionAuto).not.toHaveBeenCalled();
  });

  it("keeps the boot fallback for a restored session from another task", () => {
    const harness = makeDefaultSessionTabSyncHarness({
      otherSessionTaskId: "task-B",
      restoredSessionId: OTHER_SESSION_ID,
    });

    startSessionTabSync(harness);

    expect(harness.setActiveSessionAuto).not.toHaveBeenCalled();
  });

  it("keeps the boot fallback when the restored session is not current", () => {
    const harness = makeDefaultSessionTabSyncHarness({
      includeOtherInTaskList: false,
      restoredSessionId: OTHER_SESSION_ID,
    });

    startSessionTabSync(harness);

    expect(harness.setActiveSessionAuto).not.toHaveBeenCalled();
  });

  it("keeps the boot fallback for a restored session from another environment", () => {
    const harness = makeDefaultSessionTabSyncHarness({
      otherEnvironmentId: "env-B",
      restoredSessionId: OTHER_SESSION_ID,
    });

    startSessionTabSync(harness);

    expect(harness.setActiveSessionAuto).not.toHaveBeenCalled();
  });

  it("keeps the boot fallback when multiple Agent groups have a selected session", () => {
    const harness = makeDefaultSessionTabSyncHarness({
      restoredSessionIds: [ACTIVE_SESSION_ID, OTHER_SESSION_ID],
    });

    startSessionTabSync(harness);

    expect(harness.setActiveSessionAuto).not.toHaveBeenCalled();
  });

  it("adopts the only valid restored session when another selected panel is stale", () => {
    const harness = makeDefaultSessionTabSyncHarness({
      restoredSessionIds: ["missing-session", OTHER_SESSION_ID],
    });

    startSessionTabSync(harness);

    expect(harness.setActiveSessionAuto).toHaveBeenCalledWith(TASK_ID, OTHER_SESSION_ID);
  });

  it("adopts the restored selection after late environment layout restoration", () => {
    const harness = makeDefaultSessionTabSyncHarness();
    useDockviewStore.setState({ currentLayoutEnvId: null });
    startSessionTabSync(harness);

    harness.api.groups = [{ activePanel: { id: OTHER_SESSION_PANEL_ID } }];
    useDockviewStore.setState({ isRestoringLayout: true, currentLayoutEnvId: "env-A" });
    useDockviewStore.setState({ isRestoringLayout: false });

    expect(harness.setActiveSessionAuto).toHaveBeenCalledWith(TASK_ID, OTHER_SESSION_ID);
  });
});

describe("setupSessionTabSync explicit activation", () => {
  it("pins the session when the active panel change follows explicit session-tab user intent", () => {
    const harness = makeDefaultSessionTabSyncHarness();

    startSessionTabSync(harness);
    markSessionTabUserActivationIntent(OTHER_SESSION_ID);
    harness.fireActivePanelChange(OTHER_SESSION_PANEL_ID);

    expect(harness.setActiveSession).toHaveBeenCalledWith(TASK_ID, OTHER_SESSION_ID);
    expect(harness.activePanelSetActive).not.toHaveBeenCalled();
  });

  it("ignores active panel changes while Dockview is restoring layout", () => {
    const harness = makeDefaultSessionTabSyncHarness();

    useDockviewStore.setState({ isRestoringLayout: true });
    startSessionTabSync(harness);
    markSessionTabUserActivationIntent(OTHER_SESSION_ID);
    harness.fireActivePanelChange(OTHER_SESSION_PANEL_ID);

    expect(harness.setActiveSession).not.toHaveBeenCalled();
    expect(harness.activePanelSetActive).not.toHaveBeenCalled();
  });

  it("restores the active panel when intent exists for a different session", () => {
    const harness = makeDefaultSessionTabSyncHarness();

    startSessionTabSync(harness);
    markSessionTabUserActivationIntent(ACTIVE_SESSION_ID);
    harness.fireActivePanelChange(OTHER_SESSION_PANEL_ID);

    expect(harness.setActiveSession).not.toHaveBeenCalled();
    expect(harness.activePanelSetActive).toHaveBeenCalledTimes(1);
  });

  it("ignores null and non-session panel changes", () => {
    const harness = makeDefaultSessionTabSyncHarness();

    startSessionTabSync(harness);
    harness.fireActivePanelChange(null);
    harness.fireActivePanelChange("files");

    expect(harness.setActiveSession).not.toHaveBeenCalled();
    expect(harness.activePanelSetActive).not.toHaveBeenCalled();
    expect(harness.otherPanelSetActive).not.toHaveBeenCalled();
  });

  it("restores active panel for stale session panels without an environment mapping", () => {
    const harness = makeDefaultSessionTabSyncHarness({ includeOtherEnv: false });

    startSessionTabSync(harness);
    markSessionTabUserActivationIntent(OTHER_SESSION_ID);
    harness.fireActivePanelChange(OTHER_SESSION_PANEL_ID);

    expect(harness.setActiveSession).not.toHaveBeenCalled();
    expect(harness.activePanelSetActive).toHaveBeenCalledTimes(1);
  });
});
