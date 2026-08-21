import { describe, expect, it, vi } from "vitest";
import { create, type StoreApi } from "zustand";
import { immer } from "zustand/middleware/immer";
import type { Agent, AgentProfile } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import { createSettingsSlice } from "@/lib/state/slices/settings/settings-slice";
import type { SettingsSlice } from "@/lib/state/slices/settings/types";
import type { AppState } from "@/lib/state/store";
import { registerAgentsHandlers } from "@/lib/ws/handlers/agents";
import { applyProfileDuplicated } from "./use-profile-duplicate";

vi.mock("@/app/actions/agents", () => ({ duplicateAgentProfileAction: vi.fn() }));
vi.mock("@/components/state-provider", () => ({ useAppStoreApi: vi.fn() }));
vi.mock("@/components/toast-provider", () => ({ useToast: vi.fn() }));

const COPY_NAME = "Default Copy";
const OLD_REVISION = "2026-08-11T21:00:00Z";
const NEW_REVISION = "2026-08-11T22:00:00Z";

/** Builds an AgentProfile fixture with an optional updatedAt. */
function profile(id: string, agentId: string, name = id, updatedAt = ""): AgentProfile {
  return {
    id,
    agentId,
    name,
    agentDisplayName: "Test Agent",
    model: "",
    allowIndexing: false,
    autoApprove: false,
    cliFlags: [],
    cliPassthrough: false,
    enabled: true,
    createdAt: "",
    updatedAt,
  } as unknown as AgentProfile;
}

/** Builds an Agent fixture carrying the given profiles. */
function agent(id: string, ...profiles: AgentProfile[]): Agent {
  return { id, name: id, profiles } as unknown as Agent;
}

/** Builds an AgentProfileOption fixture with an optional updatedAt. */
function option(id: string, updatedAt = ""): AgentProfileOption {
  return {
    id,
    label: id,
    agent_id: "a1",
    agent_name: "a1",
    cli_passthrough: false,
    updatedAt,
  };
}

/** Builds the settings slice state the duplicate merge operates on. */
function stateWith(agents: Agent[], options: AgentProfileOption[]) {
  return {
    settingsAgents: { items: agents, version: 0 },
    agentProfiles: { items: options, version: 0 },
  };
}

