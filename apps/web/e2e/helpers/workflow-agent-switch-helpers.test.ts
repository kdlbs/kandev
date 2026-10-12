import { describe, expect, it, vi } from "vitest";
import type { Agent } from "../../lib/types/http-agents";

vi.mock("../fixtures/test-base", () => ({
  expect: { poll: vi.fn() },
}));

import { createWorkflowAgentProfiles } from "../tests/workflow/workflow-agent-switch-helpers";
import type { ApiClient } from "./api-client";

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

describe("createWorkflowAgentProfiles", () => {
  it("uses the E2E mock agent when a disabled agent is listed first", async () => {
    const profileTargets: string[] = [];
    const apiClient = {
      listAgents: async () => ({
        agents: [
          agent("disabled-agent-id", "Codex app-server"),
          agent("mock-agent-id", "mock-agent"),
        ],
        total: 2,
      }),
      createAgentProfile: async (agentId: string, name: string) => {
        profileTargets.push(agentId);
        return { id: name };
      },
    } as unknown as ApiClient;

    await createWorkflowAgentProfiles(apiClient);

    expect(profileTargets).toEqual(["mock-agent-id", "mock-agent-id"]);
  });
});
