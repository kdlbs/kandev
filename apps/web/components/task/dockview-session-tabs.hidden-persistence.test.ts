import { describe, expect, it } from "vitest";
import { getEnvHiddenSessions } from "@/lib/env-hidden-sessions";
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
const ENV_KEY_PREFIX = "kandev.dockview.env-hidden-sessions-v1.";
const ENV_CROSS_TASK = "env-cross-task";

/** The env id encoded in one of this suite's sessionStorage keys. */
function envIdFor(storageKey: string): string {
  return storageKey.slice(ENV_KEY_PREFIX.length) || "null-env";
}

/** Build an app-store stub whose active task owns `sessionIds` (`loaded`
 *  marks the task's session list authoritative). */
function makeTaskStore(taskId: string, sessionIds: string[], loaded = true) {
  const items = sessionIds.map((sessionId) => ({ id: sessionId, task_id: taskId }));
  return {
    getState: () => ({
      tasks: { activeTaskId: taskId },
      taskSessions: {
        items: Object.fromEntries(items.map((session) => [session.id, session])),
      },
      taskSessionsByTask: {
        itemsByTaskId: { [taskId]: items },
        loadedByTaskId: { [taskId]: loaded },
      },
    }),
  };
}

/**
 * Merge several task stores into one shared-environment state whose active
 * task switches. Hydration bookkeeping of every task stays visible to the
 * effect regardless of which task is active.
 */
function makeSharedEnvStore(taskStores: Array<ReturnType<typeof makeTaskStore>>) {
  let activeTaskId = taskStores[0]?.getState().tasks.activeTaskId ?? TASK_ID;
  return {
    setActiveTask: (taskId: string) => {
      activeTaskId = taskId;
    },
    getState: () => {
      const states = taskStores.map((store) => store.getState());
      return {
        tasks: { activeTaskId },
        taskSessions: { items: Object.assign({}, ...states.map((s) => s.taskSessions.items)) },
        taskSessionsByTask: {
          itemsByTaskId: Object.assign(
            {},
            ...states.map((s) => s.taskSessionsByTask.itemsByTaskId),
          ),
          loadedByTaskId: Object.assign(
            {},
            ...states.map((s) => s.taskSessionsByTask.loadedByTaskId),
          ),
        },
      };
    },
  };
}

function makeAppStore() {
  return makeTaskStore(TASK_ID, [ACTIVE_SESSION_ID, HIDDEN_SESSION_ID]);
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
      hideSessionPanel(api, HIDDEN_SESSION_ID, TASK_ID);
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
        hideSessionPanel(firstLoad.api, HIDDEN_SESSION_ID, TASK_ID);
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
      hideSessionPanel(api, HIDDEN_SESSION_ID, TASK_ID);
      runAutoSessionTabEffect(ACTIVE_SESSION_ID, appStore as never, refs as never);
    });

    expect(api.getPanel(`session:${HIDDEN_SESSION_ID}`)).toBeNull();
  });

  it("preserves hidden markers when switching tasks in one environment", () => {
    const { api } = makeReorderingAutoSessionApi();
    const appStore = makeSharedEnvStore([
      makeTaskStore("task-A", [ACTIVE_SESSION_ID, HIDDEN_SESSION_ID]),
      makeTaskStore("task-B", ["session-task-b"]),
    ]);
    const refs = makeRefs();

    runInEnvironment(api, "env-shared", () => {
      runAutoSessionTabEffect(ACTIVE_SESSION_ID, appStore as never, refs as never);
      hideSessionPanel(api, HIDDEN_SESSION_ID, TASK_ID);
      appStore.setActiveTask("task-B");
      runAutoSessionTabEffect("session-task-b", appStore as never, refs as never);
      appStore.setActiveTask("task-A");
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
        hideSessionPanel(firstLoad.api, HIDDEN_SESSION_ID, TASK_ID);
      });
      expect(getEnvHiddenSessions(envIdFor(storageKey))).toEqual([HIDDEN_SESSION_ID]);

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
        hideSessionPanel(api, HIDDEN_SESSION_ID, TASK_ID);
        expect(getEnvHiddenSessions(envIdFor(storageKey))).toEqual([HIDDEN_SESSION_ID]);

        // The reopen menu clears the hide intent before adding the panel.
        clearHiddenSessionPanel(api, HIDDEN_SESSION_ID);
        runAutoSessionTabEffect(ACTIVE_SESSION_ID, appStore as never, makeRefs() as never);
      });

      expect(getEnvHiddenSessions(envIdFor(storageKey))).toEqual([]);
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
        hideSessionPanel(beforeReload.api, HIDDEN_SESSION_ID, TASK_ID);
      });
      expect(getEnvHiddenSessions(envIdFor(storageKey))).toEqual([HIDDEN_SESSION_ID]);

      // Fresh API after reload; the store has no sessions and does not yet
      // mark the task's list as loaded.
      const afterReload = makeReorderingAutoSessionApi();
      const emptyStore = makeTaskStore(TASK_ID, [], false);
      runInEnvironment(afterReload.api, "env-hydrate", () => {
        runAutoSessionTabEffect(ACTIVE_SESSION_ID, emptyStore as never, makeRefs() as never);
      });

      // The record survives the pre-hydration pass.
      expect(getEnvHiddenSessions(envIdFor(storageKey))).toEqual([HIDDEN_SESSION_ID]);

      // Hydration completes: the list loads and the effect reconciles; the
      // hidden session exists, so the record keeps it and the panel stays
      // absent while the active session renders.
      runInEnvironment(afterReload.api, "env-hydrate", () => {
        runAutoSessionTabEffect(ACTIVE_SESSION_ID, appStore as never, makeRefs() as never);
      });
      expect(afterReload.api.getPanel(`session:${ACTIVE_SESSION_ID}`)).not.toBeNull();
      expect(afterReload.api.getPanel(`session:${HIDDEN_SESSION_ID}`)).toBeNull();
      expect(getEnvHiddenSessions(envIdFor(storageKey))).toEqual([HIDDEN_SESSION_ID]);
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
        hideSessionPanel(api, HIDDEN_SESSION_ID, TASK_ID);
      });
      expect(getEnvHiddenSessions(envIdFor(storageKey))).toEqual([HIDDEN_SESSION_ID]);

      // The session disappears from the store but the list has not reloaded,
      // so the record must survive until the refreshed list confirms it.
      const staleListStore = makeTaskStore(TASK_ID, [ACTIVE_SESSION_ID], false);
      runInEnvironment(api, "env-prune", () => {
        runAutoSessionTabEffect(ACTIVE_SESSION_ID, staleListStore as never, makeRefs() as never);
      });
      expect(getEnvHiddenSessions(envIdFor(storageKey))).toEqual([HIDDEN_SESSION_ID]);

      // The refreshed list loads without the hidden session; pruning removes it.
      const refreshedStore = makeTaskStore(TASK_ID, [ACTIVE_SESSION_ID]);
      runInEnvironment(api, "env-prune", () => {
        runAutoSessionTabEffect(ACTIVE_SESSION_ID, refreshedStore as never, makeRefs() as never);
      });
      expect(getEnvHiddenSessions(envIdFor(storageKey))).toEqual([]);
    } finally {
      window.sessionStorage.removeItem(storageKey);
    }
  });
});