describe("applyProfileDuplicated", () => {
  it("appends the copy to the owning agent and surfaces it as an option", () => {
    const source = profile("p1", "a1", "Default");
    const copy = profile("p2", "a1", COPY_NAME);
    const initial = stateWith([agent("a1", source)], []);

    const next = applyProfileDuplicated(initial, agent("a1"), copy);

    const profiles = next.settingsAgents.items.find((a) => a.id === "a1")?.profiles ?? [];
    expect(profiles.map((p) => p.id)).toEqual(["p1", "p2"]);
    // Every profile (source + copy) is rebuilt into the options slice.
    expect(next.agentProfiles.items.map((o) => o.id)).toEqual(["p1", "p2"]);
  });

  it("dedupes by id so a WS-delivered copy is not double-inserted", () => {
    const source = profile("p1", "a1");
    const copy = profile("p2", "a1", COPY_NAME);
    // The WS agent.profile.created handler delivered the copy first.
    const initial = stateWith([agent("a1", source)], [option("p2")]);

    const next = applyProfileDuplicated(initial, agent("a1"), copy);

    const profiles = next.settingsAgents.items.find((a) => a.id === "a1")?.profiles ?? [];
    expect(profiles.filter((p) => p.id === "p2")).toHaveLength(1);
    expect(next.agentProfiles.items.filter((o) => o.id === "p2")).toHaveLength(1);
  });

  it("keeps the copy visible when the owning agent left the store mid-request", () => {
    const copy = profile("p2", "a1", COPY_NAME);
    const initial = stateWith([], []);

    const next = applyProfileDuplicated(initial, agent("a1"), copy);

    // settingsAgents is unchanged (no agent to attach to), but the copy must
    // still surface in the agentProfiles slice instead of being dropped.
    expect(next.settingsAgents.items).toEqual([]);
    expect(next.agentProfiles.items.map((o) => o.id)).toEqual(["p2"]);
  });

  it("does not erase a WS-delivered copy and refreshes it with agent metadata", () => {
    const copy = profile("p2", "a1", COPY_NAME);
    // WS delivered the option first (with a stub agent_name) while the
    // owning agent is absent; the merge must replace it with the full-agent
    // option rather than keep the empty-name stub.
    const initial = stateWith([], [option("p2")]);

    const next = applyProfileDuplicated(initial, agent("a1"), copy);

    const options = next.agentProfiles.items.filter((o) => o.id === "p2");
    expect(options).toHaveLength(1);
    expect(options[0].agent_name).toBe("a1");
  });

  it("keeps a newer WS-delivered orphan option over a stale duplicate response", () => {
    // Owner absent; WS delivered a NEWER version of the copy than this
    // delayed HTTP response. The merge must not regress it to the stale one.
    const staleCreated = profile("p2", "a1", COPY_NAME, OLD_REVISION);
    const newerOrphan = option("p2", NEW_REVISION);
    const initial = stateWith([], [newerOrphan]);

    const next = applyProfileDuplicated(initial, agent("a1"), staleCreated);

    const options = next.agentProfiles.items.filter((o) => o.id === "p2");
    expect(options).toHaveLength(1);
    // The preserved option keeps the newer revision (label from the option).
    expect(options[0].label).toBe("p2");
    expect(options[0].updatedAt).toBe(NEW_REVISION);
  });

  it("keeps a newer WS-delivered option over a stale rebuilt one when the owner is present", () => {
    // The option was updated by WS (enabled=false, newer revision) while the
    // settingsAgents profile is stale. The merge must not regress it to the
    // rebuilt (older) version — a disabled profile must stay hidden.
    const staleProfile = profile("p1", "a1", "Default", OLD_REVISION);
    const copy = profile("p2", "a1", COPY_NAME, OLD_REVISION);
    const newerCopyOption = { ...option("p2", NEW_REVISION), enabled: false };
    const initial = stateWith([agent("a1", staleProfile)], [newerCopyOption]);

    const next = applyProfileDuplicated(initial, agent("a1"), copy);

    const options = next.agentProfiles.items.filter((o) => o.id === "p2");
    expect(options).toHaveLength(1);
    expect(options[0].enabled).toBe(false);
    expect(options[0].updatedAt).toBe(NEW_REVISION);
  });

  it("orders revisions by instant, not lexically, for offset timestamps", () => {
    // "2026-08-11T23:30:00+02:00" is 21:30 UTC — OLDER than 22:00Z, but
    // lexically larger. A string compare would keep the stale duplicate
    // response over the newer WS-delivered option.
    const staleCreated = profile("p2", "a1", COPY_NAME, "2026-08-11T23:30:00+02:00");
    const newerOrphan = option("p2", "2026-08-11T22:00:00Z");
    const initial = stateWith([], [newerOrphan]);

    const next = applyProfileDuplicated(initial, agent("a1"), staleCreated);

    const options = next.agentProfiles.items.filter((o) => o.id === "p2");
    expect(options).toHaveLength(1);
    expect(options[0].updatedAt).toBe("2026-08-11T22:00:00Z");
  });

  it("keeps a newer WS-delivered version instead of the stale duplicate response", () => {
    // Another tab edited the copy after creation: agent.profile.updated
    // delivered the newer version before this delayed duplicate response.
    const newer = profile("p2", "a1", "Default Copy (edited)", NEW_REVISION);
    const staleCreated = profile("p2", "a1", COPY_NAME, OLD_REVISION);
    const source = profile("p1", "a1");
    const initial = stateWith([agent("a1", source, newer)], [option("p1"), option("p2")]);

    const next = applyProfileDuplicated(initial, agent("a1"), staleCreated);

    const stored = next.settingsAgents.items.find((a) => a.id === "a1")?.profiles ?? [];
    expect(stored.find((p) => p.id === "p2")?.name).toBe("Default Copy (edited)");
    expect(next.agentProfiles.items.find((o) => o.id === "p2")?.label).toContain(
      "Default Copy (edited)",
    );
  });

  it("carries the owning agent's capability_status onto the copy when the agent is absent from the store", () => {
    // Mirrors the mid-request scenario above, but the duplicated profile's
    // agent is unhealthy: the copy must not silently read as healthy just
    // because it took the stub-agent branch (toAgentProfileOption dropped
    // capability_status when only { id, name } was passed).
    const copy = profile("p2", "a1", COPY_NAME);
    const unhealthyAgent = {
      ...agent("a1"),
      capability_status: "not_installed",
      capability_error: "agent CLI not found",
    } as Agent;
    const initial = stateWith([], []);

    const next = applyProfileDuplicated(initial, unhealthyAgent, copy);

    const created = next.agentProfiles.items.find((o) => o.id === "p2");
    expect(created?.capability_status).toBe("not_installed");
    expect(created?.capability_error).toBe("agent CLI not found");
  });

  it("preserves WS-delivered orphan options for agents absent from the store", () => {
    // p-orphan was delivered by WS for an agent not present in settingsAgents;
    // duplicating a profile of a DIFFERENT agent must not wipe it.
    const source = profile("p1", "a2");
    const copy = profile("p2", "a2", COPY_NAME);
    const initial = stateWith([agent("a2", source)], [option("p-orphan")]);

    const next = applyProfileDuplicated(initial, agent("a2"), copy);

    const ids = next.agentProfiles.items.map((o) => o.id);
    expect(ids).toContain("p-orphan");
    expect(ids).toContain("p2");
    expect(ids.filter((id) => id === "p2")).toHaveLength(1);
  });
});

