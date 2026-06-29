import { describe, expect, it, vi } from "vitest";
import { markSessionTabUserActivationIntent } from "./session-tab-activation-intent";
import { setupSessionTabSync } from "./dockview-session-tab-sync";

function makeSessionTabSyncHarness(args: {
  activeTaskId: string;
  activeSessionId: string;
  otherSessionId: string;
}) {
  let activePanelChange: ((panel: { id: string } | null) => void) | null = null;
  const activePanelSetActive = vi.fn();
  const otherPanelSetActive = vi.fn();
  const panels = [
    { id: `session:${args.activeSessionId}`, api: { setActive: activePanelSetActive } },
    { id: `session:${args.otherSessionId}`, api: { setActive: otherPanelSetActive } },
  ];
  const api = {
    panels,
    getPanel: (id: string) => panels.find((panel) => panel.id === id) ?? null,
    onDidActivePanelChange: (callback: (panel: { id: string } | null) => void) => {
      activePanelChange = callback;
      return { dispose: vi.fn() };
    },
  };
  const setActiveSession = vi.fn();
  const appStore = {
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
      environmentIdBySessionId: {
        [args.activeSessionId]: "env-A",
        [args.otherSessionId]: "env-A",
      },
      setActiveSession,
    }),
  };

  return {
    api,
    appStore,
    setActiveSession,
    activePanelSetActive,
    fireActivePanelChange: (panelId: string) => {
      activePanelChange?.({ id: panelId });
    },
  };
}

describe("setupSessionTabSync", () => {
  it("does not pin a session when Dockview activates another session panel without user intent", () => {
    const harness = makeSessionTabSyncHarness({
      activeTaskId: "task-A",
      activeSessionId: "s-active",
      otherSessionId: "s-other",
    });

    setupSessionTabSync(harness.api as never, harness.appStore as never);
    harness.fireActivePanelChange("session:s-other");

    expect(harness.setActiveSession).not.toHaveBeenCalled();
    expect(harness.activePanelSetActive).toHaveBeenCalledTimes(1);
  });

  it("pins the session when the active panel change follows explicit session-tab user intent", () => {
    const harness = makeSessionTabSyncHarness({
      activeTaskId: "task-A",
      activeSessionId: "s-active",
      otherSessionId: "s-other",
    });

    setupSessionTabSync(harness.api as never, harness.appStore as never);
    markSessionTabUserActivationIntent("s-other");
    harness.fireActivePanelChange("session:s-other");

    expect(harness.setActiveSession).toHaveBeenCalledWith("task-A", "s-other");
    expect(harness.activePanelSetActive).not.toHaveBeenCalled();
  });
});