describe("hidden session tab pruning across tasks", () => {
  it("keeps another task's hidden record while only the active task is hydrated", () => {
    // Regression: a shared environment spans tasks. The earlier gate trusted the
    // active task's loaded list, so reloading into task A and hydrating A
    // pruned task B's persisted hidden record; switching to B resurrected B's
    // panel. Each record is retired only by its owning task's list.
    const storageKey = "kandev.dockview.env-hidden-sessions-v1.env-cross-task";
    window.sessionStorage.removeItem(storageKey);
    const taskB = "task-B";
    const taskBSessionId = "session-task-b";
    const taskBHiddenSessionId = "session-task-b-hidden";

    try {
      // Before the reload, task B had two sessions and one was hidden.
      const beforeReload = makeReorderingAutoSessionApi();
      const taskBStore = makeTaskStore(taskB, [taskBSessionId, taskBHiddenSessionId]);
      runInEnvironment(beforeReload.api, ENV_CROSS_TASK, () => {
        runAutoSessionTabEffect(taskBSessionId, taskBStore as never, makeRefs() as never);
        hideSessionPanel(beforeReload.api, taskBHiddenSessionId, taskB);
      });
      expect(getEnvHiddenSessions(envIdFor(storageKey))).toEqual([taskBHiddenSessionId]);

      // Reload lands on task A; its store knows nothing about task B.
      const afterReload = makeReorderingAutoSessionApi();
      const taskAOnlyStore = makeTaskStore(TASK_ID, [ACTIVE_SESSION_ID]);
      runInEnvironment(afterReload.api, ENV_CROSS_TASK, () => {
        runAutoSessionTabEffect(ACTIVE_SESSION_ID, taskAOnlyStore as never, makeRefs() as never);
      });

      // Task A is fully authoritative, but it does not own B's record: the
      // record survives and A's reconciliation does not recreate B's panel.
      expect(getEnvHiddenSessions(envIdFor(storageKey))).toEqual([taskBHiddenSessionId]);
      expect(afterReload.api.getPanel(`session:${taskBHiddenSessionId}`)).toBeNull();

      // Switching to B with its list still unloaded must not prune it either.
      const taskBUnloadedStore = makeSharedEnvStore([
        makeTaskStore(TASK_ID, [ACTIVE_SESSION_ID]),
        makeTaskStore(taskB, [taskBSessionId], false),
      ]);
      taskBUnloadedStore.setActiveTask(taskB);
      runInEnvironment(afterReload.api, ENV_CROSS_TASK, () => {
        runAutoSessionTabEffect(taskBSessionId, taskBUnloadedStore as never, makeRefs() as never);
      });
      expect(getEnvHiddenSessions(envIdFor(storageKey))).toEqual([taskBHiddenSessionId]);
      expect(afterReload.api.getPanel(`session:${taskBHiddenSessionId}`)).toBeNull();

      // Task B hydrates with the session present: the record keeps the panel
      // hidden and the visible sibling renders.
      const taskBHydratedStore = makeTaskStore(taskB, [taskBSessionId, taskBHiddenSessionId]);
      runInEnvironment(afterReload.api, ENV_CROSS_TASK, () => {
        runAutoSessionTabEffect(taskBSessionId, taskBHydratedStore as never, makeRefs() as never);
      });
      expect(getEnvHiddenSessions(envIdFor(storageKey))).toEqual([taskBHiddenSessionId]);
      expect(afterReload.api.getPanel(`session:${taskBSessionId}`)).not.toBeNull();
      expect(afterReload.api.getPanel(`session:${taskBHiddenSessionId}`)).toBeNull();
    } finally {
      window.sessionStorage.removeItem(storageKey);
    }
  });
});
