import { expect, it, vi } from "vitest";
import type { SessionId, TaskSessionsResponse } from "@/lib/types/http";
import { fetchTaskSession, listTaskSessions } from "@/lib/api/domains/session-api";
import type * as SessionApi from "@/lib/api/domains/session-api";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { CENTER_GROUP } from "@/lib/state/layout-manager";
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
import { setupRemovalSurfaceFocus } from "./dockview-session-removal-focus";

vi.mock("@/lib/api/domains/session-api", async (original) => ({
  ...(await original<typeof SessionApi>()),
  fetchTaskSession: vi.fn(),
  listTaskSessions: vi.fn(),
}));

const LATER = "later" as SessionId;

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
  view.api.getPanel("session:old")!.api.setActive();
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
  ["disposed", "session:next"],
  ["unproven", "session:next"],
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
    const oldPanel = view.api.getPanel("session:old");
    if (oldPanel) view.api.removePanel(oldPanel);
    runAutoSessionTabEffect(null, store, refs);
    expect(view.centerActivePanelId()).toBe("session:next");
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
