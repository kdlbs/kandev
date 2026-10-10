import { act, renderHook } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { describe, expect, it } from "vitest";

import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { normalizeAgentProfile } from "@/lib/api/domains/agent-profile-normalize";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import type { Agent } from "@/lib/types/http";
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
