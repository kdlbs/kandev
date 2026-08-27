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

const TIMESTAMP = "2026-07-26T10:00:00Z";
const NEWER_TIMESTAMP = "2026-07-26T11:00:00Z";
const NOTIFICATION_TYPE = "notification";
const PROFILE_CREATED = "agent.profile.created";
const PROFILE_UPDATED = "agent.profile.updated";
const PROFILE_DELETED = "agent.profile.deleted";
const AVAILABLE_UPDATED = "agent.available.updated";
const AGENT_NAME = "claude-acp";
const NOT_INSTALLED = "not_installed";
const AGENT_NOT_INSTALLED_ERROR = "agent not installed";

/** Builds a WS notification message fixture with the given action, payload, and timestamp. */
function message(action: string, payload: unknown, timestamp = TIMESTAMP) {
  return {
    id: `message-${action}`,
    type: NOTIFICATION_TYPE,
    action,
    timestamp,
    payload,
  };
}

/** Builds a kanban profile payload fixture, overridable per test. */
function profilePayload(overrides: Record<string, unknown> = {}) {
  return {
    id: "p1",
    agent_id: "agent-1",
    name: "Default",
    model: "mock-fast",
    enabled: true,
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
    ...overrides,
  };
}

/** Builds an AvailableAgent fixture (agent.available.updated payload entry), overridable per test. */
function availableAgentPayload(overrides: Record<string, unknown> = {}) {
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
      status: "ok",
    },
    updated_at: TIMESTAMP,
    ...overrides,
  };
}

/** Builds an office-scoped profile payload fixture (workspace_id set). */
function officeProfilePayload() {
  return {
    id: "profile-office",
    agent_id: "agent-1",
    name: "Office Agent",
    workspace_id: "ws-1",
    enabled: true,
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
  };
}

/** Registers the agents WS handlers and returns them keyed by action. */
function handlersFor(store: ReturnType<typeof makeStore>) {
  return registerAgentsHandlers(store) as unknown as Record<
    string,
    (event: ReturnType<typeof message>) => void
  >;
}

/** Builds a settingsAgents Agent fixture, overridable per test. */
function settingsAgentPayload(overrides: Record<string, unknown> = {}) {
  return {
    id: "agent-1",
    name: AGENT_NAME,
    supports_mcp: false,
    profiles: [],
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
    ...overrides,
  };
}

/** Builds an AvailableAgent payload reporting the agent as not installed. */
function notInstalledAvailableAgentPayload(error = AGENT_NOT_INSTALLED_ERROR) {
  return availableAgentPayload({
    model_config: {
      default_model: "default",
      available_models: [],
      supports_dynamic_models: false,
      status: NOT_INSTALLED,
      error,
    },
  });
}

