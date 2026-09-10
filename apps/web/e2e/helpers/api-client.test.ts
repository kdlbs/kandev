import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient } from "./api-client";

describe("ApiClient.createAgentProfile", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("forwards auto_approve while preserving the profile request contract", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/api/v1/app-state?path=%2Fsettings%2Fagents")) {
        return Response.json({ interimSettingsInterlockToken: "test-token" });
      }
      if (url.endsWith("/api/v1/agents/agent-1/profiles")) {
        const body = JSON.parse(String(init?.body)) as Record<string, unknown>;
        expect(body).toMatchObject({
          name: "TUI profile",
          model: "mock-fast",
          auto_approve: true,
          cli_passthrough: true,
        });
        expect(init?.headers).toMatchObject({
          "Content-Type": "application/json",
          "X-Kandev-Interim-Settings-Interlock": "test-token",
        });
        return Response.json({
          id: "profile-1",
          name: "TUI profile",
          agent_id: "agent-1",
          model: "mock-fast",
          auto_approve: true,
          cli_passthrough: true,
        });
      }
      throw new Error(`unexpected request: ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    const profile = await new ApiClient("http://backend.test").createAgentProfile(
      "agent-1",
      "TUI profile",
      { model: "mock-fast", auto_approve: true, cli_passthrough: true },
    );

    expect(profile.id).toBe("profile-1");
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});

describe("ApiClient user settings", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  // Contract coverage for the existing settings restore flow.
  it("round-trips the startup choice and workspace scope", async () => {
    const baseline = {
      startup_page: "threads",
      workspace_id: "workspace-1",
      workflow_filter_id: "workflow-1",
    };
    let saved: unknown;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.endsWith("/api/v1/app-state?path=%2Fsettings%2Fagents")) {
          return Response.json({ interimSettingsInterlockToken: "test-token" });
        }
        if (url.endsWith("/api/v1/user/settings")) {
          if (init?.method === "PATCH") saved = JSON.parse(String(init.body));
          return Response.json({ settings: baseline });
        }
        throw new Error(`unexpected request: ${url}`);
      }),
    );
    const client = new ApiClient("http://backend.test");
    const { settings } = await client.getUserSettings();
    await client.saveUserSettings({
      startup_page: settings.startup_page,
      workspace_id: settings.workspace_id,
      workflow_filter_id: settings.workflow_filter_id,
    });
    expect(saved).toEqual(baseline);
  });
});
