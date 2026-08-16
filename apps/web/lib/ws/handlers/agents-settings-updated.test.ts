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

const TIMESTAMP = "2026-08-13T10:00:00Z";
const SETTINGS_UPDATED = "agent.settings.updated";

function message(action: string, payload: unknown) {
  return { id: `message-${action}`, type: "notification", action, timestamp: TIMESTAMP, payload };
}

function savedAgent(overrides: Record<string, unknown> = {}) {
  return {
    id: "agent-1",
    name: "fuel-claude",
    supports_mcp: false,
    tui_config: {
      command: "fuelclaude",
      display_name: "Fuel Claude",
      wait_for_terminal: true,
    },
    profiles: [{ id: "p1", name: "Default" }],
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
    ...overrides,
  };
}

/**
 * These pin the split between two actions that were briefly conflated. The
 * settings record was first broadcast on `agent.updated`, whose handler expects
 * `{agentId, status}` — it read `undefined` and pushed `{id: undefined}` into
 * the runtime-status list instead of updating settings.
 */
describe("agent.settings.updated handler", () => {
  it("replaces the agent's settings record in settingsAgents", () => {
    const store = makeStore();
    store.getState().setSettingsAgents([savedAgent()] as never);
    const handlers = registerAgentsHandlers(store);

    handlers[SETTINGS_UPDATED]?.(
      message(SETTINGS_UPDATED, {
        agent: savedAgent({
          supports_mcp: true,
          tui_config: {
            command: "fuelclaude",
            display_name: "Fuel Claude",
            wait_for_terminal: true,
            mcp_strategy: "claude",
          },
        }),
      }) as never,
    );

    const [updated] = store.getState().settingsAgents.items;
    expect(updated.supports_mcp).toBe(true);
    expect(updated.tui_config?.mcp_strategy).toBe("claude");
  });

  it("preserves locally normalized profiles", () => {
    const store = makeStore();
    store.getState().setSettingsAgents([savedAgent()] as never);
    const handlers = registerAgentsHandlers(store);

    // The event carries agent-level settings; profiles have their own events,
    // so an agent record arriving without them must not wipe the list.
    handlers[SETTINGS_UPDATED]?.(
      message(SETTINGS_UPDATED, {
        agent: savedAgent({ supports_mcp: true, profiles: [] }),
      }) as never,
    );

    expect(store.getState().settingsAgents.items[0].profiles).toHaveLength(1);
  });

  it("ignores an agent it does not already track", () => {
    const store = makeStore();
    store.getState().setSettingsAgents([savedAgent()] as never);
    const handlers = registerAgentsHandlers(store);

    handlers[SETTINGS_UPDATED]?.(
      message(SETTINGS_UPDATED, { agent: savedAgent({ id: "agent-other" }) }) as never,
    );

    expect(store.getState().settingsAgents.items).toHaveLength(1);
    expect(store.getState().settingsAgents.items[0].id).toBe("agent-1");
  });

  it("ignores a payload with no agent rather than corrupting the list", () => {
    const store = makeStore();
    store.getState().setSettingsAgents([savedAgent()] as never);
    const handlers = registerAgentsHandlers(store);

    handlers[SETTINGS_UPDATED]?.(message(SETTINGS_UPDATED, {}) as never);
    handlers[SETTINGS_UPDATED]?.(message(SETTINGS_UPDATED, { agent: {} }) as never);

    expect(store.getState().settingsAgents.items).toHaveLength(1);
    expect(store.getState().settingsAgents.items[0].id).toBe("agent-1");
  });
});
