import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://backend.test" }),
}));

const reloadMocks = vi.hoisted(() => ({
  signalBackendReloadRequired: vi.fn(),
}));

vi.mock("@/lib/platform/backend-reload-coordinator", () => ({
  signalBackendReloadRequired: (...args: unknown[]) =>
    reloadMocks.signalBackendReloadRequired(...args),
}));

import { deleteAgentProfileAction } from "./agents";

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
});
