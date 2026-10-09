import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createSettingsSlice } from "@/lib/state/slices/settings/settings-slice";
import type { SettingsSlice } from "@/lib/state/slices/settings/types";
import type { AgentUpdateStatus } from "@/lib/api/domains/agent-update-api";

const fetchStatuses = vi.fn();
vi.mock("@/lib/api/domains/agent-update-api", () => ({
  listAgentUpdateStatuses: (...args: unknown[]) => fetchStatuses(...args),
}));
import { refreshRuntimeUpdateStatuses } from "./runtime-update-statuses";

function store() {
  return create<SettingsSlice>()(immer((set, get, api) => createSettingsSlice(set, get, api)));
}
function status(enabled = false): AgentUpdateStatus {
  return {
    agent_name: "gemini",
    display_name: "Gemini",
    runtime_id: "npm:@google/gemini-cli",
    owner: "kandev",
    mechanism: "npm_candidate",
    management: "managed",
    source: "@google/gemini-cli",
    guidance_url: "",
    current_version: "1.0.0",
    available: true,
    enabled: true,
    auto_update_supported: true,
    auto_update: enabled,
    package: "@google/gemini-cli",
    default_version: "1.0.0",
    effective_version: "1.0.0",
    latest_version: "2.0.0",
    check_state: "update_available",
  };
}
function deferred() {
  let resolve!: (v: { statuses: AgentUpdateStatus[] }) => void;
  const promise = new Promise<{ statuses: AgentUpdateStatus[] }>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}
afterEach(() => vi.clearAllMocks());

describe("shared runtime discovery", () => {
  // @covers AC-AGENTS-RUNTIME-NOTIFY-001.2
  it("shares a lookup, settles without a component owner, and preserves data for a second consumer", async () => {
    const cache = store();
    const request = deferred();
    fetchStatuses.mockReturnValueOnce(request.promise);
    const first = refreshRuntimeUpdateStatuses(cache);
    const second = refreshRuntimeUpdateStatuses(cache);
    expect(fetchStatuses).toHaveBeenCalledTimes(1);
    expect(cache.getState().agentRuntimeUpdates.loading).toBe(true);
    request.resolve({ statuses: [status()] });
    await Promise.all([first, second]);
    expect(cache.getState().agentRuntimeUpdates.byAgent.gemini).toEqual(status());
    expect(cache.getState().agentRuntimeUpdates.loading).toBe(false);
    await refreshRuntimeUpdateStatuses(cache);
    expect(fetchStatuses).toHaveBeenCalledTimes(1);
  });
  it("serializes a forced refresh after an older response and isolates separate stores", async () => {
    const cache = store();
    const old = deferred();
    fetchStatuses
      .mockReturnValueOnce(old.promise)
      .mockResolvedValueOnce({ statuses: [status(true)] });
    const first = refreshRuntimeUpdateStatuses(cache);
    const newer = refreshRuntimeUpdateStatuses(cache, true);
    old.resolve({ statuses: [status(false)] });
    await Promise.all([first, newer]);
    expect(cache.getState().agentRuntimeUpdates.byAgent.gemini.auto_update).toBe(true);
    const other = store();
    fetchStatuses.mockResolvedValueOnce({ statuses: [] });
    await refreshRuntimeUpdateStatuses(other);
    expect(other.getState().agentRuntimeUpdates.byAgent).toEqual({});
    expect(cache.getState().agentRuntimeUpdates.byAgent.gemini.auto_update).toBe(true);
  });
  it("keeps the last good snapshot and clears loading after an offline failure", async () => {
    const cache = store();
    fetchStatuses
      .mockResolvedValueOnce({ statuses: [status()] })
      .mockRejectedValueOnce(new Error("offline"));
    await refreshRuntimeUpdateStatuses(cache);
    expect(await refreshRuntimeUpdateStatuses(cache, true)).toBe(false);
    expect(cache.getState().agentRuntimeUpdates.byAgent.gemini).toEqual(status());
    expect(cache.getState().agentRuntimeUpdates.loading).toBe(false);
  });
});
