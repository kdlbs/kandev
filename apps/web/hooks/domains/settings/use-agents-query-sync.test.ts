import { describe, expect, it } from "vitest";
import type { Agent, AgentProfile } from "@/lib/types/http";
import { upsertProfileInAgents } from "./use-agents-query-sync";

const profileA = {
  id: "profile-a",
  agentId: "agent-a",
  agentDisplayName: "Codex",
  name: "Default",
} as AgentProfile;
const agentA = {
  id: "agent-a",
  name: "codex",
  profiles: [profileA],
} as Agent;

describe("agent query sync helpers", () => {
  it("upserts a profile under its owning agent", () => {
    const profileB = { ...profileA, id: "profile-b", name: "Review" } as AgentProfile;

    expect(upsertProfileInAgents([agentA], profileB)).toEqual([
      expect.objectContaining({ profiles: [profileA, profileB] }),
    ]);
    expect(
      upsertProfileInAgents([agentA], { ...profileA, name: "Renamed" } as AgentProfile),
    ).toEqual([
      expect.objectContaining({
        profiles: [expect.objectContaining({ id: "profile-a", name: "Renamed" })],
      }),
    ]);
  });

  it("leaves the list unchanged when the owner is not in cache", () => {
    const profile = { ...profileA, agentId: "missing-agent" } as AgentProfile;
    const agents = [agentA];
    const result = upsertProfileInAgents(agents, profile);
    expect(result).toBe(agents);
  });
});
