import { describe, expect, it } from "vitest";
import { subagentMetaChips } from "./subagent-meta";
import type { SubagentTaskPayload } from "@/components/task/chat/types";

describe("subagentMetaChips", () => {
  it("returns empty list for undefined payload", () => {
    expect(subagentMetaChips(undefined)).toEqual([]);
  });

  it("returns empty list when no metric fields are present", () => {
    const payload: SubagentTaskPayload = { description: "do work", subagent_type: "general" };
    expect(subagentMetaChips(payload)).toEqual([]);
  });

  it("formats a full Claude payload", () => {
    const payload: SubagentTaskPayload = {
      subagent_type: "general-purpose",
      agent_id: "agent_0123456789abcdef",
      duration_ms: 2200,
      total_tokens: 9987,
      tool_use_count: 0,
      status: "complete",
    };
    expect(subagentMetaChips(payload)).toEqual([
      { label: "duration", value: "2.2s" },
      { label: "tokens", value: "9,987 tokens" },
      { label: "tools", value: "0 tools" },
    ]);
  });

  it("formats an OpenCode payload", () => {
    const payload: SubagentTaskPayload = {
      subagent_type: "task",
      model: "opencode/big-pickle",
      child_session_id: "sess_abcdef0123456789",
    };
    expect(subagentMetaChips(payload)).toEqual([
      { label: "model", value: "opencode/big-pickle" },
      { label: "session", value: "sess_abcdef0…" },
    ]);
  });

  it("formats a Cursor payload (duration only)", () => {
    const payload: SubagentTaskPayload = { subagent_type: "task", duration_ms: 850 };
    expect(subagentMetaChips(payload)).toEqual([{ label: "duration", value: "850ms" }]);
  });

  it("singularizes a single tool use", () => {
    expect(subagentMetaChips({ tool_use_count: 1 })).toEqual([{ label: "tools", value: "1 tool" }]);
  });

  it("skips zero/negative duration and zero tokens but keeps zero tools", () => {
    const payload: SubagentTaskPayload = { duration_ms: 0, total_tokens: 0, tool_use_count: 0 };
    expect(subagentMetaChips(payload)).toEqual([{ label: "tools", value: "0 tools" }]);
  });

  it("does not truncate short session ids", () => {
    expect(subagentMetaChips({ child_session_id: "short" })).toEqual([
      { label: "session", value: "short" },
    ]);
  });
});
