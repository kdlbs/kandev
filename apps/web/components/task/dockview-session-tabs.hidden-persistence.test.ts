import { describe, expect, it } from "vitest";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { hideSessionPanel, runAutoSessionTabEffect } from "./dockview-session-tabs";
import { makeReorderingAutoSessionApi } from "./dockview-session-tabs.test-utils";

const TASK_ID = "task-A";
const ACTIVE_SESSION_ID = "session-active";
const HIDDEN_SESSION_ID = "session-hidden";

function makeAppStore() {
  return {
    getState: () => ({
      tasks: { activeTaskId: TASK_ID },
      taskSessionsByTask: {
        itemsByTaskId: {
          [TASK_ID]: [{ id: ACTIVE_SESSION_ID }, { id: HIDDEN_SESSION_ID }],
        },
      },
    }),
  };
}

function makeRefs() {
  return {
    sessionTabCreatedRef: { current: new Set<string>() },
    hiddenSessionIdsRef: { current: new Set<string>() },
    hiddenSessionEnvIdRef: { current: null as string | null },
    prevTaskIdRef: { current: "task-old" as string | null },
    prevSessionIdRef: { current: "session-old" as string | null },
  };
}

function runInEnvironment(
  api: ReturnType<typeof makeReorderingAutoSessionApi>["api"],
  envId: string,
  run: () => void,
) {
  const previous = useDockviewStore.getState();
  useDockviewStore.setState({ api, currentLayoutEnvId: envId, preMaximizeLayout: null });
  try {
    run();
  } finally {
    useDockviewStore.setState({
      api: previous.api,
      currentLayoutEnvId: previous.currentLayoutEnvId,
      preMaximizeLayout: previous.preMaximizeLayout,
    });
  }
}

describe("hidden session tab persistence", () => {
  it("keeps hidden tabs scoped when switching environments in one Dockview API", () => {
    const { api } = makeReorderingAutoSessionApi();
    const appStore = makeAppStore();
    const refs = makeRefs();

    runInEnvironment(api, "env-cache-a", () => {
      runAutoSessionTabEffect(ACTIVE_SESSION_ID, appStore as never, refs as never);
      hideSessionPanel(api, HIDDEN_SESSION_ID);
    });
    runInEnvironment(api, "env-cache-b", () => {
      runAutoSessionTabEffect(ACTIVE_SESSION_ID, appStore as never, refs as never);
    });

    expect(api.getPanel(`session:${HIDDEN_SESSION_ID}`)).not.toBeNull();

    const visiblePanel = api.getPanel(`session:${HIDDEN_SESSION_ID}`);
    if (visiblePanel) api.removePanel(visiblePanel);

    runInEnvironment(api, "env-cache-a", () => {
      runAutoSessionTabEffect(ACTIVE_SESSION_ID, appStore as never, refs as never);
    });

    expect(api.getPanel(`session:${HIDDEN_SESSION_ID}`)).toBeNull();
  });
});
