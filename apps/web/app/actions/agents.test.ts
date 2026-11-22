import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://backend.test" }),
}));

const reloadMocks = vi.hoisted(() => ({
  signalBackendReloadRequired: vi.fn(),
}));

const translationMocks = vi.hoisted(() => ({
  t: vi.fn(() => "localized correction message"),
}));

vi.mock("@/lib/platform/backend-reload-coordinator", () => ({
  signalBackendReloadRequired: (...args: unknown[]) =>
    reloadMocks.signalBackendReloadRequired(...args),
}));

vi.mock("@/lib/i18n", () => ({
  t: translationMocks.t,
}));

import {
  createAgentProfileAction,
  deleteAgentProfileAction,
  reorderAgentProfilesAction,
} from "./agents";

const bootWindow = window as unknown as { __KANDEV_BOOT_PAYLOAD__?: unknown };

beforeEach(() => {
  bootWindow.__KANDEV_BOOT_PAYLOAD__ = {
    interimSettingsInterlockToken: "boot-token",
  };
  reloadMocks.signalBackendReloadRequired.mockReset();
});

afterEach(() => {
  vi.unstubAllGlobals();
  delete bootWindow.__KANDEV_BOOT_PAYLOAD__;
});

describe("deleteAgentProfileAction", () => {
  it("returns a handled result for the stale interlock response", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          error: "interim settings interlock required",
          error_code: "interim_settings_interlock_required",
        }),
        { status: 403, headers: { "Content-Type": "application/json" } },
      ),
    );
    vi.stubGlobal("fetch", fetcher);

    await expect(deleteAgentProfileAction("profile-1")).resolves.toEqual({
      status: "error",
      message: "interim settings interlock required",
      handled: true,
    });
    expect(reloadMocks.signalBackendReloadRequired).toHaveBeenCalledWith(
      "settings_interlock_rejected",
    );
    expect(
      new Headers(fetcher.mock.calls[0]?.[1]?.headers).get("X-Kandev-Interim-Settings-Interlock"),
    ).toBe("boot-token");
  });

  it("keeps the typed conflict result for profile deletion", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            active_sessions: [{ id: "session-1" }],
            watchers: [],
            routing_tiers: [],
            automations: [],
            utility_agents: [],
          }),
          { status: 409, headers: { "Content-Type": "application/json" } },
        ),
      ),
    );

    await expect(deleteAgentProfileAction("profile-1")).resolves.toMatchObject({
      status: "conflict",
      activeSessions: [{ id: "session-1" }],
    });
    expect(reloadMocks.signalBackendReloadRequired).not.toHaveBeenCalled();
  });

  it("keeps a dynamic-profile-only 409 as a conflict result", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            error: "agent profile is used by 1 dynamic profile(s)",
            dynamic_profiles: [{ id: "dynamic-1", name: "Live selector" }],
          }),
          { status: 409, headers: { "Content-Type": "application/json" } },
        ),
      ),
    );

    await expect(deleteAgentProfileAction("profile-1")).resolves.toEqual({
      status: "conflict",
      activeSessions: [],
      watchers: [],
      routingTiers: [],
      automations: [],
      utilityAgents: [],
      dynamicProfiles: [{ id: "dynamic-1", name: "Live selector" }],
    });
  });
});

describe("reorderAgentProfilesAction", () => {
  it("bounds the real fetch request with a fifteen-second abort signal", async () => {
    const controller = new AbortController();
    const timeout = vi.spyOn(AbortSignal, "timeout").mockReturnValue(controller.signal);
    const fetcher = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ agent_id: "agent", profile_ids: ["b", "a"], revision: 2 }), {
        status: 200,
      }),
    );
    vi.stubGlobal("fetch", fetcher);
    try {
      await reorderAgentProfilesAction("agent", ["b", "a"]);
      expect(timeout).toHaveBeenCalledWith(15_000);
      expect(fetcher.mock.calls[0][1].signal).toBe(controller.signal);
    } finally {
      timeout.mockRestore();
    }
  });
});

describe("agentSettingsRequest validation errors", () => {
  it("localizes the Claude alias conflict and preserves the API error evidence", async () => {
    const responseBody = {
      error: "backend validation message",
      error_code: "claude_exact_model_alias_conflict",
    };
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify(responseBody), {
          status: 400,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );

    await expect(
      createAgentProfileAction("agent-1", {
        name: "Default",
        model: "sonnet",
        auto_approve: false,
        allow_indexing: false,
        cli_passthrough: false,
        require_exact_model: true,
        env_vars: [{ key: "ANTHROPIC_MODEL", value: "claude-sonnet-5-5" }],
      }),
    ).rejects.toMatchObject({
      message: "localized correction message",
      status: 400,
      errorCode: "claude_exact_model_alias_conflict",
      body: responseBody,
    });
    expect(translationMocks.t).toHaveBeenCalledWith("agents:claudeExactModelAliasConflict", {
      envVar: "ANTHROPIC_MODEL",
    });
  });
});