describe("agent update job websocket handlers", () => {
  it("upserts update snapshots and appends each output chunk once", () => {
    const store = makeStore();
    const handlers = handlersFor(store);
    const snapshot = {
      job_id: "update-1",
      agent_name: AGENT_NAME,
      status: "updating",
      current_version: "1.0.0",
      target_version: "1.1.0",
      started_at: TIMESTAMP,
    };

    handlers["agent.update.started"](message("agent.update.started", snapshot));
    handlers["agent.update.output"](
      message("agent.update.output", {
        job_id: "update-1",
        agent_name: AGENT_NAME,
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
      ).updateJobs.byAgent[AGENT_NAME],
    ).toMatchObject({ status: "succeeded", output: "downloaded\n" });
  });
});

describe("agent profile events", () => {
  it("ignores office-scoped profile events so they never leak into global settings", () => {
    const store = makeStore();
    const handlers = handlersFor(store);
    // The backend wraps the profile under { profile: ... } in the event.
    const payload = { profile: officeProfilePayload() };

    handlers[PROFILE_CREATED](message(PROFILE_CREATED, payload));
    handlers[PROFILE_UPDATED](message(PROFILE_UPDATED, payload));

    const state = store.getState();
    expect(state.agentProfiles.items).toHaveLength(0);
    expect(state.settingsAgents.items).toHaveLength(0);
  });

  it("does not let a delayed profile event overwrite newer stored state", () => {
    const store = makeStore();
    const handlers = handlersFor(store);
    const newer = profilePayload({ model: "new-model", updated_at: NEWER_TIMESTAMP });
    handlers[PROFILE_CREATED](message(PROFILE_CREATED, { profile: newer }, "2026-07-26T11:05:00Z"));
    // A delayed older event (e.g. a replayed duplicate response) must not
    // regress the newer revision.
    const older = profilePayload({ model: "stale-model", updated_at: "2026-07-26T09:00:00Z" });
    handlers[PROFILE_UPDATED](message(PROFILE_UPDATED, { profile: older }, "2026-07-26T11:04:00Z"));

    const stored = store.getState().agentProfiles.items[0];
    expect(stored?.model).toBe("new-model");
  });

  it("does not resurrect a deleted profile from a delayed create event", () => {
    const store = makeStore();
    const handlers = handlersFor(store);

    handlers[PROFILE_CREATED](message(PROFILE_CREATED, { profile: profilePayload() }, TIMESTAMP));
    expect(store.getState().agentProfiles.items).toHaveLength(1);

    handlers[PROFILE_DELETED](
      message(PROFILE_DELETED, { profile: profilePayload() }, "2026-07-26T12:00:00Z"),
    );
    expect(store.getState().agentProfiles.items).toHaveLength(0);

    // A create event produced BEFORE the deletion is stale: no resurrection.
    handlers[PROFILE_CREATED](message(PROFILE_CREATED, { profile: profilePayload() }, TIMESTAMP));
    expect(store.getState().agentProfiles.items).toHaveLength(0);

    // A genuinely newer create (after the deletion) wins and clears the
    // tombstone.
    handlers[PROFILE_CREATED](
      message(
        "agent.profile.created",
        { profile: profilePayload({ updated_at: "2026-07-26T13:00:00Z" }) },
        "2026-07-26T13:00:00Z",
      ),
    );
    expect(store.getState().agentProfiles.items).toHaveLength(1);
  });

  it("preserves capability_status on the rebuilt option when a profile update arrives", () => {
    const store = makeStore();
    const handlers = handlersFor(store);
    store.setState((state) => ({
      ...state,
      settingsAgents: {
        items: [
          settingsAgentPayload({
            capability_status: NOT_INSTALLED,
            capability_error: AGENT_NOT_INSTALLED_ERROR,
          }),
        ],
      },
    }));

    handlers[PROFILE_CREATED](message(PROFILE_CREATED, { profile: profilePayload() }, TIMESTAMP));
    expect(store.getState().agentProfiles.items[0]?.capability_status).toBe(NOT_INSTALLED);

    const updatedAt = "2026-07-26T14:00:00Z";
    handlers[PROFILE_UPDATED](
      message(
        PROFILE_UPDATED,
        { profile: profilePayload({ model: "mock-slow", updated_at: updatedAt }) },
        updatedAt,
      ),
    );

    const updated = store.getState().agentProfiles.items[0];
    expect(updated?.model).toBe("mock-slow");
    expect(updated?.capability_status).toBe(NOT_INSTALLED);
    expect(updated?.capability_error).toBe(AGENT_NOT_INSTALLED_ERROR);
  });

  it("ignores office-scoped delete events", () => {
    const store = makeStore();
    const handlers = handlersFor(store);
    handlers[PROFILE_DELETED](message(PROFILE_DELETED, { profile: officeProfilePayload() }));

    expect(store.getState().agentProfiles.items).toHaveLength(0);
    expect(store.getState().settingsAgents.items).toHaveLength(0);
  });
});

describe("agent.available.updated capability refresh", () => {
  it("does not let an older websocket snapshot clobber a newer capability snapshot", () => {
    const store = makeStore();
    const handlers = handlersFor(store);

    handlers[AVAILABLE_UPDATED](
      message(AVAILABLE_UPDATED, {
        agents: [
          availableAgentPayload({
            model_config: {
              default_model: "default",
              available_models: [],
              supports_dynamic_models: false,
              status: "ok",
            },
            updated_at: NEWER_TIMESTAMP,
          }),
        ],
      }),
    );
    handlers[AVAILABLE_UPDATED](
      message(AVAILABLE_UPDATED, {
        agents: [notInstalledAvailableAgentPayload()],
      }),
    );

    expect(store.getState().availableAgents.items[0]?.model_config.status).toBe("ok");
  });

  it("refreshes capability_status on matching profiles when agent.available.updated reports the agent is ready", () => {
    const store = makeStore();
    const handlers = handlersFor(store);
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
            capability_status: NOT_INSTALLED,
            capability_error: AGENT_NOT_INSTALLED_ERROR,
          },
        ],
      },
    }));

    handlers[AVAILABLE_UPDATED](message(AVAILABLE_UPDATED, { agents: [availableAgentPayload()] }));

    const updated = store.getState().agentProfiles.items[0];
    expect(updated?.capability_status).toBe("ok");
    expect(updated?.capability_error).toBeUndefined();
  });

  it("maps the not_configured cache-miss sentinel back to undefined so the profile stays healthy", () => {
    const store = makeStore();
    const handlers = handlersFor(store);
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
          },
        ],
      },
    }));

    handlers[AVAILABLE_UPDATED](
      message(AVAILABLE_UPDATED, {
        agents: [
          availableAgentPayload({
            model_config: {
              default_model: "default",
              available_models: [],
              supports_dynamic_models: false,
              status: "not_configured",
              error: "no host-utility entry",
            },
          }),
        ],
      }),
    );

    const updated = store.getState().agentProfiles.items[0];
    expect(updated?.capability_status).toBeUndefined();
    expect(updated?.capability_error).toBeUndefined();
  });
});

