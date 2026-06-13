import { describe, expect, it } from "vitest";
import type { Message, Turn } from "@/lib/types/http";
import { buildMessageDebugEntries, hasMessageDebugMetadata } from "./message-debug-metadata";

const TEST_TIMESTAMP = "2026-06-13T19:00:00Z";

function message(overrides: Partial<Message> = {}): Message {
  return {
    id: "m1",
    session_id: "s1",
    task_id: "t1",
    turn_id: "turn1",
    author_type: "agent",
    content: "hello",
    type: "agent",
    created_at: TEST_TIMESTAMP,
    ...overrides,
  } as Message;
}

function turn(overrides: Partial<Turn> = {}): Turn {
  return {
    id: "turn1",
    session_id: "s1",
    task_id: "t1",
    started_at: TEST_TIMESTAMP,
    created_at: TEST_TIMESTAMP,
    updated_at: TEST_TIMESTAMP,
    ...overrides,
  } as Turn;
}

describe("message debug metadata", () => {
  it("hides the debug affordance when message and turn metadata are empty", () => {
    expect(hasMessageDebugMetadata(message(), turn())).toBe(false);
  });

  it("shows the debug affordance when turn metadata exists", () => {
    expect(
      hasMessageDebugMetadata(
        message(),
        turn({ metadata: { model: "gpt-5.5", prompt_usage: { total_tokens: 42 } } }),
      ),
    ).toBe(true);
  });

  it("builds a compact summary while preserving raw metadata", () => {
    const entries = buildMessageDebugEntries(
      message({ metadata: { tool_call_id: "tool1" } }),
      turn({
        metadata: {
          model: "gpt-5.5",
          agent_type: "codex-acp",
          prompt_usage: { total_tokens: 42, provider_reported_cost_subcents: 123 },
        },
      }),
      { usageMultiplier: "3x" },
    );

    expect(entries.model).toBe("gpt-5.5");
    expect(entries.usage_multiplier).toBe("3x");
    expect(entries.agent_type).toBe("codex-acp");
    expect(entries.prompt_usage).toEqual({
      total_tokens: 42,
      provider_reported_cost_subcents: 123,
    });
    expect(entries.message_metadata).toEqual({ tool_call_id: "tool1" });
  });
});
