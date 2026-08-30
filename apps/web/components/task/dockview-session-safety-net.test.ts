import { afterEach, describe, expect, it, vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { hideSessionPanel, setupChatPanelSafetyNet } from "./dockview-session-tabs";

const TASK_ID = "task-A";
const ACTIVE_SESSION_ID = "session-active";

afterEach(() => {
  vi.unstubAllGlobals();
  useDockviewStore.setState({ api: null, currentLayoutEnvId: null, isRestoringLayout: false });
});

describe("setupChatPanelSafetyNet", () => {
  it("does not recreate the sole session panel after an explicit hide", () => {
    const appStore = {
      getState: () => ({
        tasks: { activeTaskId: TASK_ID, activeSessionId: ACTIVE_SESSION_ID },
        taskSessionsByTask: { itemsByTaskId: { [TASK_ID]: [{ id: ACTIVE_SESSION_ID }] } },
      }),
    };
    let onRemove: ((panel: { id: string }) => void) | undefined;
    const addPanel = vi.fn();
    const api = {
      panels: [],
      addPanel,
      getPanel: () => null,
      removePanel: vi.fn(),
      onDidRemovePanel: (handler: (panel: { id: string }) => void) => {
        onRemove = handler;
        return { dispose: vi.fn() };
      },
    } as unknown as DockviewApi;

    useDockviewStore.setState({
      api,
      currentLayoutEnvId: "env-hide",
      preMaximizeLayout: null,
      isRestoringLayout: false,
    });
    hideSessionPanel(api, ACTIVE_SESSION_ID);
    setupChatPanelSafetyNet(api as never, appStore as never);
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(0);
      return 0;
    });

    expect(onRemove).toBeDefined();
    onRemove?.({ id: `session:${ACTIVE_SESSION_ID}` });

    expect(addPanel).not.toHaveBeenCalled();
  });
});
