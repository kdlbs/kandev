import { describe, expect, it, vi } from "vitest";
import type { AppState } from "@/lib/state/store";
import { registerTasksHandlers } from "./tasks";
import { makeDeletedMessage, makeStore } from "./tasks.test-helpers";

vi.mock("@/lib/recent-tasks", () => ({ removeRecentTask: vi.fn() }));

type Handlers = ReturnType<typeof registerTasksHandlers>;
type UpsertAction = "task.created" | "task.updated";

/** Dispatches an upsert event without narrowing the handler union to `never`. */
function dispatchUpsert(
  handlers: Handlers,
  action: UpsertAction,
  payload: Record<string, unknown>,
) {
  const handler = handlers[action] as ((message: unknown) => void) | undefined;
  handler?.({ id: "msg-1", type: "notification", action, payload });
}

const QUICK_CHAT_PAYLOAD = {
  task_id: "qc-task",
  workspace_id: "ws-1",
  workflow_id: "",
  workflow_step_id: "",
  title: "Claude - Chat 1",
  is_ephemeral: true,
  primary_session_id: "qc-session",
  metadata: { agent_profile_id: "agent-1" },
};

/**
 * Quick chats are ephemeral tasks, which the kanban handlers deliberately skip.
 * These pin that they are still routed into the shared tab strip — without it a
 * quick chat only ever exists on the device that created it.
 */
describe("task lifecycle events drive the quick-chat tab strip", () => {
  it.each(["task.created", "task.updated"] as const)(
    "%s adds a quick chat started on another device",
    (action) => {
      const store = makeStore();
      const handlers = registerTasksHandlers(store);

      dispatchUpsert(handlers, action, QUICK_CHAT_PAYLOAD);

      expect(store.getState().upsertQuickChatSessionFromEvent).toHaveBeenCalledWith(
        expect.objectContaining({ sessionId: "qc-session", taskId: "qc-task" }),
      );
    },
  );

  it("keeps quick chats off the kanban board", () => {
    const store = makeStore();
    const handlers = registerTasksHandlers(store);

    dispatchUpsert(handlers, "task.created", QUICK_CHAT_PAYLOAD);

    expect(store.getState().kanban.tasks).toHaveLength(0);
  });

  it("task.deleted drops the tab for a quick chat closed elsewhere", () => {
    const store = makeStore({ environmentIdBySessionId: {} } as unknown as Partial<AppState>);
    const handlers = registerTasksHandlers(store);

    handlers["task.deleted"]?.(makeDeletedMessage({ task_id: "qc-task" }));

    expect(store.getState().removeQuickChatSessionsForTask).toHaveBeenCalledWith("qc-task");
  });
});
