import { act, renderHook } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { describe, expect, it } from "vitest";

import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { normalizeAgentProfile } from "@/lib/api/domains/agent-profile-normalize";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import type { Agent } from "@/lib/types/http";
import { agentProfileId } from "@/lib/types/ids";
import { useAgentCreationStoreSync } from "./use-agent-creation-store-sync";

const profile = normalizeAgentProfile({
  id: "accepted-hook-profile",
  agent_id: "hook-owner",
  name: "Accepted",
  model: "review-model",
  created_at: "2026-10-10T12:00:00Z",
  updated_at: "2026-10-10T12:00:00Z",
});
const owner: Agent = {
  id: "hook-owner",
  name: "claude-code",
  supports_mcp: true,
  mcp_config_path: "",
  profiles: [profile],
  created_at: profile.createdAt,
  updated_at: profile.updatedAt,
};

function setup(current = owner) {
  function Wrapper({ children }: PropsWithChildren) {
    return (
      <StateProvider
        initialState={{
          settingsAgents: { items: [current] },
          agentProfiles: {
            orderByAgent: {},
            items: current.profiles.map((item) => toAgentProfileOption(current, item)),
            version: 1,
          },
        }}
      >
        {children}
      </StateProvider>
    );
  }
  return renderHook(() => ({ ...useAgentCreationStoreSync(), store: useAppStoreApi() }), {
    wrapper: Wrapper,
  });
}

describe("accepted creation publication guards", () => {
  // @covers AC-AGENTS-TARGET-CREATION-CATALOGUE-001.3
  it("deduplicates a matching event copy and publishes the accepted normalized fields", () => {
    const { result } = setup();
    act(() =>
      result.current.upsertAgent(owner, { profiles: [{ ...profile, name: "Saved name" }] }),
    );
    const state = result.current.store.getState();
    expect(state.settingsAgents.items[0].profiles).toEqual([{ ...profile, name: "Saved name" }]);
    expect(state.agentProfiles.items).toHaveLength(1);
    expect(state.agentProfiles.items[0].label).toContain("Saved name");
  });

  // @covers AC-AGENTS-TARGET-CREATION-CATALOGUE-001.2
  it("keeps a strictly newer received copy of the accepted identity", () => {
    const current = {
      ...profile,
      name: "Newer live name",
      model: "live-model",
      updatedAt: "2026-10-10T12:00:00.100Z",
    };
    const { result } = setup({ ...owner, profiles: [current] });
    act(() => result.current.upsertAgent(owner, { profiles: [profile] }));
    const state = result.current.store.getState();
    expect(state.settingsAgents.items[0].profiles).toEqual([current]);
    expect(state.agentProfiles.items[0]).toMatchObject({
      model: "live-model",
      updatedAt: current.updatedAt,
    });
  });

  // @covers AC-AGENTS-TARGET-CREATION-CATALOGUE-001.5
  it("retains a pending MCP draft on a newer persisted accepted copy", () => {
    const current = { ...profile, name: "Newer live name", updatedAt: "2026-10-10T12:00:01Z" };
    const { result } = setup({ ...owner, profiles: [current] });
    const pending = {
      ...profile,
      mcp_config: { enabled: true, servers: "{}", dirty: true, error: null },
    };
    act(() => result.current.upsertAgent(owner, { profiles: [pending] }));
    expect(result.current.store.getState().settingsAgents.items[0].profiles).toEqual([
      { ...current, mcp_config: pending.mcp_config },
    ]);
  });

  it("does not reinsert a missing current owner from its captured creation baseline", () => {
    const { result } = setup();
    act(() => result.current.store.setState({ settingsAgents: { items: [] } }));
    const before = result.current.store.getState();
    act(() => result.current.upsertAgent(owner, { profiles: [profile] }));
    expect(result.current.store.getState().settingsAgents.items).toEqual([]);
    expect(result.current.store.getState().agentProfiles).toBe(before.agentProfiles);
  });

  it("applies submitted owner fields while retaining current capability metadata", () => {
    const { result } = setup({ ...owner, supports_mcp: false, inference_capable: false });
    act(() =>
      result.current.upsertAgent(owner, {
        profiles: [profile],
        agentPatch: { workspace_id: "submitted-workspace", mcp_config_path: "submitted-path" },
      }),
    );
    expect(result.current.store.getState().settingsAgents.items[0]).toMatchObject({
      supports_mcp: false,
      inference_capable: false,
      workspace_id: "submitted-workspace",
      mcp_config_path: "submitted-path",
    });
  });
});

