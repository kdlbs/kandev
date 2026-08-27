import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";

import { createSettingsSlice } from "./settings-slice";
import type { SettingsSlice } from "./types";
import type { AvailableAgent, CapabilityStatus } from "@/lib/types/http-agents";

const AGENT_NAME = "claude-acp";
const TIMESTAMP = "2026-07-26T10:00:00Z";
const NEWER_TIMESTAMP = "2026-07-26T11:00:00Z";

function makeStore() {
  return create<SettingsSlice>()(immer((set, get, store) => createSettingsSlice(set, get, store)));
}

function updateJob(overrides: Record<string, unknown> = {}) {
  return {
    job_id: "update-1",
    agent_name: AGENT_NAME,
    status: "updating",
    current_version: "1.0.0",
    target_version: "1.1.0",
    output: "",
    started_at: TIMESTAMP,
    ...overrides,
  };
}

describe("user settings snapshots", () => {
  it("rejects an older server snapshot but allows an equal-revision optimistic update", () => {
    const store = makeStore();
    const actions = store.getState();
    const current = {
      ...actions.userSettings,
      appStatusBarEnabled: true,
      revision: 2,
    };

    actions.setUserSettings(current);
    actions.setUserSettings({ ...current, appStatusBarEnabled: false, revision: 1 });
    expect(store.getState().userSettings).toMatchObject({
      appStatusBarEnabled: true,
      revision: 2,
    });

    actions.setUserSettings({ ...current, appStatusBarEnabled: false });
    expect(store.getState().userSettings).toMatchObject({
      appStatusBarEnabled: false,
      revision: 2,
    });
  });
});

describe("settings update jobs", () => {
  it("rehydrates the newest retained job for each agent", () => {
    const store = makeStore();
    const actions = store.getState() as SettingsSlice & {
      setAgentUpdateJobs: (jobs: ReturnType<typeof updateJob>[]) => void;
    };

    actions.setAgentUpdateJobs([
      updateJob({ job_id: "older", started_at: TIMESTAMP }),
      updateJob({ job_id: "newer", started_at: "2026-07-26T10:01:00Z" }),
    ]);

    expect(
      (store.getState() as SettingsSlice & { updateJobs: { byAgent: Record<string, unknown> } })
        .updateJobs.byAgent["claude-acp"],
    ).toMatchObject({ job_id: "newer" });
  });

  it("does not let an older HTTP snapshot clobber newer websocket output", () => {
    const store = makeStore();
    const actions = store.getState() as SettingsSlice & {
      upsertAgentUpdateJob: (job: ReturnType<typeof updateJob>) => void;
      appendAgentUpdateOutput: (agentName: string, jobId: string, chunk: string) => void;
    };

    actions.upsertAgentUpdateJob(updateJob({ output: "downloaded\n" }));
    actions.appendAgentUpdateOutput("claude-acp", "update-1", "refreshed\n");
    actions.upsertAgentUpdateJob(updateJob({ status: "refreshing", output: "downloaded\n" }));

    expect(
      (
        store.getState() as SettingsSlice & {
          updateJobs: { byAgent: Record<string, { output?: string; status: string }> };
        }
      ).updateJobs.byAgent["claude-acp"],
    ).toMatchObject({
      output: "downloaded\nrefreshed\n",
      status: "refreshing",
    });
  });

  it("drops stale job events after a retry starts", () => {
    const store = makeStore();
    const actions = store.getState() as SettingsSlice & {
      upsertAgentUpdateJob: (job: ReturnType<typeof updateJob>) => void;
    };

    actions.upsertAgentUpdateJob(
      updateJob({ job_id: "retry", started_at: "2026-07-26T10:02:00Z" }),
    );
    actions.upsertAgentUpdateJob(
      updateJob({
        job_id: "original",
        status: "failed",
        started_at: TIMESTAMP,
      }),
    );

    expect(
      (
        store.getState() as SettingsSlice & {
          updateJobs: { byAgent: Record<string, { job_id: string }> };
        }
      ).updateJobs.byAgent["claude-acp"].job_id,
    ).toBe("retry");
  });

  it("keeps only the newest 64 KiB of streamed output", () => {
    const store = makeStore();
    const actions = store.getState() as SettingsSlice & {
      upsertAgentUpdateJob: (job: ReturnType<typeof updateJob>) => void;
      appendAgentUpdateOutput: (agentName: string, jobId: string, chunk: string) => void;
    };

    actions.upsertAgentUpdateJob(updateJob());
    actions.appendAgentUpdateOutput("claude-acp", "update-1", `old${"x".repeat(64 * 1024)}tail`);

    const output = (
      store.getState() as SettingsSlice & {
        updateJobs: { byAgent: Record<string, { output: string }> };
      }
    ).updateJobs.byAgent["claude-acp"].output;
    expect(output).toHaveLength(64 * 1024);
    expect(output.endsWith("tail")).toBe(true);
    expect(output.startsWith("old")).toBe(false);
  });
});

/** Builds an AvailableAgent fixture reporting the agent as settled-but-uninstalled. */
function notInstalledAvailableAgent(overrides: Partial<AvailableAgent> = {}): AvailableAgent {
  return {
    name: AGENT_NAME,
    display_name: "Claude",
    supports_mcp: false,
    installation_paths: [],
    available: false,
    capabilities: {
      supports_session_resume: false,
      supports_shell: false,
      supports_workspace_only: false,
    },
    model_config: {
      default_model: "default",
      available_models: [],
      supports_dynamic_models: false,
      status: "not_installed",
      error: "agent not installed",
    },
    updated_at: TIMESTAMP,
    ...overrides,
  };
}