describe("applyProfileDuplicated capability_status sync", () => {
  it("picks up a fresh agent.available.updated capability instead of reverting to the stale settingsAgents value", () => {
    // Regression for the desync where agent.available.updated only refreshed
    // agentProfiles options: settingsAgents stayed on its stale boot-time
    // capability_status, so duplicating any profile rebuilt every option from
    // that stale value and clobbered the refresh (both directions).
    const store = create<SettingsSlice>()(
      immer((set, get, api) => createSettingsSlice(set, get, api)),
    ) as unknown as StoreApi<AppState>;
    const handlers = registerAgentsHandlers(store) as unknown as Record<
      string,
      (event: {
        id: string;
        type: string;
        action: string;
        timestamp: string;
        payload: unknown;
      }) => void
    >;
    const source = profile("p1", "a1", "Default");
    store.setState((state) => ({
      ...state,
      settingsAgents: {
        items: [
          {
            id: "a1",
            name: "a1",
            supports_mcp: false,
            profiles: [source],
            capability_status: "not_installed",
            capability_error: "agent not installed",
            created_at: "",
            updated_at: "",
          } as unknown as Agent,
        ],
      },
    }));

    // The agent gets installed: available.updated reports it healthy.
    handlers["agent.available.updated"]({
      id: "m1",
      type: "notification",
      action: "agent.available.updated",
      timestamp: "2026-08-11T21:00:00Z",
      payload: {
        agents: [
          {
            name: "a1",
            display_name: "a1",
            supports_mcp: false,
            installation_paths: [],
            available: true,
            capabilities: {
              supports_session_resume: false,
              supports_shell: false,
              supports_workspace_only: false,
            },
            model_config: {
              default_model: "default",
              available_models: [],
              supports_dynamic_models: false,
              status: "ok",
            },
            updated_at: "2026-08-11T21:00:00Z",
          },
        ],
      },
    });

    // `agent("a1")` mirrors the click-time Agent captured by the caller
    // before the WS refresh above landed in the store — it carries no
    // capability_status, so it must lose to the rebuilt value for both the
    // pre-existing profile and the new copy.
    const copy = profile("p2", "a1", COPY_NAME);
    const next = applyProfileDuplicated(store.getState(), agent("a1"), copy);

    const rebuilt = next.agentProfiles.items.find((o) => o.id === "p1");
    expect(rebuilt?.capability_status).toBe("ok");
    expect(rebuilt?.capability_error).toBeUndefined();

    const rebuiltCopy = next.agentProfiles.items.find((o) => o.id === "p2");
    expect(rebuiltCopy?.capability_status).toBe("ok");
    expect(rebuiltCopy?.capability_error).toBeUndefined();
  });
});
