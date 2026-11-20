import { expect, it, vi } from "vitest";
import type { SessionId, TaskSessionsResponse } from "@/lib/types/http";
import { fetchTaskSession, listTaskSessions } from "@/lib/api/domains/session-api";
import type * as SessionApi from "@/lib/api/domains/session-api";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { CENTER_GROUP, RIGHT_TOP_GROUP } from "@/lib/state/layout-manager";
import { prepareSessionRemoval } from "@/lib/ws/handlers/task-primary-focus";
import {
  flushFocusWork,
  membership,
  promote,
  response,
  withSiblings,
} from "@/lib/ws/handlers/task-primary-focus.test-helpers";
import { makeReorderingAutoSessionApi } from "./dockview-session-tabs.test-utils";
import { runAutoSessionTabEffect, setupChatPanelSafetyNet } from "./dockview-session-tabs";
import { isRemovalChatBlocked, setupRemovalSurfaceFocus } from "./dockview-session-removal-focus";
import { registerTaskSessionHandlers } from "@/lib/ws/handlers/agent-session";
import { hiddenSessionIdsFor, hideSessionPanel } from "./dockview-hidden-session-panels";
import { setupSessionTabSync } from "./dockview-session-tab-sync";
import { markSessionTabUserActivationIntent } from "./session-tab-activation-intent";

vi.mock("@/lib/api/domains/session-api", async (original) => ({
  ...(await original<typeof SessionApi>()),
  fetchTaskSession: vi.fn(),
  listTaskSessions: vi.fn(),
}));

const LATER = "later" as SessionId;
const OLD_PANEL_ID = "session:old";
const NEXT_PANEL_ID = "session:next";
const LATEST_PANEL_ID = "session:latest";
const REMOVED_ACTION = "session.removed";