/**
 * Builds an AvailableAgent fixture for a non-inference agent (e.g. a
 * TUI/passthrough agent) that never enters the host-utility probe cache, so
 * `model_config.status` stays the cache-miss "not_configured" regardless of
 * install state. `available` reflects the agent's own detection separately.
 */
function nonInferenceAvailableAgent(overrides: Partial<AvailableAgent> = {}): AvailableAgent {
  return {
    name: AGENT_NAME,
    display_name: "Claude",
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
      status: "not_configured",
    },
    updated_at: TIMESTAMP,
    ...overrides,
  };
}

/** Seeds one profile + its owning settingsAgents entry, both under AGENT_NAME/"agent-1". */
function seedSingleAgentProfile(
  store: ReturnType<typeof makeStore>,
  capabilityStatus?: CapabilityStatus,
) {
  const capability = capabilityStatus ? { capability_status: capabilityStatus } : {};
  store.setState((state) => ({
    ...state,
    agentProfiles: {
      ...state.agentProfiles,
      items: [
        {
          id: "profile-1",
          label: "Claude • Default",
          agent_id: "agent-1",
          agent_name: AGENT_NAME,
          cli_passthrough: false,
          ...capability,
        },
      ],
    },
    settingsAgents: {
      items: [
        {
          id: "agent-1",
          name: AGENT_NAME,
          supports_mcp: false,
          profiles: [],
          created_at: TIMESTAMP,
          updated_at: TIMESTAMP,
          ...capability,
        },
      ],
    },
  }));
}

describe("setAvailableAgents capability propagation", () => {
  it("does not let an older HTTP snapshot clobber a newer capability snapshot", () => {
    const store = makeStore();
    const actions = store.getState();
    seedSingleAgentProfile(store, "not_installed");

    actions.setAvailableAgents([
      notInstalledAvailableAgent({
        available: true,
        model_config: {
          default_model: "default",
          available_models: [],
          supports_dynamic_models: false,
          status: "ok",
        },
        updated_at: NEWER_TIMESTAMP,
      }),
    ]);
    actions.setAvailableAgentsLoading(true);
    actions.setAvailableAgents([notInstalledAvailableAgent({ updated_at: TIMESTAMP })]);

    expect(store.getState().availableAgents.items[0]?.model_config.status).toBe("ok");
    expect(store.getState().agentProfiles.items[0]?.capability_status).toBe("ok");
    expect(store.getState().availableAgents.loading).toBe(false);
  });

  it("flips a profile and its settingsAgents entry from probing to the settled status a poll snapshot reports", () => {
    const store = makeStore();
    const actions = store.getState();
    seedSingleAgentProfile(store, "probing");

    actions.setAvailableAgents([notInstalledAvailableAgent()]);

    const profile = store.getState().agentProfiles.items[0];
    const settingsAgent = store.getState().settingsAgents.items[0];
    expect(profile?.capability_status).toBe("not_installed");
    expect(profile?.capability_error).toBe("agent not installed");
    expect(settingsAgent?.capability_status).toBe("not_installed");
    expect(settingsAgent?.capability_error).toBe("agent not installed");
  });

  it("treats a non-inference agent's own failed detection as not_installed, not healthy", () => {
    // TUI/passthrough agents never enter the host-utility probe cache (they
    // are not InferenceAgents), so model_config.status is permanently
    // "not_configured" regardless of whether the agent is actually
    // installed. `available: false` is the only signal that its own
    // detection failed; collapsing "not_configured" to undefined here would
    // silently show it as healthy in Handoff.
    const store = makeStore();
    const actions = store.getState();
    seedSingleAgentProfile(store);

    actions.setAvailableAgents([nonInferenceAvailableAgent({ available: false })]);

    const profile = store.getState().agentProfiles.items[0];
    const settingsAgent = store.getState().settingsAgents.items[0];
    expect(profile?.capability_status).toBe("not_installed");
    expect(settingsAgent?.capability_status).toBe("not_installed");
  });

  it("keeps a not-yet-probed non-inference agent healthy when its own detection succeeded", () => {
    // The pre-existing cache-miss case: available (installed), just not yet
    // (or never) probed by the host-utility cache. Must stay undefined so
    // it does not vanish from Handoff.
    const store = makeStore();
    const actions = store.getState();
    seedSingleAgentProfile(store);

    actions.setAvailableAgents([nonInferenceAvailableAgent({ available: true })]);

    const profile = store.getState().agentProfiles.items[0];
    const settingsAgent = store.getState().settingsAgents.items[0];
    expect(profile?.capability_status).toBeUndefined();
    expect(settingsAgent?.capability_status).toBeUndefined();
  });

  it("leaves capability_status untouched when the poll snapshot has no matching agent name", () => {
    const store = makeStore();
    const actions = store.getState();

    store.setState((state) => ({
      ...state,
      agentProfiles: {
        ...state.agentProfiles,
        items: [
          {
            id: "profile-1",
            label: "Claude • Default",
            agent_id: "agent-1",
            agent_name: AGENT_NAME,
            cli_passthrough: false,
            capability_status: "probing",
          },
        ],
      },
    }));

    actions.setAvailableAgents([notInstalledAvailableAgent({ name: "other-agent" })]);

    expect(store.getState().agentProfiles.items[0]?.capability_status).toBe("probing");
  });
});
