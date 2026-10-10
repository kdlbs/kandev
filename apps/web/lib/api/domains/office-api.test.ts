import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import { updateAgentProfile } from "./office-api";

const OFFICE_AGENT_ID = "office-agent";
const DYNAMIC_PROFILE_ID = "dynamic-profile";
const OFFICE_UPDATE_PATH = `http://api.test/api/v1/office/agents/${OFFICE_AGENT_ID}`;
const originalFetch = global.fetch;
const fetchSpy = vi.fn<typeof fetch>();

beforeEach(() => {
  fetchSpy.mockReset();
  fetchSpy.mockResolvedValue(
    new Response(
      JSON.stringify({
        agent: {
          id: OFFICE_AGENT_ID,
          workspace_id: "workspace-1",
          name: "CEO",
          role: "ceo",
          status: "idle",
          execution_agent_profile_id: DYNAMIC_PROFILE_ID,
          created_at: "2026-09-28T00:00:00Z",
          updated_at: "2026-09-28T00:00:00Z",
        },
      }),
      { status: 200, headers: { "Content-Type": "application/json" } },
    ),
  );
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => {
  global.fetch = originalFetch;
  vi.restoreAllMocks();
});

describe("Office agent execution profile binding", () => {
  it("sends the selected source profile and normalizes the saved binding", async () => {
    const saved = await updateAgentProfile(OFFICE_AGENT_ID, {
      executionAgentProfileId: DYNAMIC_PROFILE_ID,
    });

    const [input, init] = fetchSpy.mock.calls[0] ?? [];
    expect(String(input)).toBe(OFFICE_UPDATE_PATH);
    expect(init?.method).toBe("PATCH");
    expect(JSON.parse(init?.body as string)).toEqual({
      agent_profile_id: DYNAMIC_PROFILE_ID,
    });
    expect(saved.executionAgentProfileId).toBe(DYNAMIC_PROFILE_ID);
  });

  it("sends an empty binding when the selection is cleared", async () => {
    await updateAgentProfile(OFFICE_AGENT_ID, { executionAgentProfileId: "" });

    const [, init] = fetchSpy.mock.calls[0] ?? [];
    expect(JSON.parse(init?.body as string)).toEqual({ agent_profile_id: "" });
  });
});
