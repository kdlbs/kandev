import { beforeEach, describe, expect, it, vi } from "vitest";

import type { Agent, ListAgentsResponse, ListWorkspacesResponse } from "@/lib/types/http";
import { loadSettingsInitialState } from "./settings-routes";

const mocks = vi.hoisted(() => ({
  fetchUserSettings: vi.fn(),
  listAgentDiscovery: vi.fn(),
  listAgents: vi.fn(),
  listAvailableAgents: vi.fn(),
  listExecutors: vi.fn(),
  listWorkspaces: vi.fn(),
}));

vi.mock("@/lib/api/domains/settings-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/settings-api")>()),
  fetchUserSettings: mocks.fetchUserSettings,
  listAgentDiscovery: mocks.listAgentDiscovery,
  listAgents: mocks.listAgents,
  listAvailableAgents: mocks.listAvailableAgents,
  listExecutors: mocks.listExecutors,
}));

vi.mock("@/lib/api/domains/workspace-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/workspace-api")>()),
  listWorkspaces: mocks.listWorkspaces,
}));

const TEST_AGENT = {
  id: "agent-1",
  name: "Agent",
  supports_mcp: false,
  profiles: [],
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
} as unknown as Agent;

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function agentResponse(agents: Agent[]): ListAgentsResponse {
  return { agents, total: agents.length };
}

function emptyWorkspaceResponse(): ListWorkspacesResponse {
  return { workspaces: [], total: 0 };
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.fetchUserSettings.mockRejectedValue(new Error("settings unavailable"));
  mocks.listAgentDiscovery.mockResolvedValue({ agents: [], total: 0 });
  mocks.listAvailableAgents.mockResolvedValue({ agents: [], tools: [], total: 0 });
  mocks.listExecutors.mockResolvedValue({ executors: [], total: 0 });
  mocks.listWorkspaces.mockResolvedValue(emptyWorkspaceResponse());
  mocks.listAgents.mockResolvedValue(agentResponse([]));
});

describe("loadSettingsInitialState", () => {
  it("retries the full snapshot when a profile event races the request", async () => {
    const firstAgentsRead = deferred<ListAgentsResponse>();
    mocks.listAgents
      .mockImplementationOnce(() => firstAgentsRead.promise)
      .mockResolvedValueOnce(agentResponse([TEST_AGENT]));
    let version = 0;

    const initialStatePromise = loadSettingsInitialState(() => version);
    version = 1;
    firstAgentsRead.resolve(agentResponse([]));

    const initialState = await initialStatePromise;

    expect(mocks.listAgents).toHaveBeenCalledTimes(2);
    expect(initialState.agentProfiles?.version).toBe(1);
    expect(initialState.settingsAgents?.items).toEqual([TEST_AGENT]);
  });

  it("uses the current generation when the request starts after a profile event", async () => {
    mocks.listAgents.mockResolvedValue(agentResponse([TEST_AGENT]));

    const initialState = await loadSettingsInitialState(() => 2);

    expect(initialState.agentProfiles?.version).toBe(2);
    expect(initialState.settingsAgents?.items).toEqual([TEST_AGENT]);
  });
});
