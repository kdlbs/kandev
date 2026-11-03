import { describe, expect, it } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { buildGroupedRenderItems } from "./use-processed-messages";

function message(id: string, type: Message["type"], metadata?: Record<string, unknown>): Message {
  return {
    id,
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    author_type: "agent",
    content: id,
    type,
    metadata,
    created_at: "",
    turn_id: "turn-1",
  };
}

function tool(id: string): Message {
  return message(id, "tool_execute", {
    status: "complete",
    normalized: { shell_exec: { command: "run" } },
  });
}

describe("buildGroupedRenderItems context compaction placement", () => {
  it("keeps compaction rows at the top level between surrounding tool groups", () => {
    const compaction = message("compact-1", "tool_call", {
      status: "complete",
      normalized: {
        kind: "context_compaction",
        context_compaction: { compaction_id: "compact-1", status: "completed" },
      },
    });
    const items = buildGroupedRenderItems(
      [tool("t1"), tool("t2"), compaction, tool("t3"), tool("t4")],
      "s1",
      { canAnchorPrepareProgress: false },
    );

    expect(items.map((item) => item.type)).toEqual(["turn_group", "message", "turn_group"]);
    expect(items[1]).toMatchObject({ type: "message", message: { id: "compact-1" } });
  });
});
