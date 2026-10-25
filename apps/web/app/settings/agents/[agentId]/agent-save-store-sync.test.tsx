import { act, render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";
import type { Agent } from "@/lib/types/http";
import { registerAgentsHandlers } from "@/lib/ws/handlers/agents";
import { syncSavedAgentToStore } from "./agent-save-store-sync";

const MOCK_AGENT_NAME = "mock-agent";
const AGENT_ID = "agent";
const LIVE_PROFILE_ID = "live";
const CREATED_PROFILE_ID = "created";
const DELETED_PROFILE_ID = "deleted";
const ORPHAN_AGENT_ID = "office-agent";
const ORPHAN_PROFILE_ID = "office-profile";
const SAVED_AGENT_NAME = "saved-agent-name";
const UNRELATED_AGENT_ID = "unrelated-agent";
const UNRELATED_PROFILE_ID = "unrelated-profile";
const CREATED_AGENT_ID = "created-agent";
const CREATED_AGENT_PROFILE_ID = "created-agent-profile";

function Capture({ onStore }: { onStore: (store: StoreApi<AppState>) => void }) {
  onStore(useAppStoreApi());
  return null;
}

function profile(id: string, name = id) {
  return { id, name, model: "mock-fast", enabled: true } as Agent["profiles"][number];
}

function option(id: string, agentId = AGENT_ID) {
  return {
    id,
    label: id,
    agent_id: agentId,
    agent_name: MOCK_AGENT_NAME,
    cli_passthrough: false,
  };
}

function profileEvent(id: string, name: string, timestamp: string, agentId = AGENT_ID) {
  return {
    id,
    type: "notification",
    action: "agent.profile.created",
    timestamp,
    payload: {
      profile: {
        id,
        agent_id: agentId,
        name,
        model: "mock-fast",
        enabled: true,
        created_at: "2026-07-26T10:00:00Z",
        updated_at: timestamp,
      },
    },
  };
}

describe("agent save response membership", () => {
  it("keeps a created agent when another profile changes during creation", () => {
    let store!: StoreApi<AppState>;
    render(
      <StateProvider>
        <Capture onStore={(value) => (store = value)} />
      </StateProvider>,
    );
    const versionAtSaveStart = store.getState().agentProfiles.version;
    const handlers = registerAgentsHandlers(store);
    const createdAgent = {
      id: CREATED_AGENT_ID,
      name: CREATED_AGENT_ID,
      profiles: [profile(CREATED_AGENT_PROFILE_ID)],
    } as Agent;

    act(() => {
      handlers["agent.profile.created"]!(
        profileEvent(
          UNRELATED_PROFILE_ID,
          "Created while agent save was pending",
          "2026-07-26T11:00:00Z",
          UNRELATED_AGENT_ID,
        ) as never,
      );
    });
    expect(store.getState().agentProfiles.version).toBe(versionAtSaveStart + 1);

    act(() => syncSavedAgentToStore(store, createdAgent, versionAtSaveStart));
    expect(store.getState().settingsAgents.items.map((item) => item.id)).toEqual([]);

    act(() => syncSavedAgentToStore(store, createdAgent, versionAtSaveStart, true));
    expect(store.getState().settingsAgents.items.map((item) => item.id)).toEqual([
      CREATED_AGENT_ID,
    ]);
    expect(store.getState().agentProfiles.items.map((item) => item.id)).toEqual([
      UNRELATED_PROFILE_ID,
      CREATED_AGENT_PROFILE_ID,
    ]);
  });
});

describe("agent save response membership", () => {
  it("keeps profile create/delete events that arrive while the save response is pending", async () => {
    let store!: StoreApi<AppState>;
    render(
      <StateProvider>
        <Capture onStore={(value) => (store = value)} />
      </StateProvider>,
    );
    const existing = {
      id: AGENT_ID,
      name: MOCK_AGENT_NAME,
      profiles: [profile(LIVE_PROFILE_ID), profile(DELETED_PROFILE_ID)],
    } as Agent;
    act(() => {
      store.getState().setSettingsAgents([existing]);
      store.getState().setAgentProfiles([option(LIVE_PROFILE_ID), option(DELETED_PROFILE_ID)]);
    });
    const versionAtSaveStart = store.getState().agentProfiles.version;
    const handlers = registerAgentsHandlers(store);
    let resolveSave!: (agent: Agent) => void;
    const response = new Promise<Agent>((resolve) => {
      resolveSave = resolve;
    });
    const save = response.then((agent) => syncSavedAgentToStore(store, agent, versionAtSaveStart));

    act(() => {
      handlers["agent.profile.created"]!(
        profileEvent(CREATED_PROFILE_ID, "Created during save", "2026-07-26T11:00:00Z") as never,
      );
      handlers["agent.profile.deleted"]!({
        id: "deleted-event",
        type: "notification",
        action: "agent.profile.deleted",
        timestamp: "2026-07-26T12:00:00Z",
        payload: {
          profile: {
            id: DELETED_PROFILE_ID,
            agent_id: AGENT_ID,
            name: "Deleted during save",
            model: "mock-fast",
            enabled: true,
            created_at: "2026-07-26T10:00:00Z",
            updated_at: "2026-07-26T10:00:00Z",
          },
        },
      } as never);
    });

    await act(async () => {
      resolveSave({
        ...existing,
        name: SAVED_AGENT_NAME,
        profiles: [profile(LIVE_PROFILE_ID), profile(DELETED_PROFILE_ID)],
      });
      await save;
    });

    const savedAgent = store.getState().settingsAgents.items[0];
    expect(savedAgent.name).toBe(SAVED_AGENT_NAME);
    expect(savedAgent.profiles.map((item) => item.id)).toEqual([
      CREATED_PROFILE_ID,
      LIVE_PROFILE_ID,
    ]);
    expect(store.getState().agentProfiles.items.map((item) => item.id)).toEqual([
      CREATED_PROFILE_ID,
      LIVE_PROFILE_ID,
    ]);
  });
});

describe("agent save profile option retention", () => {
  it("preserves flat profile options whose agents are absent from the settings list", () => {
    let store!: StoreApi<AppState>;
    render(
      <StateProvider>
        <Capture onStore={(value) => (store = value)} />
      </StateProvider>,
    );
    const existing = {
      id: AGENT_ID,
      name: MOCK_AGENT_NAME,
      profiles: [profile(LIVE_PROFILE_ID)],
    } as Agent;
    act(() => {
      store.getState().setSettingsAgents([existing]);
      store
        .getState()
        .setAgentProfiles([option(LIVE_PROFILE_ID), option(ORPHAN_PROFILE_ID, ORPHAN_AGENT_ID)]);
    });

    act(() => {
      syncSavedAgentToStore(
        store,
        {
          ...existing,
          name: SAVED_AGENT_NAME,
          profiles: [
            { ...profile(LIVE_PROFILE_ID, "Saved profile"), agentDisplayName: "Saved agent" },
          ],
        },
        store.getState().agentProfiles.version,
      );
    });

    const options = store.getState().agentProfiles.items;
    expect(options.map((item) => item.id)).toEqual([LIVE_PROFILE_ID, ORPHAN_PROFILE_ID]);
    expect(options[0]).toMatchObject({
      label: "Saved agent • Saved profile",
      agent_name: SAVED_AGENT_NAME,
    });
    expect(options[1]).toMatchObject({
      id: ORPHAN_PROFILE_ID,
      agent_id: ORPHAN_AGENT_ID,
      label: ORPHAN_PROFILE_ID,
    });
  });
});

it("preserves an Office option owned by the saved agent", () => {
  let store!: StoreApi<AppState>;
  render(
    <StateProvider>
      <Capture onStore={(value) => (store = value)} />
    </StateProvider>,
  );
  const existing = {
    id: AGENT_ID,
    name: MOCK_AGENT_NAME,
    profiles: [profile(LIVE_PROFILE_ID)],
  } as Agent;
  act(() => {
    store.getState().setSettingsAgents([existing]);
    store
      .getState()
      .setAgentProfiles([
        option(LIVE_PROFILE_ID),
        { ...option("office"), workspace_id: "workspace" },
      ]);
    syncSavedAgentToStore(store, existing, store.getState().agentProfiles.version);
  });
  expect(store.getState().agentProfiles.items.map((item) => item.id)).toEqual([
    LIVE_PROFILE_ID,
    "office",
  ]);
});
