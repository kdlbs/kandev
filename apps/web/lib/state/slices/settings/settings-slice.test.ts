import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";

import { createSettingsSlice } from "./settings-slice";
import type { SettingsSlice } from "./types";

function makeStore() {
  return create<SettingsSlice>()(immer((set, get, store) => createSettingsSlice(set, get, store)));
}

function updateJob(overrides: Record<string, unknown> = {}) {
  return {
    job_id: "update-1",
    agent_name: "claude-acp",
    status: "updating",
    current_version: "1.0.0",
    target_version: "1.1.0",
    output: "",
    started_at: "2026-07-26T10:00:00Z",
    ...overrides,
  };
}

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
