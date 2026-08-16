import { describe, expect, it } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { isRichOutputMessage, kandevToolStemOf } from "./types";

function toolCall(rawName: string, field: "tool_name" | "title" | "content" = "title"): Message {
  const metadata: Record<string, unknown> = {
    status: "complete",
    normalized: { kind: "generic", generic: { name: "other" } },
  };
  if (field !== "content") metadata[field] = rawName;
  return {
    id: "message-1",
    session_id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    content: field === "content" ? rawName : "",
    type: "tool_call",
    metadata,
    created_at: "2026-08-14T12:00:00Z",
  };
}

describe("kandevToolStemOf", () => {
  it.each(["tool_name", "title", "content"] as const)("reads the %s field", (field) => {
    expect(kandevToolStemOf(toolCall("mcp__kandev__show_rich_output_kandev", field))).toBe(
      "show_rich_output",
    );
  });

  it("ignores a generic category and keeps scanning", () => {
    expect(kandevToolStemOf(toolCall("mcp__kandev__show_rich_output_kandev"))).toBe(
      "show_rich_output",
    );
  });
});

describe("isRichOutputMessage", () => {
  it("matches only the exact Kandev rich-output tool call", () => {
    expect(isRichOutputMessage(toolCall("mcp__kandev__show_rich_output_kandev"))).toBe(true);
    expect(isRichOutputMessage(toolCall("kandev/show_rich_output_kandev"))).toBe(true);
    expect(isRichOutputMessage(toolCall("mcp__kandev__show_walkthrough_kandev"))).toBe(false);
    expect(isRichOutputMessage(toolCall("mcp__other__show_rich_output"))).toBe(false);
  });

  it("rejects non-tool messages", () => {
    expect(isRichOutputMessage({ ...toolCall("show_rich_output_kandev"), type: "message" })).toBe(
      false,
    );
  });
});
