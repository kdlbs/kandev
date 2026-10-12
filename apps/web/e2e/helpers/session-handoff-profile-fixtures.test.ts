import { describe, expect, it } from "vitest";
import type { ApiClient } from "./api-client";
import { createHandoffProfiles } from "./session-handoff-profile-fixtures";

describe("handoff profile fixtures", () => {
  it("does not create a profile on an unrelated agent retained by an earlier test", async () => {
    const apiClient = {
      listAgents: async () => ({
        agents: [
          { id: "retained-agent", name: "unregistered-test-agent" },
          { id: "mock-owner", name: "mock-agent" },
          { id: "dynamic", name: "dynamic" },
        ],
      }),
      createAgentProfile: async (agentId: string, name: string) => {
        if (agentId !== "mock-owner") throw new Error("unknown agent: unregistered-test-agent");
        return { id: name, agentId };
      },
    } as unknown as ApiClient;

    await expect(createHandoffProfiles(apiClient)).resolves.toMatchObject({
      profileA: { agentId: "mock-owner" },
      profileB: { agentId: "mock-owner" },
    });
  });
});
