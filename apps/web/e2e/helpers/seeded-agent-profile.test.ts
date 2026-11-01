import { expect, it } from "vitest";
import type { ApiClient } from "./api-client";
import { seededAgentProfile } from "./seeded-agent-profile";

it("selects the seeded profile even when an earlier agent has no profiles", async () => {
  const seed = { id: "seed-profile", name: "Seed" };
  const agents = [
    { id: "empty", profiles: [] },
    { id: "seed-agent", profiles: [seed] },
  ];
  const api = { listAgents: async () => ({ agents }) } as unknown as ApiClient;

  expect(await seededAgentProfile(api, seed.id)).toEqual({ agent: agents[1], profile: seed });
});

it("rejects a seeded profile with more than one owner", async () => {
  const seed = { id: "seed-profile", name: "Seed" };
  const agents = [
    { id: "first", profiles: [seed] },
    { id: "second", profiles: [seed] },
  ];
  const api = { listAgents: async () => ({ agents }) } as unknown as ApiClient;

  await expect(seededAgentProfile(api, seed.id)).rejects.toThrow("exactly one owning agent");
});
