import { describe, expect, it } from "vitest";
import { sessionId, taskId, type Message, type MessageType } from "@/lib/types/http";
import { hasAgentActivityAfterNotice } from "./running-notice-activity";

const notice: Message = {
  id: "notice",
  session_id: sessionId("session-1"),
  task_id: taskId("task-1"),
  turn_id: "turn-1",
  author_type: "agent",
  type: "status",
  content: "Still waiting on Compact conversation.",
  created_at: "2026-09-30T12:00:00.123456Z",
  metadata: { action_visibility: "running" },
};

function activity(overrides: Partial<Message> = {}): Message {
  return {
    ...notice,
    id: "activity",
    type: "tool_call",
    metadata: { status: "complete" },
    content: "Conversation compacted",
    created_at: "2026-09-30T12:00:01Z",
    ...overrides,
  };
}

// @covers AC-AGENTS-AGENT-STALL-RECOVERY-001.6
describe("hasAgentActivityAfterNotice", () => {
  it("uses whole-turn evidence projected onto a paginated notice", () => {
    const resolved = { ...notice, metadata: { ...notice.metadata, running_notice_resolved: true } };
    expect(hasAgentActivityAfterNotice(undefined, resolved)).toBe(true);
    expect(hasAgentActivityAfterNotice([resolved], notice)).toBe(true);
  });

  it.each<MessageType>([
    "message",
    "content",
    "thinking",
    "tool_call",
    "tool_read",
    "tool_edit",
    "tool_execute",
    "tool_search",
    "agent_plan",
    "todo",
    "permission_request",
  ])("resolves a notice after an agent %s row", (type) => {
    expect(hasAgentActivityAfterNotice([notice, activity({ type })], notice)).toBe(true);
  });

  it("uses updates to tool rows created before the notice", () => {
    const tool = activity({
      created_at: "2026-09-30T11:59:00Z",
      updated_at: "2026-09-30T12:00:01Z",
    });
    expect(hasAgentActivityAfterNotice([tool, notice], notice)).toBe(true);
  });

  it("preserves timestamp precision below one millisecond", () => {
    expect(
      hasAgentActivityAfterNotice(
        [activity({ created_at: "2026-09-30T12:00:00.123456001Z" })],
        notice,
      ),
    ).toBe(true);
  });

  it("compares instants across timezone offsets", () => {
    expect(
      hasAgentActivityAfterNotice([activity({ created_at: "2026-09-30T13:00:01+01:00" })], notice),
    ).toBe(true);
  });

  it.each([
    ["queued user input", { author_type: "user" }],
    ["system status", { type: "status" }],
    ["system error", { type: "error" }],
    ["script execution", { type: "script_execution" }],
    ["another turn", { turn_id: "turn-2" }],
    ["missing turn identity", { turn_id: undefined }],
    ["another session", { session_id: sessionId("session-2") }],
    ["earlier activity", { created_at: "2026-09-30T11:59:00Z" }],
    ["equal timestamp", { created_at: notice.created_at }],
    ["invalid timestamp", { created_at: "invalid" }],
  ] satisfies [string, Partial<Message>][])("ignores %s", (_, overrides) => {
    expect(hasAgentActivityAfterNotice([activity(overrides)], notice)).toBe(false);
  });

  it("ignores the notice's own later update", () => {
    expect(
      hasAgentActivityAfterNotice([{ ...notice, updated_at: "2026-09-30T12:00:01Z" }], notice),
    ).toBe(false);
  });

  it("has no evidence when messages or notice identity are missing", () => {
    expect(hasAgentActivityAfterNotice(undefined, notice)).toBe(false);
    expect(hasAgentActivityAfterNotice([activity()], { ...notice, turn_id: undefined })).toBe(
      false,
    );
    expect(hasAgentActivityAfterNotice([activity()], { ...notice, created_at: "invalid" })).toBe(
      false,
    );
  });
});
