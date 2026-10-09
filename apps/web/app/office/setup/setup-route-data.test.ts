import { beforeEach, describe, expect, it, vi } from "vitest";

import { getOnboardingState } from "@/lib/api/domains/office-api";
import { fetchUserSettings, listAgents } from "@/lib/api/domains/settings-api";
import { listWorkspaces } from "@/lib/api/domains/workspace-api";
import type { Agent, ListWorkspacesResponse, UserSettingsResponse } from "@/lib/types/http";
import { loadSetupRouteData } from "./setup-route-data";

vi.mock("@/lib/api/domains/office-api", () => ({
  getOnboardingState: vi.fn(),
}));

vi.mock("@/lib/api/domains/settings-api", () => ({
  fetchUserSettings: vi.fn(),
  listAgents: vi.fn(),
}));

vi.mock("@/lib/api/domains/workspace-api", () => ({
  listWorkspaces: vi.fn(),
}));

const mockGetOnboardingState = vi.mocked(getOnboardingState);
const mockFetchUserSettings = vi.mocked(fetchUserSettings);
const mockListAgents = vi.mocked(listAgents);
const mockListWorkspaces = vi.mocked(listWorkspaces);
const FIXTURE_TIMESTAMP = "2026-01-01T00:00:00Z";

beforeEach(() => {
  vi.resetAllMocks();
  mockGetOnboardingState.mockResolvedValue({
    completed: false,
    fsWorkspaces: [],
  });
  mockListAgents.mockResolvedValue({
    agents: [],
    total: 0,
  });
  mockFetchUserSettings.mockResolvedValue(emptyUserSettings());
  mockListWorkspaces.mockResolvedValue(workspaces([]));
});

describe("loadSetupRouteData", () => {
  it("keeps the newest-first selector default after a saved Settings reorder", async () => {
    const savedAgent = agent("a", "A", ["old", "new"]);
    savedAgent.profiles[0].createdAt = FIXTURE_TIMESTAMP;
    savedAgent.profiles[1].createdAt = "2026-02-01T00:00:00Z";
    savedAgent.profile_order_revision = 4;
    mockListAgents.mockResolvedValueOnce({ agents: [savedAgent], total: 1 });
    const data = await loadSetupRouteData("new");
    expect(data.kind).toBe("wizard");
    if (data.kind !== "wizard") throw new Error("Expected setup wizard");
    expect(data.props.agentProfiles.map((profile) => profile.id)).toEqual(["new", "old"]);
    expect(data.props.defaultAgentProfileId).toBe("new");
  });

  it("returns redirect when onboarding completed and mode is not new", async () => {
    mockGetOnboardingState.mockResolvedValueOnce({
      completed: true,
      fsWorkspaces: [],
    });

    await expect(loadSetupRouteData()).resolves.toEqual({
      kind: "redirect",
      href: "/office",
    });
  });

  it("returns wizard props when onboarding is not completed", async () => {
    mockGetOnboardingState.mockResolvedValueOnce({
      completed: false,
      fsWorkspaces: [{ name: "repo" }],
    });
    mockListAgents.mockResolvedValueOnce({
      agents: [agent("agent-a", "Agent A", ["profile-a"])],
      total: 1,
    });

    await expect(loadSetupRouteData()).resolves.toMatchObject({
      kind: "wizard",
      props: {
        fsWorkspaces: [{ name: "repo" }],
        mode: undefined,
        defaultAgentProfileId: "profile-a",
        suggestedWorkspaceName: "Default",
      },
    });
  });

  it("prefers the agent matching default_utility_agent_id", async () => {
    mockListAgents.mockResolvedValueOnce({
      agents: [
        agent("agent-a", "Agent A", ["profile-a"]),
        agent("agent-b", "Agent B", ["profile-b"]),
      ],
      total: 2,
    });
    mockFetchUserSettings.mockResolvedValueOnce(
      userSettings({ default_utility_agent_id: "agent-b" }),
    );

    const data = await loadSetupRouteData("new");

    expect(data).toMatchObject({
      kind: "wizard",
      props: {
        mode: "new",
        defaultAgentProfileId: "profile-b",
      },
    });
  });

  it("suggests Default 2 when Default is already used by an office workspace", async () => {
    mockListWorkspaces.mockResolvedValueOnce(
      workspaces([
        { id: "kanban-ws", name: "Default" },
        { id: "office-ws", name: "Default", office_workflow_id: "office-flow" },
      ]),
    );

    const data = await loadSetupRouteData("new");

    expect(data).toMatchObject({
      kind: "wizard",
      props: {
        suggestedWorkspaceName: "Default 2",
      },
    });
  });

  it("falls back to an empty wizard when API calls fail", async () => {
    mockGetOnboardingState.mockRejectedValueOnce(new Error("offline"));
    mockListAgents.mockRejectedValueOnce(new Error("offline"));
    mockFetchUserSettings.mockRejectedValueOnce(new Error("offline"));
    mockListWorkspaces.mockRejectedValueOnce(new Error("offline"));

    await expect(loadSetupRouteData("new")).resolves.toEqual({
      kind: "wizard",
      props: {
        agentProfiles: [],
        fsWorkspaces: [],
        mode: "new",
        defaultAgentProfileId: "",
        suggestedWorkspaceName: "Default",
      },
    });
  });
});

function agent(id: string, name: string, profileIds: string[]): Agent {
  return {
    id,
    name,
    supports_mcp: false,
    created_at: FIXTURE_TIMESTAMP,
    updated_at: FIXTURE_TIMESTAMP,
    profiles: profileIds.map((profileId) => ({
      id: profileId,
      name: `${name} Profile`,
      agentDisplayName: name,
    })),
  } as unknown as Agent;
}

function emptyUserSettings(): UserSettingsResponse {
  return userSettings({});
}

function userSettings(settings: Partial<UserSettingsResponse["settings"]>): UserSettingsResponse {
  return {
    settings: {
      user_id: "user-1",
      workspace_id: "" as unknown as UserSettingsResponse["settings"]["workspace_id"],
      workflow_filter_id: "",
      repository_ids: [],
      updated_at: FIXTURE_TIMESTAMP,
      ...settings,
    },
  };
}

function workspaces(items: Array<Record<string, unknown>>): ListWorkspacesResponse {
  return { workspaces: items, total: items.length } as ListWorkspacesResponse;
}
