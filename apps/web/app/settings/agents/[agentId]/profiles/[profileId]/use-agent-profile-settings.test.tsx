import { act, render, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { listAgents } from "@/lib/api";
import type { Agent, ListAgentsResponse } from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";
import { useAgentProfileSettings } from "./use-agent-profile-settings";
import { registerAgentsHandlers } from "@/lib/ws/handlers/agents";
import type { BackendMessageMap } from "@/lib/types/backend";

const AGENT_ID = "agent";
const AGENT_NAME = "mock-agent";
const PROFILE_IDS = { created: "created", deleted: "deleted", live: "live" };
const MEMBERSHIP_EVENT_TIME = "2026-02-01T00:00:00Z";

vi.mock("@/lib/api", async (importOriginal) => {
  const actual = (await importOriginal()) as Record<string, unknown>;
  return { ...actual, listAgents: vi.fn() };
});

function option(id: string) {
  return { id, label: id, agent_id: AGENT_ID, agent_name: AGENT_NAME, cli_passthrough: false };
}

const snapshot = (profileIds: string[]): ListAgentsResponse => ({
  agents: [
    {
      id: AGENT_ID,
      name: AGENT_NAME,
      profiles: profileIds.map((id) => ({ id, name: id })),
    } as Agent,
  ],
  total: profileIds.length,
});

describe("useAgentProfileSettings list snapshot guard", () => {
  afterEach(() => vi.clearAllMocks());

  it("discards a delayed pre-create/delete GET and refreshes both mirrored lists", async () => {
    const pending = Promise.withResolvers<ListAgentsResponse>();
    vi.mocked(listAgents)
      .mockReturnValueOnce(pending.promise)
      .mockResolvedValueOnce(snapshot([PROFILE_IDS.created, PROFILE_IDS.live]));
    let store!: StoreApi<AppState>;
    function Harness() {
      store = useAppStoreApi();
      useAgentProfileSettings(AGENT_NAME, "missing");
      return null;
    }
    render(
      <StateProvider>
        <Harness />
      </StateProvider>,
    );
    await waitFor(() => expect(listAgents).toHaveBeenCalledTimes(1));
    const api = store;
    const baseAgent = {
      id: AGENT_ID,
      name: AGENT_NAME,
      profiles: [
        { id: PROFILE_IDS.live, name: PROFILE_IDS.live },
        { id: PROFILE_IDS.deleted, name: PROFILE_IDS.deleted },
      ],
    } as Agent;
    act(() => {
      api.getState().setSettingsAgents([baseAgent]);
      api.getState().setAgentProfiles([option(PROFILE_IDS.live), option(PROFILE_IDS.deleted)]);
      const handlers = registerAgentsHandlers(api);
      handlers["agent.profile.created"]!({
        timestamp: MEMBERSHIP_EVENT_TIME,
        payload: {
          profile: {
            id: PROFILE_IDS.created,
            agent_id: AGENT_ID,
            name: "Created",
            created_at: MEMBERSHIP_EVENT_TIME,
            updated_at: MEMBERSHIP_EVENT_TIME,
          },
        },
      } as BackendMessageMap["agent.profile.created"]);
      handlers["agent.profile.deleted"]!({
        timestamp: MEMBERSHIP_EVENT_TIME,
        payload: { profile: { id: PROFILE_IDS.deleted, agent_id: AGENT_ID } },
      } as BackendMessageMap["agent.profile.deleted"]);
    });
    await act(async () => {
      pending.resolve(snapshot([PROFILE_IDS.live, PROFILE_IDS.deleted]));
    });
    await waitFor(() => expect(listAgents).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(api.getState().settingsAgents.items[0]?.profiles.map((profile) => profile.id)).toEqual(
        [PROFILE_IDS.created, PROFILE_IDS.live],
      ),
    );
    expect(api.getState().agentProfiles.items.map((item) => item.id)).toEqual([
      PROFILE_IDS.created,
      PROFILE_IDS.live,
    ]);
  });
});
