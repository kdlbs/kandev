import { describe, it, expect } from "vitest";
import type { Message, MessageType } from "@/lib/types/http";
import { deduplicateAgentBootResumes, isAgentBootResumeMessage } from "./use-processed-messages";

function makeMessage(
  id: string,
  type: MessageType,
  metadata?: Record<string, unknown>,
  content = "",
): Message {
  return {
    id,
    session_id: "s1",
    task_id: "t1",
    author_type: "agent",
    content,
    type,
    metadata,
    created_at: "",
  };
}

function bootStarted(id: string): Message {
  return makeMessage(id, "script_execution", {
    script_type: "agent_boot",
    agent_name: "Mock",
    is_resuming: false,
    status: "exited",
  });
}

function bootResumed(id: string): Message {
  return makeMessage(id, "script_execution", {
    script_type: "agent_boot",
    agent_name: "Mock",
    is_resuming: true,
    status: "exited",
  });
}

describe("isAgentBootResumeMessage", () => {
  it("returns true for script_execution agent_boot with is_resuming=true", () => {
    expect(isAgentBootResumeMessage(bootResumed("r1"))).toBe(true);
  });

  it("returns false for a Started (non-resuming) agent_boot", () => {
    expect(isAgentBootResumeMessage(bootStarted("s1"))).toBe(false);
  });

  it("returns false for a setup/cleanup script", () => {
    const setup = makeMessage("x", "script_execution", {
      script_type: "setup",
      is_resuming: true,
    });
    expect(isAgentBootResumeMessage(setup)).toBe(false);
  });

  it("returns false for unrelated message types", () => {
    expect(isAgentBootResumeMessage(makeMessage("m1", "message"))).toBe(false);
  });

  it("returns false when metadata is missing", () => {
    const msg = makeMessage("x", "script_execution");
    expect(isAgentBootResumeMessage(msg)).toBe(false);
  });
});

describe("deduplicateAgentBootResumes", () => {
  it("returns an empty list unchanged", () => {
    expect(deduplicateAgentBootResumes([])).toEqual([]);
  });

  it("returns the list unchanged when there are no resume messages", () => {
    const messages = [bootStarted("s1"), makeMessage("m1", "message", undefined, "hi")];
    expect(deduplicateAgentBootResumes(messages)).toEqual(messages);
  });

  it("returns the list unchanged when there is exactly one resume message", () => {
    const messages = [
      bootStarted("s1"),
      makeMessage("m1", "message", undefined, "hi"),
      bootResumed("r1"),
    ];
    expect(deduplicateAgentBootResumes(messages)).toEqual(messages);
  });

  it("keeps only the last resume message when multiple exist", () => {
    const messages = [bootResumed("r1"), bootResumed("r2"), bootResumed("r3")];
    const result = deduplicateAgentBootResumes(messages);
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe("r3");
  });

  it("preserves Started and non-boot messages while deduping resumes", () => {
    const started = bootStarted("s1");
    const userMsg = makeMessage("m1", "message", undefined, "hello");
    const r1 = bootResumed("r1");
    const r2 = bootResumed("r2");
    const agentMsg = makeMessage("m2", "message", undefined, "reply");
    const r3 = bootResumed("r3");

    const result = deduplicateAgentBootResumes([started, userMsg, r1, r2, agentMsg, r3]);

    expect(result.map((m) => m.id)).toEqual(["s1", "m1", "m2", "r3"]);
  });

  it("does not touch setup/cleanup script executions", () => {
    const setup = makeMessage("x", "script_execution", {
      script_type: "setup",
      status: "exited",
    });
    const messages = [setup, bootResumed("r1"), bootResumed("r2")];
    const result = deduplicateAgentBootResumes(messages);
    expect(result.map((m) => m.id)).toEqual(["x", "r2"]);
  });
});
