import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";

import { createSettingsSlice } from "./settings-slice";
import type { SettingsSlice } from "./types";
import type { AvailableAgent } from "@/lib/types/http-agents";

const AGENT_NAME = "claude-acp";
const TIMESTAMP = "2026-07-26T10:00:00Z";

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
      updateJob({ job_id: "older", started_at: "2026-07-26T10:00:00Z" }),
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
        started_at: "2026-07-26T10:00:00Z",
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

describe("setAvailableAgents capability propagation", () => {
  it("flips a profile and its settingsAgents entry from probing to the settled status a poll snapshot reports", () => {
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
      settingsAgents: {
        items: [
          {
            id: "agent-1",
            name: AGENT_NAME,
            supports_mcp: false,
            profiles: [],
            capability_status: "probing",
            created_at: TIMESTAMP,
            updated_at: TIMESTAMP,
          },
        ],
      },
    }));

    actions.setAvailableAgents([notInstalledAvailableAgent()]);

    const profile = store.getState().agentProfiles.items[0];
    const settingsAgent = store.getState().settingsAgents.items[0];
    expect(profile?.capability_status).toBe("not_installed");
    expect(profile?.capability_error).toBe("agent not installed");
    expect(settingsAgent?.capability_status).toBe("not_installed");
    expect(settingsAgent?.capability_error).toBe("agent not installed");
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