/** Seeds settingsAgents with a single unhealthy AGENT_NAME entry. */
function seedUnhealthySettingsAgent(store: ReturnType<typeof makeStore>) {
  store.setState((state) => ({
    ...state,
    settingsAgents: {
      items: [
        settingsAgentPayload({
          capability_status: NOT_INSTALLED,
          capability_error: AGENT_NOT_INSTALLED_ERROR,
        }),
      ],
    },
  }));
}

describe("agent.available.updated keeps settingsAgents in sync so later profile events don't revert capability", () => {
  it("install then edit: a profile made healthy by available.updated stays healthy after a profile.updated event", () => {
    const store = makeStore();
    const handlers = handlersFor(store);
    seedUnhealthySettingsAgent(store);

    // The profile already exists (created while the agent was unhealthy).
    handlers[PROFILE_CREATED](message(PROFILE_CREATED, { profile: profilePayload() }, TIMESTAMP));
    expect(store.getState().agentProfiles.items[0]?.capability_status).toBe(NOT_INSTALLED);

    // The agent was just installed: a fresh available.updated snapshot
    // reports it healthy.
    handlers[AVAILABLE_UPDATED](message(AVAILABLE_UPDATED, { agents: [availableAgentPayload()] }));

    // The user edits the profile in Settings; the backend broadcasts the
    // update. Before the fix, handleProfileUpdated rebuilt the option from
    // the still-stale settingsAgents entry and reverted it to unhealthy.
    const updatedAt = "2026-07-26T15:00:00Z";
    handlers[PROFILE_UPDATED](
      message(
        PROFILE_UPDATED,
        { profile: profilePayload({ model: "mock-slow", updated_at: updatedAt }) },
        updatedAt,
      ),
    );

    const updated = store.getState().agentProfiles.items[0];
    expect(updated?.capability_status).toBe("ok");
    expect(updated?.capability_error).toBeUndefined();
  });

  it("break then edit: a profile made unhealthy by available.updated stays unhealthy after a profile.updated event", () => {
    const store = makeStore();
    const handlers = handlersFor(store);
    store.setState((state) => ({
      ...state,
      settingsAgents: { items: [settingsAgentPayload()] },
    }));

    // The profile already exists (created while the agent was healthy).
    handlers[PROFILE_CREATED](message(PROFILE_CREATED, { profile: profilePayload() }, TIMESTAMP));
    expect(store.getState().agentProfiles.items[0]?.capability_status).toBeUndefined();

    // The agent breaks after boot (uninstalled/deauthed): available.updated
    // reports it not_installed.
    handlers[AVAILABLE_UPDATED](
      message(AVAILABLE_UPDATED, {
        agents: [notInstalledAvailableAgentPayload("agent CLI not found")],
      }),
    );

    // Any subsequent profile edit must not resurrect it into Handoff. Before
    // the fix, this rebuilt the option from the still-stale (healthy)
    // settingsAgents entry and re-listed the uninstalled provider.
    const updatedAt = "2026-07-26T15:00:00Z";
    handlers[PROFILE_UPDATED](
      message(
        PROFILE_UPDATED,
        { profile: profilePayload({ model: "mock-slow", updated_at: updatedAt }) },
        updatedAt,
      ),
    );

    const updated = store.getState().agentProfiles.items[0];
    expect(updated?.capability_status).toBe(NOT_INSTALLED);
    expect(updated?.capability_error).toBe("agent CLI not found");
  });

  it("does not touch settingsAgents entries for agents the snapshot has no match for", () => {
    const store = makeStore();
    const handlers = handlersFor(store);
    store.setState((state) => ({
      ...state,
      settingsAgents: {
        items: [
          settingsAgentPayload({ id: "agent-other", name: "other-agent", capability_status: "ok" }),
        ],
      },
    }));

    handlers[AVAILABLE_UPDATED](message(AVAILABLE_UPDATED, { agents: [availableAgentPayload()] }));

    expect(store.getState().settingsAgents.items[0]?.capability_status).toBe("ok");
  });
});

