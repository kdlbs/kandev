import { describe, expect, it } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import {
  isLaunchErrorSurfaceMessage,
  isMatchingTaskLaunchError,
  isRichOutputMessage,
  kandevToolStemOf,
  type ToolCallMetadata,
} from "./types";

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

  it("uses the normalized generic tool identity as a fallback", () => {
    const message = toolCall("");
    const metadata = message.metadata as ToolCallMetadata;
    metadata.normalized!.generic!.name = "show_rich_output_kandev";

    expect(kandevToolStemOf(message)).toBe("show_rich_output");
  });
});

describe("isRichOutputMessage", () => {
  it("matches only the exact Kandev rich-output tool call", () => {
    expect(isRichOutputMessage(toolCall("mcp__kandev__show_rich_output_kandev"))).toBe(true);
    expect(isRichOutputMessage(toolCall("kandev/show_rich_output_kandev"))).toBe(true);
    expect(isRichOutputMessage(toolCall("mcp__kandev__show_walkthrough_kandev"))).toBe(false);
    expect(isRichOutputMessage(toolCall("other/show_rich_output_kandev"))).toBe(false);
    expect(isRichOutputMessage(toolCall("mcp__other__show_rich_output"))).toBe(false);
  });

  it("treats a namespaced normalized identity as authoritative", () => {
    const message = toolCall("mcp.kandev.show_rich_output_kandev");
    const metadata = message.metadata as ToolCallMetadata;
    metadata.normalized!.generic!.name = "other/show_rich_output_kandev";

    expect(isRichOutputMessage(message)).toBe(false);
  });

  it("rejects non-tool messages", () => {
    expect(isRichOutputMessage({ ...toolCall("show_rich_output_kandev"), type: "message" })).toBe(
      false,
    );
  });
});

describe("task launch error presentation predicates", () => {
  it("matches a summary error only by session and stamp", () => {
    const activeError = { session_id: "session-1", stamp: "stamp-1" };

    expect(
      isMatchingTaskLaunchError(activeError, {
        sessionId: "session-1",
        errorStamp: "stamp-1",
      }),
    ).toBe(true);
    expect(
      isMatchingTaskLaunchError(activeError, {
        sessionId: "session-2",
        errorStamp: "stamp-1",
      }),
    ).toBe(false);
    expect(
      isMatchingTaskLaunchError(activeError, {
        sessionId: "session-1",
        errorStamp: "old-stamp",
      }),
    ).toBe(false);
  });

  it("matches task-level errors only when the candidate has no session", () => {
    const activeError = { stamp: "stamp-1" };

    expect(isMatchingTaskLaunchError(activeError, { errorStamp: "stamp-1" })).toBe(true);
    expect(
      isMatchingTaskLaunchError(activeError, {
        sessionId: "session-1",
        errorStamp: "stamp-1",
      }),
    ).toBe(false);
  });

  it.each([
    { metadata: { empty_turn: true }, expected: true },
    { metadata: { failure_kind: "missing_pr_branch" }, expected: true },
    { metadata: { failure_kind: "provider_quota_limited" }, expected: false },
    { metadata: undefined, expected: false },
  ])("classifies launch-only message noise", ({ metadata, expected }) => {
    expect(isLaunchErrorSurfaceMessage({ id: "message-1", metadata } as never)).toBe(expected);
  });
});
