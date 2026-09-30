import { describe, expect, it } from "vitest";
import type { Agent } from "../../lib/types/http-agents";
import { getMockAgent } from "./agent-fixtures";

function agent(id: string, name = id): Agent {
  return {
    id,
    name,
    supports_mcp: false,
    profiles: [],
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

describe("getMockAgent", () => {
  it("selects mock-agent when a feature-disabled agent is listed first", () => {
    const mockAgent = agent("mock-agent-id", "mock-agent");
    expect(getMockAgent([agent("disabled-agent-id", "Codex app-server"), mockAgent])).toBe(
      mockAgent,
    );
  });

  it("fails clearly when the mock-agent fixture is unavailable", () => {
    expect(() => getMockAgent([agent("disabled-agent-id", "Codex app-server")])).toThrow(
      "mock-agent unavailable in test fixtures",
    );
  });
});
