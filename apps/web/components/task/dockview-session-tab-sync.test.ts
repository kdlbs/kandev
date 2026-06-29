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
  api: { setActive: ReturnType<typeof vi.fn<[], void>> };
};

type SessionTabSyncApi = {
  panels: SessionTabSyncPanel[];
  getPanel: (id: string) => SessionTabSyncPanel | null;
  onDidActivePanelChange: (callback: (panel: { id: string } | null) => void) => {
    dispose: ReturnType<typeof vi.fn<[], void>>;
  };
};

type SessionTabSyncStore = {
  getState: () => {
    tasks: { activeTaskId: string; activeSessionId: string };
    taskSessions: {
      items: Record<string, { id: string; task_id: string }>;
    };
    environmentIdBySessionId: Record<string, string>;
    setActiveSession: ReturnType<typeof vi.fn<[string, string], void>>;
  };
};

type SessionTabSyncHarness = ReturnType<typeof makeSessionTabSyncHarness>;

function makeSessionTabSyncHarness(args: {
  activeTaskId: string;
  activeSessionId: string;
  otherSessionId: string;
  includeOtherEnv?: boolean;
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
    getPanel: (id: string) => panels.find((panel) => panel.id === id) ?? null,
    onDidActivePanelChange: (callback: (panel: { id: string } | null) => void) => {
      activePanelChange = callback;
      return { dispose: vi.fn() };
    },
  };
  const setActiveSession = vi.fn();
  const environmentIdBySessionId = {
    [args.activeSessionId]: "env-A",
    ...(args.includeOtherEnv === false ? {} : { [args.otherSessionId]: "env-A" }),
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
          [args.otherSessionId]: { id: args.otherSessionId, task_id: args.activeTaskId },
        },
      },
      environmentIdBySessionId,
      setActiveSession,
    }),
  };

  return {
    api,
    appStore,
    setActiveSession,
    activePanelSetActive,
    otherPanelSetActive,
    fireActivePanelChange: (panelId: string | null) => {
      activePanelChange?.(panelId ? { id: panelId } : null);
    },
  };
}

function makeDefaultSessionTabSyncHarness(args?: { includeOtherEnv?: boolean }) {
  return makeSessionTabSyncHarness({
    activeTaskId: TASK_ID,
    activeSessionId: ACTIVE_SESSION_ID,
    otherSessionId: OTHER_SESSION_ID,
    includeOtherEnv: args?.includeOtherEnv,
  });
}

function startSessionTabSync(harness: SessionTabSyncHarness) {
  setupSessionTabSync(
    harness.api as unknown as DockviewReadyEvent["api"],
    harness.appStore as unknown as StoreApi<AppState>,
  );
}

describe("setupSessionTabSync", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-01-01T00:00:00Z"));
    clearSessionTabUserActivationIntentsForTest();
    useDockviewStore.setState({ isRestoringLayout: false });
  });

  afterEach(() => {
    clearSessionTabUserActivationIntentsForTest();
    useDockviewStore.setState({ isRestoringLayout: false });
    vi.useRealTimers();
  });

  it("does not pin a session when Dockview activates another session panel without user intent", () => {
    const harness = makeDefaultSessionTabSyncHarness();

    startSessionTabSync(harness);
    harness.fireActivePanelChange(OTHER_SESSION_PANEL_ID);

    expect(harness.setActiveSession).not.toHaveBeenCalled();
    expect(harness.activePanelSetActive).toHaveBeenCalledTimes(1);
  });

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
