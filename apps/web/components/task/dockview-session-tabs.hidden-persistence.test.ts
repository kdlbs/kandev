import { describe, expect, it } from "vitest";
import { useDockviewStore } from "@/lib/state/dockview-store";
import {
  clearHiddenSessionPanel,
  hideSessionPanel,
  runAutoSessionTabEffect,
} from "./dockview-session-tabs";
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
        loadedByTaskId: { [TASK_ID]: true },
      },
    }),
  };
}

function makeRefs() {
  return {
    sessionTabCreatedRef: { current: new Set<string>() },
    hiddenSessionIdsRef: { current: new Set<string>() },
    hiddenSessionApiRef: { current: null },
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

describe("hidden session tab state", () => {
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

  it("refreshes hidden state when the Dockview API is replaced in the same environment", () => {
    const firstLoad = makeReorderingAutoSessionApi();
    const refs = makeRefs();
    const appStore = makeAppStore();
    const storageKey = "kandev.dockview.env-hidden-sessions-v1.env-reload";
    window.sessionStorage.removeItem(storageKey);

    try {
      runInEnvironment(firstLoad.api, "env-reload", () => {
        runAutoSessionTabEffect(ACTIVE_SESSION_ID, appStore as never, refs as never);
        hideSessionPanel(firstLoad.api, HIDDEN_SESSION_ID);
      });

      const reloaded = makeReorderingAutoSessionApi();
      runInEnvironment(reloaded.api, "env-reload", () => {
        runAutoSessionTabEffect(ACTIVE_SESSION_ID, appStore as never, refs as never);
      });

      // A fresh Dockview API after reload rehydrates the persisted hide
      // record, so the explicitly closed panel stays absent until reopened.
      expect(reloaded.api.getPanel(`session:${ACTIVE_SESSION_ID}`)).not.toBeNull();
      expect(reloaded.api.getPanel(`session:${HIDDEN_SESSION_ID}`)).toBeNull();
    } finally {
      window.sessionStorage.removeItem(storageKey);
    }
  });

  it("keeps a closed panel absent during layout-only reconciliation", () => {
    const { api } = makeReorderingAutoSessionApi();
    const refs = makeRefs();
    const appStore = makeAppStore();

    runInEnvironment(api, "env-layout", () => {
      runAutoSessionTabEffect(ACTIVE_SESSION_ID, appStore as never, refs as never);
      hideSessionPanel(api, HIDDEN_SESSION_ID);
      runAutoSessionTabEffect(ACTIVE_SESSION_ID, appStore as never, refs as never);
    });

    expect(api.getPanel(`session:${HIDDEN_SESSION_ID}`)).toBeNull();
  });

  it("preserves hidden markers when switching tasks in one environment", () => {
    const { api } = makeReorderingAutoSessionApi();
    let activeTaskId = "task-A";
    const appStore = {
      getState: () => ({
        tasks: { activeTaskId },
        taskSessionsByTask: {
          itemsByTaskId: {
            "task-A": [{ id: ACTIVE_SESSION_ID }, { id: HIDDEN_SESSION_ID }],
            "task-B": [{ id: "session-task-b" }],
          },
          loadedByTaskId: { "task-A": true, "task-B": true },
        },
      }),
    };
    const refs = makeRefs();

    runInEnvironment(api, "env-shared", () => {
      runAutoSessionTabEffect(ACTIVE_SESSION_ID, appStore as never, refs as never);
      hideSessionPanel(api, HIDDEN_SESSION_ID);
      activeTaskId = "task-B";
      runAutoSessionTabEffect("session-task-b", appStore as never, refs as never);
      activeTaskId = "task-A";
      runAutoSessionTabEffect(ACTIVE_SESSION_ID, appStore as never, refs as never);
    });

    expect(api.getPanel(`session:${HIDDEN_SESSION_ID}`)).toBeNull();
    expect(refs.hiddenSessionIdsRef.current.has(HIDDEN_SESSION_ID)).toBe(true);
  });

  it("keeps an automatically selected hidden fallback closed", () => {
    const { api } = makeReorderingAutoSessionApi();
    const appStore = makeAppStore();
    const refs = makeRefs();

    runInEnvironment(api, "env-fallback", () => {
      runAutoSessionTabEffect(ACTIVE_SESSION_ID, appStore as never, refs as never);
      const hiddenPanel = api.getPanel(`session:${HIDDEN_SESSION_ID}`);
      expect(hiddenPanel).not.toBeNull();
      if (hiddenPanel) api.removePanel(hiddenPanel);
      refs.hiddenSessionIdsRef.current.add(HIDDEN_SESSION_ID);

      runAutoSessionTabEffect(HIDDEN_SESSION_ID, appStore as never, refs as never);
    });

    expect(api.getPanel(`session:${HIDDEN_SESSION_ID}`)).toBeNull();
    expect(refs.hiddenSessionIdsRef.current.has(HIDDEN_SESSION_ID)).toBe(true);
  });
});

describe("hidden session tab persistence record", () => {
  it("persists the hide record so a fresh Dockview API after reload keeps the panel absent", () => {
    const storageKey = "kandev.dockview.env-hidden-sessions-v1.env-reload-record";
    window.sessionStorage.removeItem(storageKey);

    try {
      const firstLoad = makeReorderingAutoSessionApi();
      const appStore = makeAppStore();

      runInEnvironment(firstLoad.api, "env-reload-record", () => {
        runAutoSessionTabEffect(ACTIVE_SESSION_ID, appStore as never, makeRefs() as never);
        hideSessionPanel(firstLoad.api, HIDDEN_SESSION_ID);
      });
      expect(JSON.parse(window.sessionStorage.getItem(storageKey) ?? "[]")).toEqual([
        HIDDEN_SESSION_ID,
      ]);

      const afterReload = makeReorderingAutoSessionApi();
      runInEnvironment(afterReload.api, "env-reload-record", () => {
        runAutoSessionTabEffect(ACTIVE_SESSION_ID, appStore as never, makeRefs() as never);
      });

      expect(afterReload.api.getPanel(`session:${ACTIVE_SESSION_ID}`)).not.toBeNull();
      expect(afterReload.api.getPanel(`session:${HIDDEN_SESSION_ID}`)).toBeNull();
    } finally {
      window.sessionStorage.removeItem(storageKey);
    }
  });

  it("clears the persisted hide record when the hidden session is reopened", () => {
    const storageKey = "kandev.dockview.env-hidden-sessions-v1.env-reopen";
    window.sessionStorage.removeItem(storageKey);

    try {
      const { api } = makeReorderingAutoSessionApi();
      const appStore = makeAppStore();

      runInEnvironment(api, "env-reopen", () => {
        runAutoSessionTabEffect(ACTIVE_SESSION_ID, appStore as never, makeRefs() as never);
        hideSessionPanel(api, HIDDEN_SESSION_ID);
        expect(JSON.parse(window.sessionStorage.getItem(storageKey) ?? "[]")).toEqual([
          HIDDEN_SESSION_ID,
        ]);

        // The reopen menu clears the hide intent before adding the panel.
        clearHiddenSessionPanel(api, HIDDEN_SESSION_ID);
        runAutoSessionTabEffect(ACTIVE_SESSION_ID, appStore as never, makeRefs() as never);
      });

      expect(JSON.parse(window.sessionStorage.getItem(storageKey) ?? "[]")).toEqual([]);
    } finally {
      window.sessionStorage.removeItem(storageKey);
    }
  });
});

describe("hidden session tab pruning after hydration", () => {
  it("defers pruning when the session list starts empty and hydrates later", () => {
    // Regression: after a reload the effect can run before the task's session
    // list hydrates. Pruning the persisted hide record against that empty list
    // erased it, so the hidden panel resurrected once the sessions arrived.
    const storageKey = "kandev.dockview.env-hidden-sessions-v1.env-hydrate";
    window.sessionStorage.removeItem(storageKey);

    try {
      const beforeReload = makeReorderingAutoSessionApi();
      const appStore = makeAppStore();

      runInEnvironment(beforeReload.api, "env-hydrate", () => {
        runAutoSessionTabEffect(ACTIVE_SESSION_ID, appStore as never, makeRefs() as never);
        hideSessionPanel(beforeReload.api, HIDDEN_SESSION_ID);
      });
      expect(JSON.parse(window.sessionStorage.getItem(storageKey) ?? "[]")).toEqual([
        HIDDEN_SESSION_ID,
      ]);

      // Fresh API after reload; the store has no sessions and does not yet
      // mark the task's list as loaded.
      const afterReload = makeReorderingAutoSessionApi();
      const emptyStore = {
        getState: () => ({
          tasks: { activeTaskId: TASK_ID },
          taskSessionsByTask: {
            itemsByTaskId: {},
            loadedByTaskId: {},
          },
        }),
      };
      runInEnvironment(afterReload.api, "env-hydrate", () => {
        runAutoSessionTabEffect(ACTIVE_SESSION_ID, emptyStore as never, makeRefs() as never);
      });

      // The record survives the pre-hydration pass.
      expect(JSON.parse(window.sessionStorage.getItem(storageKey) ?? "[]")).toEqual([
        HIDDEN_SESSION_ID,
      ]);

      // Hydration completes: the list loads and the effect reconciles; the
      // hidden session exists, so the record keeps it and the panel stays
      // absent while the active session renders.
      runInEnvironment(afterReload.api, "env-hydrate", () => {
        runAutoSessionTabEffect(ACTIVE_SESSION_ID, appStore as never, makeRefs() as never);
      });
      expect(afterReload.api.getPanel(`session:${ACTIVE_SESSION_ID}`)).not.toBeNull();
      expect(afterReload.api.getPanel(`session:${HIDDEN_SESSION_ID}`)).toBeNull();
      expect(JSON.parse(window.sessionStorage.getItem(storageKey) ?? "[]")).toEqual([
        HIDDEN_SESSION_ID,
      ]);
    } finally {
      window.sessionStorage.removeItem(storageKey);
    }
  });

  it("prunes the persisted record only once hydration is authoritative", () => {
    const storageKey = "kandev.dockview.env-hidden-sessions-v1.env-prune";
    window.sessionStorage.removeItem(storageKey);

    try {
      const { api } = makeReorderingAutoSessionApi();
      const appStore = makeAppStore();

      runInEnvironment(api, "env-prune", () => {
        runAutoSessionTabEffect(ACTIVE_SESSION_ID, appStore as never, makeRefs() as never);
        hideSessionPanel(api, HIDDEN_SESSION_ID);
      });
      expect(JSON.parse(window.sessionStorage.getItem(storageKey) ?? "[]")).toEqual([
        HIDDEN_SESSION_ID,
      ]);

      // The session disappears from the store but the list has not reloaded,
      // so the record must survive until the refreshed list confirms it.
      const staleListStore = {
        getState: () => ({
          tasks: { activeTaskId: TASK_ID },
          taskSessionsByTask: {
            itemsByTaskId: { [TASK_ID]: [{ id: ACTIVE_SESSION_ID }] },
            loadedByTaskId: { [TASK_ID]: false },
          },
        }),
      };
      runInEnvironment(api, "env-prune", () => {
        runAutoSessionTabEffect(ACTIVE_SESSION_ID, staleListStore as never, makeRefs() as never);
      });
      expect(JSON.parse(window.sessionStorage.getItem(storageKey) ?? "[]")).toEqual([
        HIDDEN_SESSION_ID,
      ]);

      // The refreshed list loads without the hidden session; pruning removes it.
      const refreshedStore = {
        getState: () => ({
          tasks: { activeTaskId: TASK_ID },
          taskSessionsByTask: {
            itemsByTaskId: { [TASK_ID]: [{ id: ACTIVE_SESSION_ID }] },
            loadedByTaskId: { [TASK_ID]: true },
          },
        }),
      };
      runInEnvironment(api, "env-prune", () => {
        runAutoSessionTabEffect(ACTIVE_SESSION_ID, refreshedStore as never, makeRefs() as never);
      });
      expect(JSON.parse(window.sessionStorage.getItem(storageKey) ?? "[]")).toEqual([]);
    } finally {
      window.sessionStorage.removeItem(storageKey);
    }
  });
});
