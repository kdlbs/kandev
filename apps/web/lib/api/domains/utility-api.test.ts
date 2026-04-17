import { describe, it, expect } from "vitest";
import {
  mergeAgentsWithCapabilities,
  type InferenceAgent,
  type AgentCapabilities,
} from "./utility-api";

const CLAUDE_CODE = "claude-code";
const CLAUDE_CODE_DISPLAY = "Claude Code";

function agent(id: string, name: string, models: InferenceAgent["models"] = null): InferenceAgent {
  return { id, name, display_name: name, models };
}

function caps(
  agentType: string,
  models?: AgentCapabilities["models"],
  currentModelId?: string,
): AgentCapabilities {
  return {
    agent_type: agentType,
    status: "ok",
    models,
    current_model_id: currentModelId,
    last_checked_at: new Date().toISOString(),
  };
}

describe("mergeAgentsWithCapabilities", () => {
  it("populates models from capabilities", () => {
    const agents = [agent(CLAUDE_CODE, CLAUDE_CODE_DISPLAY)];
    const capabilities = [
      caps(
        CLAUDE_CODE,
        [
          { id: "opus", name: "Opus" },
          { id: "sonnet", name: "Sonnet" },
        ],
        "sonnet",
      ),
    ];

    const result = mergeAgentsWithCapabilities(agents, capabilities);

    expect(result).toHaveLength(1);
    expect(result[0].models).toEqual([
      { id: "opus", name: "Opus", description: "", is_default: false },
      { id: "sonnet", name: "Sonnet", description: "", is_default: true },
    ]);
  });

  it("returns agent unchanged if no matching capabilities", () => {
    const agents = [agent("unknown-agent", "Unknown")];
    const capabilities: AgentCapabilities[] = [];

    const result = mergeAgentsWithCapabilities(agents, capabilities);

    expect(result).toHaveLength(1);
    expect(result[0]).toEqual(agents[0]);
  });

  it("returns agent unchanged if capabilities have no models", () => {
    const agents = [agent(CLAUDE_CODE, CLAUDE_CODE_DISPLAY)];
    const capabilities = [caps(CLAUDE_CODE, undefined)];

    const result = mergeAgentsWithCapabilities(agents, capabilities);

    expect(result).toHaveLength(1);
    expect(result[0].models).toBeNull();
  });

  it("handles multiple agents with different capabilities", () => {
    const agents = [
      agent(CLAUDE_CODE, CLAUDE_CODE_DISPLAY),
      agent("codex", "Codex"),
      agent("opencode", "OpenCode"),
    ];
    const capabilities = [
      caps(CLAUDE_CODE, [{ id: "opus", name: "Opus" }], "opus"),
      caps("opencode", [{ id: "gpt4o", name: "GPT-4o" }]),
    ];

    const result = mergeAgentsWithCapabilities(agents, capabilities);

    expect(result).toHaveLength(3);
    expect(result[0].models).toEqual([
      { id: "opus", name: "Opus", description: "", is_default: true },
    ]);
    expect(result[1].models).toBeNull(); // no capabilities for codex
    expect(result[2].models).toEqual([
      { id: "gpt4o", name: "GPT-4o", description: "", is_default: false },
    ]);
  });

  it("preserves model description from capabilities", () => {
    const agents = [agent(CLAUDE_CODE, CLAUDE_CODE_DISPLAY)];
    const capabilities = [
      caps(CLAUDE_CODE, [{ id: "opus", name: "Opus", description: "Most capable model" }]),
    ];

    const result = mergeAgentsWithCapabilities(agents, capabilities);

    expect(result[0].models?.[0].description).toBe("Most capable model");
  });

  it("sets is_default based on current_model_id match", () => {
    const agents = [agent(CLAUDE_CODE, CLAUDE_CODE_DISPLAY)];
    const capabilities = [
      caps(
        CLAUDE_CODE,
        [
          { id: "opus", name: "Opus" },
          { id: "sonnet", name: "Sonnet" },
          { id: "haiku", name: "Haiku" },
        ],
        "haiku",
      ),
    ];

    const result = mergeAgentsWithCapabilities(agents, capabilities);

    expect(result[0].models).toEqual([
      { id: "opus", name: "Opus", description: "", is_default: false },
      { id: "sonnet", name: "Sonnet", description: "", is_default: false },
      { id: "haiku", name: "Haiku", description: "", is_default: true },
    ]);
  });
});
