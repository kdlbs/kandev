import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { selectQuickChatHasUnseenIdle } from "@/lib/state/slices/ui/quick-chat-unseen-selectors";
import type { TaskSession } from "@/lib/types/http";
import type { TaskSessionStateChangedPayload } from "@/lib/types/backend";
import { registerTaskSessionHandlers } from "./agent-session";

const SESSION_ID = "session-1";
const TASK_ID = "task-1";
const WORKSPACE_ID = "workspace-1";
const RUNNING_AT = "2099-01-01T00:00:00Z";
const SETTLED_AT = "2099-01-01T00:01:00Z";

const ACTIVE_STATES = ["STARTING", "RUNNING"] as const;
const SETTLED_STATES = ["IDLE", "WAITING_FOR_INPUT", "COMPLETED", "FAILED", "CANCELLED"] as const;
function taskSession(state: TaskSession["state"], updatedAt: string): TaskSession {
  // The WebSocket handler only requires the stable state-transition fields.
  return {
    id: SESSION_ID,
    task_id: TASK_ID,
    state,
    updated_at: updatedAt,
  } as unknown as TaskSession;
}

function stateChange(
  newState: TaskSession["state"],
  updatedAt: string | undefined,
): TaskSessionStateChangedPayload {
  return {
    task_id: TASK_ID,
    session_id: SESSION_ID,
    new_state: newState,
    updated_at: updatedAt,
  };
}

function handleStateChange(
  store: ReturnType<typeof createAppStore>,
  payload: TaskSessionStateChangedPayload,
): void {
  registerTaskSessionHandlers(store)["session.state_changed"]!({
    id: "event-1",
    type: "notification",
    action: "session.state_changed",
    payload,
  });
}

function storeWithQuickChat(state: TaskSession["state"] = "RUNNING") {
  const store = createAppStore();
  store.getState().addQuickChatSession(SESSION_ID, WORKSPACE_ID);
  store.getState().setTaskSession(taskSession(state, RUNNING_AT));
  return store;
}

describe("quick-chat session state-change handler", () => {
  for (const previousState of ACTIVE_STATES) {
    it.each(SETTLED_STATES)(
      `marks a closed ${previousState} quick chat when it settles to %s`,
      (newState) => {
        const store = storeWithQuickChat(previousState);

        handleStateChange(store, stateChange(newState, SETTLED_AT));

        expect(selectQuickChatHasUnseenIdle(store.getState(), WORKSPACE_ID)).toBe(true);
      },
    );
  }

  it("does not mark reverse, dialog-open, replay, or timestamp-less transitions", () => {
    const reverse = storeWithQuickChat("IDLE");
    handleStateChange(reverse, stateChange("FAILED", SETTLED_AT));
    expect(selectQuickChatHasUnseenIdle(reverse.getState(), WORKSPACE_ID)).toBe(false);

    const open = storeWithQuickChat();
    open.getState().openQuickChat(SESSION_ID, WORKSPACE_ID);
    handleStateChange(open, stateChange("IDLE", SETTLED_AT));
    open.getState().closeQuickChat();
    handleStateChange(open, stateChange("IDLE", SETTLED_AT));
    expect(selectQuickChatHasUnseenIdle(open.getState(), WORKSPACE_ID)).toBe(false);

    const missingTimestamp = storeWithQuickChat();
    handleStateChange(missingTimestamp, stateChange("IDLE", undefined));
    expect(selectQuickChatHasUnseenIdle(missingTimestamp.getState(), WORKSPACE_ID)).toBe(false);
  });
});