function scene() {
  const store = withSiblings("FAILED");
  store.getState().setTaskSession({ ...response("WAITING_FOR_INPUT", LATER).session });
  const rows = Object.values(store.getState().taskSessions.items);
  store.getState().setTaskSessionsForTask("t1", rows, {});
  store.getState().setActiveTask("other-task");
  promote(store, "latest");
  store.getState().setActiveSessionAuto("t1", "old");
  const view = makeReorderingAutoSessionApi();
  const chat = view.api.getPanel("chat");
  if (chat) view.api.removePanel(chat);
  const ids = ["old", "next", "latest", LATER];
  for (const id of [...ids].reverse()) {
    view.api.addPanel({
      id: `session:${id}`,
      component: "chat",
      inactive: true,
      position: { referenceGroup: CENTER_GROUP, index: 0 },
    });
  }
  view.api.getPanel(OLD_PANEL_ID)!.api.setActive();
  const refs = {
    sessionTabCreatedRef: { current: new Set(ids) },
    prevTaskIdRef: { current: "t1" as string | null },
    prevSessionIdRef: { current: "old" as string | null },
  };
  return { store, view, refs };
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
// @covers AC-TASKS-TASK-UNREAD-DOT-001.11
it.each([
  ["follow", "session:latest"],
  ["pointer", "plan"],
  ["keyboard", "plan"],
  ["disposed", NEXT_PANEL_ID],
  ["unproven", NEXT_PANEL_ID],
  ["retarget", `session:${LATER}`],
] as const)("keeps the removal surface owned through %s", async (mode, expected) => {
  vi.mocked(fetchTaskSession).mockReset();
  vi.mocked(listTaskSessions).mockReset();
  const { store, view, refs } = scene();
  const originalDockview = useDockviewStore.getState();
  useDockviewStore.setState({ api: view.api, isRestoringLayout: false, preMaximizeLayout: null });
  const stop = setupRemovalSurfaceFocus(view.api, store);
  const proof = Promise.withResolvers<TaskSessionsResponse>();
  vi.mocked(listTaskSessions).mockReturnValue(proof.promise);
  try {
    if (mode === "disposed") stop();
    if (mode === "unproven") {
      store.getState().clearActiveSession();
      store.getState().removeTaskSession("t1", "old", { confirmed: true });
    } else {
      prepareSessionRemoval(store, "old", "t1")();
    }
    const oldPanel = view.api.getPanel(OLD_PANEL_ID);
    if (oldPanel) view.api.removePanel(oldPanel);
    runAutoSessionTabEffect(null, store, refs);
    expect(view.centerActivePanelId()).toBe(NEXT_PANEL_ID);
    if (mode === "pointer" || mode === "keyboard") {
      const event =
        mode === "pointer"
          ? new MouseEvent("pointerdown", { bubbles: true })
          : new KeyboardEvent("keydown", { key: "Enter", bubbles: true });
      window.dispatchEvent(event);
      view.api.getPanel("plan")!.api.setActive();
    }
    if (mode === "unproven") store.getState().setActiveSessionAuto("t1", "latest");
    proof.resolve(membership("latest", "next", LATER));
    await flushFocusWork();
    expect(store.getState().tasks.activeSessionId).toBe("latest");
    if (mode === "retarget") {
      vi.mocked(fetchTaskSession).mockResolvedValue(response("WAITING_FOR_INPUT", "latest"));
      promote(store, LATER);
      await flushFocusWork();
      expect(store.getState().tasks.activeSessionId).toBe(LATER);
    }
    const target = store.getState().tasks.activeSessionId;
    runAutoSessionTabEffect(target, store, refs);
    expect(view.centerActivePanelId()).toBe(expected);
  } finally {
    stop();
    useDockviewStore.setState(originalDockview);
  }
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6
it.each([false, true])(
  "preserves an independently visible split chat (focused: %s)",
  async (independentFocused) => {
    vi.mocked(fetchTaskSession).mockReset();
    vi.mocked(listTaskSessions).mockReset();
    const { store, view, refs } = scene();
    const independent = view.api.getPanel(`session:${LATER}`)!;
    view.api.removePanel(independent);
    view.api.addPanel({
      id: `session:${LATER}`,
      component: "chat",
      position: { referenceGroup: RIGHT_TOP_GROUP },
    });
    if (!independentFocused) view.api.getPanel(OLD_PANEL_ID)!.api.setActive();
    const originalDockview = useDockviewStore.getState();
    useDockviewStore.setState({ api: view.api, isRestoringLayout: false, preMaximizeLayout: null });
    const stop = setupRemovalSurfaceFocus(view.api, store);
    const proof = Promise.withResolvers<TaskSessionsResponse>();
    vi.mocked(listTaskSessions).mockReturnValue(proof.promise);
    try {
      prepareSessionRemoval(store, "old", "t1")();
      view.api.removePanel(view.api.getPanel(OLD_PANEL_ID)!);
      runAutoSessionTabEffect(null, store, refs);
      expect(view.centerActivePanelId()).toBe(NEXT_PANEL_ID);
      expect(isRemovalChatBlocked(view.api, store, NEXT_PANEL_ID, store.getState())).toBe(true);
      expect(isRemovalChatBlocked(view.api, store, `session:${LATER}`, store.getState())).toBe(
        false,
      );
      store.getState().setActiveSession("t1", "next");
      expect(isRemovalChatBlocked(view.api, store, NEXT_PANEL_ID, store.getState())).toBe(false);
      proof.resolve(membership("latest", "next", LATER));
      await flushFocusWork();
      expect(store.getState().tasks.activeSessionId).toBe("next");
    } finally {
      stop();
      useDockviewStore.setState(originalDockview);
    }
  },
);

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6, AC-TASKS-TASK-UNREAD-DOT-001.11
it.each([false, true])(
  "fences a direct remote removal across split groups (recipient focused: %s)",
  async (recipientFocused) => {
    vi.mocked(fetchTaskSession).mockReset();
    vi.mocked(listTaskSessions).mockReset();
    const { store, view, refs } = scene();
    view.api.removePanel(view.api.getPanel(LATEST_PANEL_ID)!);
    view.api.addPanel({
      id: LATEST_PANEL_ID,
      component: "chat",
      position: { referenceGroup: RIGHT_TOP_GROUP },
    });
    if (!recipientFocused) view.api.getPanel(OLD_PANEL_ID)!.api.setActive();
    const originalDockview = useDockviewStore.getState();
    useDockviewStore.setState({
      api: view.api,
      currentLayoutEnvId: null,
      isRestoringLayout: false,
      preMaximizeLayout: null,
    });
    const stop = setupRemovalSurfaceFocus(view.api, store);
    const proof = Promise.withResolvers<TaskSessionsResponse>();
    vi.mocked(listTaskSessions).mockReturnValue(proof.promise);
    try {
      registerTaskSessionHandlers(store)[REMOVED_ACTION]!({
        id: "remote-split-removal",
        type: "notification",
        action: REMOVED_ACTION,
        payload: { task_id: "t1", session_id: "old" },
      });
      expect(store.getState().tasks.activeSessionId).toBe("old");
      proof.resolve(membership("latest", "next", LATER));
      await flushFocusWork();
      expect(store.getState().tasks.activeSessionId).toBe("latest");
      runAutoSessionTabEffect("latest", store, refs);
      expect(view.api.getPanel(OLD_PANEL_ID)).toBeNull();
      expect(view.centerActivePanelId()).toBe(NEXT_PANEL_ID);
      expect(isRemovalChatBlocked(view.api, store, NEXT_PANEL_ID, store.getState())).toBe(true);
      expect(view.api.getPanel(LATEST_PANEL_ID)!.group.activePanel?.id).toBe(LATEST_PANEL_ID);
      expect(isRemovalChatBlocked(view.api, store, LATEST_PANEL_ID, store.getState())).toBe(false);
      store.getState().setActiveSession("t1", "next");
      expect(isRemovalChatBlocked(view.api, store, NEXT_PANEL_ID, store.getState())).toBe(false);
    } finally {
      stop();
      useDockviewStore.setState(originalDockview);
    }
  },
);

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6, AC-TASKS-TASK-UNREAD-DOT-001.12
it("keeps an owned group empty when its proven primary is hidden", async () => {
  vi.mocked(fetchTaskSession).mockReset();
  vi.mocked(listTaskSessions).mockReset();
  const { store, view, refs } = scene();
  const originalDockview = useDockviewStore.getState();
  useDockviewStore.setState({
    api: view.api,
    currentLayoutEnvId: null,
    isRestoringLayout: false,
    preMaximizeLayout: null,
  });
  hideSessionPanel(view.api, "latest", "t1");
  const stop = setupRemovalSurfaceFocus(view.api, store);
  const proof = Promise.withResolvers<TaskSessionsResponse>();
  vi.mocked(listTaskSessions).mockReturnValue(proof.promise);
  try {
    prepareSessionRemoval(store, "old", "t1")();
    runAutoSessionTabEffect(null, store, refs);
    expect(isRemovalChatBlocked(view.api, store, NEXT_PANEL_ID, store.getState())).toBe(true);
    proof.resolve(membership("latest", "next", LATER));
    await flushFocusWork();
    runAutoSessionTabEffect("latest", store, refs);
    expect(store.getState().tasks.activeSessionId).toBe("latest");
    expect(hiddenSessionIdsFor(view.api).has("latest")).toBe(true);
    expect(view.api.getPanel(LATEST_PANEL_ID)).toBeNull();
    expect(isRemovalChatBlocked(view.api, store, NEXT_PANEL_ID, store.getState())).toBe(true);
    store.getState().setActiveSession("t1", "next");
    runAutoSessionTabEffect("next", store, refs);
    expect(isRemovalChatBlocked(view.api, store, NEXT_PANEL_ID, store.getState())).toBe(false);
  } finally {
    stop();
    useDockviewStore.setState(originalDockview);
  }
});

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6, AC-TASKS-TASK-UNREAD-DOT-001.12
it.each(["reconcile", "implicit", "hide-other"] as const)(
  "keeps a directly proven hidden recipient closed across passive input: %s",
  async (mode) => {
    const { store, view, refs } = scene();
    const original = useDockviewStore.getState();
    useDockviewStore.setState({
      api: view.api,
      currentLayoutEnvId: null,
      isRestoringLayout: false,
      preMaximizeLayout: null,
    });
    hideSessionPanel(view.api, "latest", "t1");
    let activated: ((panel: { id: string }) => void) | undefined;
    Object.assign(view.api, {
      onDidActivePanelChange: (callback: typeof activated) => {
        activated = callback;
        return {
          dispose: () => {
            activated = undefined;
          },
        };
      },
    });
    const stop = setupRemovalSurfaceFocus(view.api, store);
    const sync = setupSessionTabSync(view.api, store);
    const proof = Promise.withResolvers<TaskSessionsResponse>();
    vi.mocked(listTaskSessions).mockReturnValue(proof.promise);
    try {
      registerTaskSessionHandlers(store)[REMOVED_ACTION]!({
        id: "direct-hidden-removal",
        type: "notification",
        action: REMOVED_ACTION,
        payload: { task_id: "t1", session_id: "old" },
      });
      proof.resolve(membership("latest", "next", LATER));
      await flushFocusWork();
      expect(store.getState().tasks.activeSessionId).toBe("latest");
      if (mode === "reconcile") runAutoSessionTabEffect("latest", store, refs);
      else view.api.removePanel(view.api.getPanel(OLD_PANEL_ID)!);
      expect(view.api.getPanel(LATEST_PANEL_ID)).toBeNull();
      if (mode === "hide-other") {
        hideSessionPanel(view.api, LATER, "t1");
        await Promise.resolve();
      } else activated?.({ id: NEXT_PANEL_ID });
      expect(store.getState().tasks.activeSessionId).toBe("latest");
      expect(isRemovalChatBlocked(view.api, store, NEXT_PANEL_ID, store.getState())).toBe(true);
      markSessionTabUserActivationIntent("next");
      activated?.({ id: NEXT_PANEL_ID });
      expect(store.getState().tasks.activeSessionId).toBe("next");
      expect(isRemovalChatBlocked(view.api, store, NEXT_PANEL_ID, store.getState())).toBe(false);
    } finally {
      sync.dispose();
      stop();
      useDockviewStore.setState(original);
    }
  },
);

// @covers AC-TASKS-TASK-UNREAD-DOT-001.12
it.each([false, true])(
  "last-chat recovery respects a recipient hidden between frames: %s",
  (hidden) => {
    const { store, view } = scene();
    const original = useDockviewStore.getState();
    useDockviewStore.setState({
      api: view.api,
      currentLayoutEnvId: null,
      isRestoringLayout: false,
      preMaximizeLayout: null,
    });
    const frames: FrameRequestCallback[] = [];
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) =>
      frames.push(callback),
    );
    let removed: ((panel: { id: string }) => void) | undefined;
    Object.assign(view.api, {
      onDidRemovePanel: (callback: typeof removed) => {
        removed = callback;
        return {
          dispose: () => {
            removed = undefined;
          },
        };
      },
    });
    const safety = setupChatPanelSafetyNet(view.api, store);
    try {
      for (const panel of [...view.api.panels]) {
        if (panel.id.startsWith("session:")) view.api.removePanel(panel);
      }
      removed?.({ id: OLD_PANEL_ID });
      frames.shift()!(0);
      store.getState().setActiveSessionAuto("t1", "latest");
      if (hidden) hideSessionPanel(view.api, "latest", "t1");
      frames.shift()!(16);
      expect(view.api.getPanel(LATEST_PANEL_ID) !== null).toBe(!hidden);
    } finally {
      safety.dispose();
      vi.unstubAllGlobals();
      useDockviewStore.setState(original);
    }
  },
);

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6, AC-TASKS-TASK-UNREAD-DOT-001.12
it.each(["pending", "hidden"] as const)(
  "retains the removed group's ownership across same-environment restore: %s",
  async (phase) => {
    const { store, view, refs } = scene();
    const original = useDockviewStore.getState();
    useDockviewStore.setState({
      api: view.api,
      currentLayoutEnvId: "removal-restore",
      isRestoringLayout: false,
      preMaximizeLayout: null,
    });
    store.setState({
      environmentIdBySessionId: Object.fromEntries(
        ["old", "next", "latest", LATER].map((id) => [id, "removal-restore"]),
      ),
    });
    hideSessionPanel(view.api, "latest", "t1");
    Object.assign(view.api, {
      onDidActivePanelChange: () => ({ dispose: () => undefined }),
    });
    const stop = setupRemovalSurfaceFocus(view.api, store);
    const sync = setupSessionTabSync(view.api, store);
    const proof = Promise.withResolvers<TaskSessionsResponse>();
    vi.mocked(listTaskSessions).mockReturnValue(proof.promise);
    try {
      prepareSessionRemoval(store, "old", "t1")();
      runAutoSessionTabEffect(null, store, refs);
      if (phase === "hidden") {
        proof.resolve(membership("latest", "next", LATER));
        await flushFocusWork();
        runAutoSessionTabEffect("latest", store, refs);
        const requestId = store.getState().beginWorkflowSessionFocus({
          taskId: "t1",
          workflowId: "wf1",
          destinationStepId: "review",
          presentationToken: 1,
          navigationRevision: store.getState().taskRemoval.navigationRevision,
        });
        store.getState().cancelWorkflowSessionFocus({ requestId: requestId! });
      }
      const selected = phase === "hidden" ? "latest" : null;
      expect(isRemovalChatBlocked(view.api, store, NEXT_PANEL_ID, store.getState())).toBe(true);
      const groups = [...view.api.groups];
      useDockviewStore.setState({ isRestoringLayout: true });
      Object.assign(view.api, { groups: [] });
      store.setState({ tasks: { ...store.getState().tasks } });
      useDockviewStore.setState({
        preMaximizeLayout: {
          columns: [{ id: "parked", groups: [{ id: groups[0].id, panels: [] }] }],
        },
        isRestoringLayout: false,
      });
      const getPanel = view.api.getPanel;
      Object.assign(view.api, { getPanel: () => null });
      expect(isRemovalChatBlocked(view.api, store, NEXT_PANEL_ID, store.getState())).toBe(true);
      Object.assign(view.api, { getPanel });
      useDockviewStore.setState({ isRestoringLayout: true, preMaximizeLayout: null });
      Object.assign(view.api, { groups });
      expect(isRemovalChatBlocked(view.api, store, NEXT_PANEL_ID, store.getState())).toBe(true);
      useDockviewStore.setState({ isRestoringLayout: false });
      expect(store.getState().tasks.activeSessionId).toBe(selected);
      expect(isRemovalChatBlocked(view.api, store, NEXT_PANEL_ID, store.getState())).toBe(true);
      if (phase === "pending") {
        proof.resolve(membership("latest", "next", LATER));
        await flushFocusWork();
        expect(store.getState().tasks.activeSessionId).toBe("latest");
      }
      store.getState().setActiveSession("t1", "next");
      expect(isRemovalChatBlocked(view.api, store, NEXT_PANEL_ID, store.getState())).toBe(false);
    } finally {
      sync.dispose();
      stop();
      useDockviewStore.setState(original);
    }
  },
);

// @covers AC-TASKS-TASK-UNREAD-DOT-001.6, AC-TASKS-TASK-UNREAD-DOT-001.12
it.each([false, true, "background", "restoring"] as const)(
  "owns a predecessor already parked before remote removal (mode: %s)",
  async (mode) => {
    const hidden = mode === true;
    const { store, view, refs } = scene();
    const original = useDockviewStore.getState();
    useDockviewStore.setState({
      api: view.api,
      currentLayoutEnvId: null,
      isRestoringLayout: false,
      preMaximizeLayout: null,
    });
    if (hidden) hideSessionPanel(view.api, "latest", "t1");
    const groups = [...view.api.groups];
    const parked = {
      columns: [
        {
          id: "center",
          groups: [
            {
              id: groups[0].id,
              activePanel: OLD_PANEL_ID,
              panels: groups[0].panels.map((panel) => ({
                id: panel.id,
                component: panel.id.startsWith("session:") ? "chat" : "plan",
                title: "Panel",
              })),
            },
          ],
        },
      ],
    };
    useDockviewStore.setState({ preMaximizeLayout: parked });
    const getPanel = view.api.getPanel;
    Object.assign(view.api, {
      groups: groups.slice(1),
      getPanel: (id: string) =>
        groups[0].panels.some((panel) => panel.id === id) ? null : getPanel(id),
    });
    if (mode === "background") store.getState().setActiveSessionAuto("t1", "latest");
    const stop = setupRemovalSurfaceFocus(view.api, store);
    const proof = Promise.withResolvers<TaskSessionsResponse>();
    vi.mocked(listTaskSessions).mockReturnValue(proof.promise);
    try {
      registerTaskSessionHandlers(store)[REMOVED_ACTION]!({
        id: "parked-remote-removal",
        type: "notification",
        action: REMOVED_ACTION,
        payload: { task_id: "t1", session_id: "old" },
      });
      if (mode === "restoring") useDockviewStore.setState({ isRestoringLayout: true });
      proof.resolve(membership("latest", "next", LATER));
      await flushFocusWork();
      if (mode === "restoring") useDockviewStore.setState({ isRestoringLayout: false });
      runAutoSessionTabEffect("latest", store, refs);
      if (hidden) {
        const savedIds = useDockviewStore
          .getState()
          .preMaximizeLayout?.columns.flatMap((column) =>
            column.groups.flatMap((group) => group.panels.map((panel) => panel.id)),
          );
        expect(savedIds).not.toContain(LATEST_PANEL_ID);
      }
      expect(isRemovalChatBlocked(view.api, store, NEXT_PANEL_ID, store.getState())).toBe(true);
      Object.assign(view.api, { groups, getPanel });
      expect(isRemovalChatBlocked(view.api, store, NEXT_PANEL_ID, store.getState())).toBe(true);
      expect(isRemovalChatBlocked(view.api, store, LATEST_PANEL_ID, store.getState())).toBe(false);
      store.getState().setActiveSession("t1", "next");
      expect(isRemovalChatBlocked(view.api, store, NEXT_PANEL_ID, store.getState())).toBe(false);
    } finally {
      stop();
      Object.assign(view.api, { groups, getPanel });
      useDockviewStore.setState(original);
    }
  },
);
