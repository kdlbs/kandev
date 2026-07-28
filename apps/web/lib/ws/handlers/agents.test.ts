import { describe, expect, it } from "vitest";
import { create, type StoreApi } from "zustand";
import { immer } from "zustand/middleware/immer";

import { createSettingsSlice } from "@/lib/state/slices/settings/settings-slice";
import type { SettingsSlice } from "@/lib/state/slices/settings/types";
import type { AppState } from "@/lib/state/store";
import { registerAgentsHandlers } from "./agents";

function makeStore() {
  return create<SettingsSlice>()(
    immer((set, get, store) => createSettingsSlice(set, get, store)),
  ) as unknown as StoreApi<AppState>;
}

function message(action: string, payload: unknown) {
  return {
    id: `message-${action}`,
    type: "notification",
    action,
    timestamp: "2026-07-26T10:00:00Z",
    payload,
  };
}

describe("agent runtime update websocket handlers", () => {
  it("upserts update snapshots and appends each output chunk once", () => {
    const store = makeStore();
    const handlers = registerAgentsHandlers(store) as unknown as Record<
      string,
      (event: ReturnType<typeof message>) => void
    >;
    const snapshot = {
      job_id: "update-1",
      agent_name: "claude-acp",
      status: "updating",
      current_version: "1.0.0",
      target_version: "1.1.0",
      started_at: "2026-07-26T10:00:00Z",
    };

    handlers["agent.update.started"](message("agent.update.started", snapshot));
    handlers["agent.update.output"](
      message("agent.update.output", {
        job_id: "update-1",
        agent_name: "claude-acp",
        chunk: "downloaded\n",
      }),
    );
    handlers["agent.update.finished"](
      message("agent.update.finished", {
        ...snapshot,
        status: "succeeded",
        output: "downloaded\n",
      }),
    );

    expect(
      (
        store.getState() as unknown as {
          updateJobs: { byAgent: Record<string, { status: string; output: string }> };
        }
      ).updateJobs.byAgent["claude-acp"],
    ).toMatchObject({ status: "succeeded", output: "downloaded\n" });
  });
});