describe("accepted creation timestamp validation", () => {
  it.each(["9999", "2026-02-30T00:00:00Z"])(
    "does not treat malformed current timestamp %s as a newer copy",
    (updatedAt) => {
      const accepted = { ...profile, updatedAt: "2026-02-28T00:00:00Z" };
      const current = { ...profile, name: "Malformed live copy", updatedAt };
      const { result } = setup({ ...owner, profiles: [current] });
      act(() => result.current.upsertAgent(owner, { profiles: [accepted] }));
      expect(result.current.store.getState().settingsAgents.items[0].profiles).toEqual([accepted]);
      expect(result.current.store.getState().agentProfiles.items[0].updatedAt).toBe(
        accepted.updatedAt,
      );
    },
  );

  it.each(["9999", "2026-02-30T00:00:00Z"])(
    "omits malformed accepted timestamp %s from recency comparison",
    (updatedAt) => {
      const accepted = { ...profile, name: "Malformed accepted copy", updatedAt };
      const current = { ...profile, updatedAt: "2026-02-28T00:00:00Z" };
      const { result } = setup({ ...owner, profiles: [current] });
      act(() => result.current.upsertAgent(owner, { profiles: [accepted] }));
      expect(result.current.store.getState().settingsAgents.items[0].profiles).toEqual([current]);
    },
  );
});

it("keeps a strictly newer accepted copy within the same millisecond", () => {
  const accepted = { ...profile, updatedAt: "2026-10-10T12:00:00.000000100Z" };
  const current = {
    ...profile,
    name: "Nanosecond newer",
    updatedAt: "2026-10-10T12:00:00.000000200Z",
  };
  const { result } = setup({ ...owner, profiles: [current] });
  act(() => result.current.upsertAgent(owner, { profiles: [accepted] }));
  expect(result.current.store.getState().settingsAgents.items[0].profiles).toEqual([current]);
});

it("prepends accepted new profiles while preserving saved Settings order and orphan selector options", () => {
  const older = { ...profile, id: agentProfileId("older"), createdAt: "2026-01-01T00:00:00Z" };
  const newer = { ...profile, id: agentProfileId("newer"), createdAt: "2026-02-01T00:00:00Z" };
  const created = { ...profile, id: agentProfileId("created"), createdAt: "2026-03-01T00:00:00Z" };
  const current = { ...owner, profile_order_revision: 1, profiles: [older, newer] };
  const { result } = setup(current);
  act(() => {
    const state = result.current.store.getState();
    state.setAgentProfiles([
      ...state.agentProfiles.items,
      { ...toAgentProfileOption(owner, profile), id: "office", workspace_id: "workspace" },
    ]);
    result.current.upsertAgent(owner, { profiles: [created] });
  });
  const state = result.current.store.getState();
  expect(state.settingsAgents.items[0].profiles.map((item) => item.id)).toEqual([
    "created",
    "older",
    "newer",
  ]);
  expect(state.agentProfiles.items.map((item) => item.id)).toEqual([
    "created",
    "newer",
    "older",
    "office",
  ]);
});

it("publishes an accepted new owner after an independent catalogue event", () => {
  const { result } = setup({ ...owner, id: "independent", profiles: [] });
  const startVersion = result.current.getAgentProfilesVersion();
  act(() => result.current.store.getState().bumpAgentProfilesVersion());
  act(() =>
    result.current.upsertAgent(owner, { profiles: [profile], ownerCreated: true }, startVersion),
  );
  const state = result.current.store.getState();
  expect(state.settingsAgents.items.map((agent) => agent.id)).toEqual(["independent", owner.id]);
  expect(state.settingsAgents.items[1].profiles).toEqual([profile]);
  expect(state.agentProfiles.items).toEqual([toAgentProfileOption(owner, profile)]);
  expect(state.agentProfiles.version).toBeGreaterThan(startVersion);
});

it("keeps an observed owner removal when a late creation result publishes", () => {
  const { result } = setup();
  const startVersion = result.current.getAgentProfilesVersion();
  act(() => {
    result.current.store.getState().setSettingsAgents([]);
    result.current.store.getState().setAgentProfiles([]);
    result.current.store.getState().bumpAgentProfilesVersion();
  });
  const removed = result.current.store.getState();
  act(() =>
    result.current.upsertAgent(owner, { profiles: [profile], ownerCreated: true }, startVersion),
  );
  expect(result.current.store.getState().settingsAgents.items).toEqual([]);
  expect(result.current.store.getState().agentProfiles).toBe(removed.agentProfiles);
});

it("keeps owner removal after the editor unmounts during pending creation", () => {
  const { result, unmount } = setup();
  const { store, upsertAgent, getAgentProfilesVersion } = result.current;
  const startVersion = getAgentProfilesVersion();
  unmount();
  act(() => {
    store.getState().setSettingsAgents([]);
    store.getState().setAgentProfiles([]);
    store.getState().bumpAgentProfilesVersion();
  });
  const removed = store.getState();
  act(() => upsertAgent(owner, { profiles: [profile], ownerCreated: true }, startVersion));
  expect(store.getState().settingsAgents.items).toEqual([]);
  expect(store.getState().agentProfiles).toBe(removed.agentProfiles);
});

it("keeps observed removals scoped to their owning store", () => {
  const removed = setup();
  act(() => removed.result.current.store.getState().setSettingsAgents([]));
  const independent = setup({ ...owner, id: "independent", profiles: [] });
  act(() =>
    independent.result.current.upsertAgent(owner, { profiles: [profile], ownerCreated: true }),
  );
  expect(
    independent.result.current.store.getState().settingsAgents.items.map((agent) => agent.id),
  ).toEqual(["independent", owner.id]);
});
