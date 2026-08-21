import { describe, expect, it } from "vitest";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type MessageType,
} from "@/lib/types/http";
import { buildGroupedRenderItems } from "./use-processed-messages";

function message(
  id: string,
  type: MessageType,
  metadata: Record<string, unknown>,
  content = "",
): Message {
  return {
    id,
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    author_type: "agent",
    content,
    type,
    metadata,
    created_at: "",
    turn_id: "turn-1",
  };
}

function toolActivity(id: string): Message {
  return message(
    id,
    "tool_execute",
    {
      status: "complete",
      normalized: { shell_exec: { command: "gh pr checks", output: { exit_code: 0 } } },
    },
    "gh pr checks",
  );
}

function richOutputCall(): Message {
  const toolName = "mcp__kandev__show_rich_output_kandev";
  return message(
    "rich",
    "tool_call",
    {
      title: toolName,
      status: "complete",
      normalized: { kind: "generic", generic: { name: "other" } },
    },
    toolName,
  );
}

describe("buildGroupedRenderItems rich-output hoisting", () => {
  it("keeps a rich presentation standalone and groups ordinary activity", () => {
    const items = buildGroupedRenderItems(
      [
        toolActivity("t1"),
        toolActivity("t2"),
        richOutputCall(),
        toolActivity("t3"),
        toolActivity("t4"),
      ],
      "s1",
      { canAnchorPrepareProgress: false },
    );

    expect(items.map((item) => item.type)).toEqual(["turn_group", "message", "turn_group"]);
    expect(items[1]).toMatchObject({ type: "message", message: { id: "rich" } });
  });
});
