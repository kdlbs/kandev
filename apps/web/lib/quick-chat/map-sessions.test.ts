import { describe, it, expect } from "vitest";
import { toQuickChatSessions } from "./map-sessions";

describe("toQuickChatSessions", () => {
  it("maps the response onto store tabs", () => {
    expect(
      toQuickChatSessions([
        {
          session_id: "session-1",
          task_id: "task-1",
          workspace_id: "ws-1",
          kind: "chat",
          name: "Chat 1",
          agent_profile_id: "agent-1",
        },
      ]),
    ).toEqual([
      {
        kind: "chat",
        sessionId: "session-1",
        taskId: "task-1",
        workspaceId: "ws-1",
        name: "Chat 1",
        agentProfileId: "agent-1",
      },
    ]);
  });

  // Boot hydration and the runtime resync share this mapper. When they had
  // their own copies, the boot one dropped task_id, so closing a boot-loaded
  // tab skipped the backend delete and orphaned its ephemeral task.
  it("always carries taskId, which the close flow needs to delete the task", () => {
    const [session] = toQuickChatSessions([
      { session_id: "s", task_id: "t", workspace_id: "w", kind: "config" },
    ]);

    expect(session.taskId).toBe("t");
    expect(session.kind).toBe("config");
    expect(session.name).toBeUndefined();
  });

  it("returns an empty list unchanged", () => {
    expect(toQuickChatSessions([])).toEqual([]);
  });
});
