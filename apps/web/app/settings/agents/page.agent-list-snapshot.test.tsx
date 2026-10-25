import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";
import type { Agent } from "@/lib/types/http";
import AgentsSettingsPage from "./page";
import { listAgents } from "@/lib/api";
import { registerAgentsHandlers } from "@/lib/ws/handlers/agents";
import type { BackendMessageMap } from "@/lib/types/backend";

const MEMBERSHIP_EVENT_TIME = "2026-02-01T00:00:00Z";

vi.mock("@/lib/api", () => ({
  createCustomTUIAgent: vi.fn().mockResolvedValue({ name: "custom" }),
  listAgents: vi.fn(),
  listAgentDiscovery: vi.fn().mockResolvedValue({ agents: [] }),
  listAvailableAgents: vi.fn().mockResolvedValue({ agents: [], tools: [] }),
}));
vi.mock("@/hooks/domains/auth/use-is-admin", () => ({ useIsAdmin: () => true }));
vi.mock("@/hooks/domains/settings/use-agent-discovery", () => ({
  useAgentDiscovery: () => ({ items: [], loading: false }),
}));
vi.mock("@/hooks/domains/settings/use-available-agents", () => ({
  useAvailableAgents: () => ({ items: [] }),
}));
vi.mock("@/hooks/domains/settings/use-agent-runtime-updates", () => ({
  useAgentRuntimeUpdates: () => ({ updateJobs: {}, previewUpdate: vi.fn(), startUpdate: vi.fn() }),
}));
vi.mock("@/hooks/domains/settings/use-agent-runtime-update-statuses", () => ({
  useAgentRuntimeUpdateStatuses: () => ({ refresh: vi.fn(), statusByAgent: {} }),
}));
vi.mock("@/hooks/domains/settings/use-profile-order", () => ({ useProfileOrder: () => vi.fn() }));
vi.mock("@/components/settings/agents/agent-profiles-section", () => ({
  AgentProfilesSubList: () => null,
}));
vi.mock("@/components/settings/host-shell-dialog", () => ({ HostShellDialog: () => null }));
vi.mock("@/components/settings/custom-tui-mcp-card", () => ({ CustomTUIMcpCard: () => null }));
vi.mock("@/components/settings/dynamic-agents-card", () => ({ DynamicAgentsCard: () => null }));
vi.mock("@/components/settings/agent-runtime-policies", () => ({
  AgentRuntimePolicies: () => null,
}));
vi.mock("@/app/settings/agents/hide-disabled-agent-profiles-setting", () => ({
  HideDisabledAgentProfilesSetting: () => null,
}));
vi.mock("@/components/settings/add-tui-agent-dialog", () => ({
  AddTUIAgentDialog: ({ onSubmit }: { onSubmit: (data: unknown) => Promise<void> }) => (
    <button onClick={() => void onSubmit({ display_name: "Custom", command: "custom" })}>
      Create fixture TUI
    </button>
  ),
}));

beforeEach(() => vi.mocked(listAgents).mockReset());

const AGENT_ID = "agent";
const AGENT_NAME = "mock-agent";
const OFFICE_AGENT_ID = "office-agent";
const PROFILE_IDS = {
  created: "created",
  deleted: "deleted",
  live: "live",
  office: "office-profile",
  stale: "stale",
};

function Capture({ onStore }: { onStore: (store: StoreApi<AppState>) => void }) {
  onStore(useAppStoreApi());
  return null;
}

function option(id: string, agentId = AGENT_ID, agentName = AGENT_NAME) {
  return { id, label: id, agent_id: agentId, agent_name: agentName, cli_passthrough: false };
}

describe("agent page list snapshot membership fence", () => {
  it("discards the custom-TUI refresh GET when create/delete events arrive before it resolves", async () => {
    let store!: StoreApi<AppState>;
    const pending = Promise.withResolvers<{ agents: Agent[]; total: number }>();
    const oldAgent = {
      id: AGENT_ID,
      name: AGENT_NAME,
      profile_order_revision: 2,
      profiles: [
        { id: PROFILE_IDS.live, name: "Live", createdAt: "2026-01-01T00:00:00Z" },
        { id: PROFILE_IDS.deleted, name: "Deleted", createdAt: "2026-01-01T00:00:00Z" },
      ],
    } as Agent;
    const freshAgent = {
      ...oldAgent,
      profiles: [
        { id: PROFILE_IDS.created, name: "Created", createdAt: MEMBERSHIP_EVENT_TIME },
        oldAgent.profiles[0],
      ],
    } as Agent;
    vi.mocked(listAgents)
      .mockReturnValueOnce(pending.promise)
      .mockResolvedValueOnce({ agents: [freshAgent], total: 1 });
    render(
      <StateProvider>
        <Capture
          onStore={(value) => {
            store = value;
          }}
        />
        <AgentsSettingsPage />
      </StateProvider>,
    );
    act(() => store.getState().applyAgentListSnapshot([oldAgent], 0));
    fireEvent.click(screen.getByRole("button", { name: "Create fixture TUI" }));
    await waitFor(() => expect(listAgents).toHaveBeenCalledTimes(1));
    const handlers = registerAgentsHandlers(store);
    act(() => {
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
    await act(async () => pending.resolve({ agents: [oldAgent], total: 1 }));
    await waitFor(() => expect(listAgents).toHaveBeenCalledTimes(2));
    expect(store.getState().settingsAgents.items[0].profiles.map((profile) => profile.id)).toEqual([
      PROFILE_IDS.created,
      PROFILE_IDS.live,
    ]);
    expect(store.getState().agentProfiles.items.map((profile) => profile.id)).toEqual([
      PROFILE_IDS.created,
      PROFILE_IDS.live,
    ]);
  });
});

describe("accepted agent-list snapshots", () => {
  it("retains Office profile options for agents absent from an accepted list snapshot", () => {
    let store!: StoreApi<AppState>;
    render(
      <StateProvider>
        <Capture
          onStore={(value) => {
            store = value;
          }}
        />
      </StateProvider>,
    );
    const api = store;
    const existing = {
      id: AGENT_ID,
      name: AGENT_NAME,
      profiles: [
        { id: PROFILE_IDS.live, name: "Live" },
        { id: PROFILE_IDS.stale, name: "Stale" },
      ],
    } as Agent;
    act(() => {
      api.getState().setSettingsAgents([existing]);
      api
        .getState()
        .setAgentProfiles([
          option(PROFILE_IDS.live),
          option(PROFILE_IDS.stale),
          option(PROFILE_IDS.office, OFFICE_AGENT_ID, "Office Agent"),
        ]);
    });

    const accepted = api.getState().applyAgentListSnapshot(
      [
        {
          ...existing,
          profiles: [existing.profiles[0]],
        } as Agent,
      ],
      api.getState().agentProfiles.version,
    );

    expect(accepted).toBe(true);
    expect(api.getState().settingsAgents.items[0].profiles.map((profile) => profile.id)).toEqual([
      PROFILE_IDS.live,
    ]);
    expect(api.getState().agentProfiles.items.map(({ agent_id, id }) => [agent_id, id])).toEqual([
      [AGENT_ID, PROFILE_IDS.live],
      [OFFICE_AGENT_ID, PROFILE_IDS.office],
    ]);
  });
});
