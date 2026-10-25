import { describe, expect, it } from "vitest";
import type { TaskSession } from "@/lib/types/http";
import { createAppStore } from "@/lib/state/store";
import { getQuickChatSetupSessionId } from "./quick-chat-session";
import {
  selectQuickChatActivity,
  selectQuickChatSessionIsWorking,
} from "./quick-chat-activity-selectors";

const WORKSPACE_A = "workspace-a";
const WORKSPACE_B = "workspace-b";
const SESSION_A = "session-a";
const SESSION_B = "session-b";

function session(
  id: string,
  state: TaskSession["state"],
  foreground_activity?: TaskSession["foreground_activity"],
): TaskSession {
  return {
    id,
    task_id: `task-${id}`,
    state,
    foreground_activity,
  } as TaskSession;
}

function addSessions(
  store: ReturnType<typeof createAppStore>,
  sessions: Array<{ sessionId: string; workspaceId: string }>,
) {
  store.setState((state) => ({
    ...state,
    quickChat: {
      ...state.quickChat,
      sessions: sessions.map(({ sessionId, workspaceId }) => ({
        sessionId,
        workspaceId,
        kind: "chat" as const,
        taskId: `task-${sessionId}`,
      })),
    },
  }));
}

describe("selectQuickChatSessionIsWorking", () => {
  it("includes session activity and environment preparation", () => {
    const store = createAppStore();
    store.setState((state) => ({
      ...state,
      taskSessions: {
        items: {
          [SESSION_A]: session(SESSION_A, "WAITING_FOR_INPUT", "background"),
          [SESSION_B]: session(SESSION_B, "IDLE"),
        },
      },
      prepareProgress: {
        bySessionId: {
          [SESSION_B]: { sessionId: SESSION_B, status: "preparing", steps: [] },
        },
      },
    }));

    expect(selectQuickChatSessionIsWorking(store.getState(), SESSION_A)).toBe(true);
    expect(selectQuickChatSessionIsWorking(store.getState(), SESSION_B)).toBe(true);
  });

  it("returns false after a session settles without preparation", () => {
    const store = createAppStore();
    store.setState((state) => ({
      ...state,
      taskSessions: { items: { [SESSION_A]: session(SESSION_A, "WAITING_FOR_INPUT") } },
    }));

    expect(selectQuickChatSessionIsWorking(store.getState(), SESSION_A)).toBe(false);
    expect(selectQuickChatSessionIsWorking(store.getState(), "missing-session")).toBe(false);
  });
});

describe("selectQuickChatActivity", () => {
  it("returns null when the workspace is absent or the dialog is open", () => {
    const store = createAppStore();
    addSessions(store, [{ sessionId: SESSION_A, workspaceId: WORKSPACE_A }]);

    expect(selectQuickChatActivity(store.getState(), null)).toBeNull();

    store.setState((state) => ({
      ...state,
      quickChat: { ...state.quickChat, isOpen: true },
    }));
    expect(selectQuickChatActivity(store.getState(), WORKSPACE_A)).toBeNull();
  });

  it("returns running before finished when any workspace conversation is working", () => {
    const store = createAppStore();
    addSessions(store, [
      { sessionId: SESSION_A, workspaceId: WORKSPACE_A },
      { sessionId: SESSION_B, workspaceId: WORKSPACE_A },
    ]);
    store.setState((state) => ({
      ...state,
      taskSessions: {
        items: {
          [SESSION_A]: session(SESSION_A, "WAITING_FOR_INPUT"),
          [SESSION_B]: session(SESSION_B, "RUNNING"),
        },
      },
      quickChat: {
        ...state.quickChat,
        unseenIdleByWorkspace: { [WORKSPACE_A]: { [SESSION_A]: true } },
      },
    }));

    expect(selectQuickChatActivity(store.getState(), WORKSPACE_A)).toBe("running");
  });

  it("returns finished only for unseen settled conversations in the requested workspace", () => {
    const store = createAppStore();
    addSessions(store, [
      { sessionId: SESSION_A, workspaceId: WORKSPACE_A },
      { sessionId: SESSION_B, workspaceId: WORKSPACE_B },
      { sessionId: getQuickChatSetupSessionId(WORKSPACE_A, "chat"), workspaceId: WORKSPACE_A },
    ]);
    store.setState((state) => ({
      ...state,
      taskSessions: {
        items: {
          [SESSION_A]: session(SESSION_A, "COMPLETED"),
          [SESSION_B]: session(SESSION_B, "COMPLETED"),
        },
      },
      quickChat: {
        ...state.quickChat,
        unseenIdleByWorkspace: {
          [WORKSPACE_A]: { [SESSION_A]: true },
          [WORKSPACE_B]: { [SESSION_B]: true },
        },
      },
    }));

    expect(selectQuickChatActivity(store.getState(), WORKSPACE_A)).toBe("finished");
    expect(selectQuickChatActivity(store.getState(), WORKSPACE_B)).toBe("finished");
    expect(selectQuickChatActivity(store.getState(), "workspace-without-markers")).toBeNull();
  });

  it("does not show a finished state after all markers are seen", () => {
    const store = createAppStore();
    addSessions(store, [{ sessionId: SESSION_A, workspaceId: WORKSPACE_A }]);
    store.setState((state) => ({
      ...state,
      taskSessions: { items: { [SESSION_A]: session(SESSION_A, "IDLE") } },
    }));

    expect(selectQuickChatActivity(store.getState(), WORKSPACE_A)).toBeNull();
  });
});
