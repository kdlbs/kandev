import { describe, expect, it } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import type { RenderItem } from "@/hooks/use-processed-messages";
import { hideSuccessfulStartupRows } from "./startup-rows";

function message(id: string, type: Message["type"], metadata?: Record<string, unknown>): Message {
  return {
    id,
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    author_type: "agent",
    content: "",
    type,
    created_at: "2026-09-28T00:00:00Z",
    metadata,
  } as Message;
}

function boot(id: string, status: string, exitCode?: number): Message {
  return message(id, "script_execution", {
    script_type: "agent_boot",
    agent_name: "Claude",
    status,
    exit_code: exitCode,
  });
}

const prepare: RenderItem = { type: "prepare_progress", id: "prepare-s1", sessionId: "s1" };
const answer: RenderItem = { type: "message", message: message("m1", "message") };

describe("hideSuccessfulStartupRows", () => {
  it("drops the environment and agent-start rows once the agent booted", () => {
    const items: RenderItem[] = [
      prepare,
      { type: "message", message: boot("b1", "exited", 0) },
      answer,
    ];

    expect(hideSuccessfulStartupRows(items)).toEqual([answer]);
  });

  it("drops a successful boot row inside a turn group and keeps the group's other messages", () => {
    const toolCall = message("tc1", "tool_call");
    const group: RenderItem = {
      type: "turn_group",
      id: "g1",
      turnId: "turn-1",
      messages: [boot("b1", "exited"), toolCall],
    };

    expect(hideSuccessfulStartupRows([prepare, group])).toEqual([
      { ...group, messages: [toolCall] },
    ]);
  });

  it("drops a turn group left empty by the filter", () => {
    const group: RenderItem = {
      type: "turn_group",
      id: "g1",
      turnId: "turn-1",
      messages: [boot("b1", "exited", 0)],
    };

    expect(hideSuccessfulStartupRows([prepare, group, answer])).toEqual([answer]);
  });

  it("keeps a failed agent start and the environment row so the failure stays visible", () => {
    const failed: RenderItem = { type: "message", message: boot("b1", "failed", 1) };
    const items: RenderItem[] = [prepare, failed, answer];

    expect(hideSuccessfulStartupRows(items)).toEqual(items);
  });

  it("keeps the environment row while the agent is still starting", () => {
    const starting: RenderItem = { type: "message", message: boot("b1", "starting") };
    const items: RenderItem[] = [prepare, starting];

    expect(hideSuccessfulStartupRows(items)).toEqual(items);
  });

  it("keeps script_execution rows that are not agent boots", () => {
    const setupScript: RenderItem = {
      type: "message",
      message: message("s1", "script_execution", {
        script_type: "setup",
        status: "exited",
        exit_code: 0,
      }),
    };

    expect(hideSuccessfulStartupRows([setupScript, answer])).toEqual([setupScript, answer]);
  });

  it("hides a later restart's preparation row but keeps its failed start row", () => {
    const first: RenderItem = { type: "message", message: boot("b1", "exited", 0) };
    const failed: RenderItem = { type: "message", message: boot("b2", "failed", 1) };
    const items: RenderItem[] = [first, answer, prepare, failed];

    expect(hideSuccessfulStartupRows(items)).toEqual([answer, failed]);
  });
});
