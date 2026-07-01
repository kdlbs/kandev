import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import { listAgentProfiles } from "./office-api";

type FetchInput = Parameters<typeof fetch>[0];
type FetchInit = Parameters<typeof fetch>[1];

const fetchSpy = vi.fn<[FetchInput, FetchInit?], Promise<Response>>();

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function mockAgentList(agents: unknown[]) {
  fetchSpy.mockResolvedValue(
    new Response(JSON.stringify({ agents }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }),
  );
}

function rawAgent(overrides: Record<string, unknown> = {}) {
  return {
    id: "agent-1",
    workspace_id: "workspace-1",
    name: "CEO",
    role: "ceo",
    status: "idle",
    created_at: "2026-05-04T00:00:00Z",
    updated_at: "2026-05-04T00:00:00Z",
    ...overrides,
  };
}

describe("office agent normalization", () => {
  it("preserves absent skill fields for legacy default fallback", async () => {
    mockAgentList([rawAgent()]);

    const response = await listAgentProfiles("workspace-1");

    expect(response.agents[0]?.skillIds).toBeUndefined();
    expect(response.agents[0]?.desiredSkills).toBeUndefined();
  });

  it("preserves explicitly cleared skill fields", async () => {
    mockAgentList([rawAgent({ skill_ids: "[]", desired_skills: "[]" })]);

    const response = await listAgentProfiles("workspace-1");

    expect(response.agents[0]?.skillIds).toEqual([]);
    expect(response.agents[0]?.desiredSkills).toEqual([]);
  });
});