describe("agent profile event ordering", () => {
  it("ignores a stale delete event when a newer revision already exists", () => {
    const store = makeStore();
    const handlers = handlersFor(store);
    const current = profilePayload({
      id: "p-stale",
      model: "new-model",
      updated_at: NEWER_TIMESTAMP,
    });
    handlers[PROFILE_CREATED](
      message(PROFILE_CREATED, { profile: current }, "2026-07-26T11:05:00Z"),
    );
    expect(store.getState().agentProfiles.items).toHaveLength(1);

    // A delayed delete carrying an OLDER snapshot than the stored revision
    // must not remove the current profile.
    const staleDelete = profilePayload({
      id: "p-stale",
      model: "new-model",
      updated_at: TIMESTAMP,
    });
    handlers[PROFILE_DELETED](
      message(PROFILE_DELETED, { profile: staleDelete }, "2026-07-26T11:02:00Z"),
    );

    const state = store.getState();
    expect(state.agentProfiles.items).toHaveLength(1);
    expect(state.agentProfiles.items[0]?.model).toBe("new-model");
    // A genuinely newer delete (after the stored revision) still applies.
    handlers[PROFILE_DELETED](
      message(PROFILE_DELETED, { profile: current }, "2026-07-26T12:00:00Z"),
    );
    expect(store.getState().agentProfiles.items).toHaveLength(0);
  });

  it("orders revisions by instant, not lexically, for fractional-second timestamps", () => {
    const store = makeStore();
    const handlers = handlersFor(store);
    // Lexically "…10:00:00Z" sorts AFTER "…10:00:00.100Z", so a naive string
    // compare would treat the whole-second revision as newer and regress it.
    const fractional = profilePayload({
      id: "p-frac",
      model: "fractional-model",
      updated_at: "2026-07-26T10:00:00.100Z",
    });
    handlers[PROFILE_CREATED](
      message(PROFILE_CREATED, { profile: fractional }, "2026-07-26T10:00:00.100Z"),
    );

    const wholeSecond = profilePayload({
      id: "p-frac",
      model: "whole-second-model",
      updated_at: TIMESTAMP,
    });
    handlers[PROFILE_UPDATED](message(PROFILE_UPDATED, { profile: wholeSecond }));

    expect(store.getState().agentProfiles.items[0]?.model).toBe("fractional-model");
  });

  it("keeps the tombstone ordered by instant for fractional-second deletes", () => {
    const store = makeStore();
    const handlers = handlersFor(store);
    handlers[PROFILE_CREATED](
      message(PROFILE_CREATED, { profile: profilePayload({ id: "p-tomb" }) }),
    );
    // Delete at a fractional second; the tombstone must beat a whole-second
    // create produced before it.
    handlers[PROFILE_DELETED](
      message(
        PROFILE_DELETED,
        { profile: profilePayload({ id: "p-tomb" }) },
        "2026-07-26T10:00:00.100Z",
      ),
    );
    expect(store.getState().agentProfiles.items).toHaveLength(0);

    handlers[PROFILE_CREATED](
      message(
        PROFILE_CREATED,
        { profile: profilePayload({ id: "p-tomb" }) },
        "2026-07-26T10:00:00.050Z",
      ),
    );
    expect(store.getState().agentProfiles.items).toHaveLength(0);
  });
});
