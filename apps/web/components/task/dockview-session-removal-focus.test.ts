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
import { runAutoSessionTabEffect } from "./dockview-session-tabs";
import { isRemovalChatBlocked, setupRemovalSurfaceFocus } from "./dockview-session-removal-focus";
import { registerTaskSessionHandlers } from "@/lib/ws/handlers/agent-session";
import { hiddenSessionIdsFor, hideSessionPanel } from "./dockview-hidden-session-panels";

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
