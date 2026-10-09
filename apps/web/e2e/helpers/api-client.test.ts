import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient, removeRoutingProfileReferences } from "./api-client";
import { loadInterimSettingsInterlockToken } from "./interim-settings-interlock";

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
          Connection: "close",
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

describe("ApiClient.deleteTask", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("refreshes the preview when the task changes before deletion", async () => {
    let preflightCount = 0;
    let deleteCount = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.endsWith("/api/v1/app-state?path=%2Fsettings%2Fagents")) {
          return Response.json({ interimSettingsInterlockToken: "test-token" });
        }
        if (url.endsWith("/api/v1/tasks/delete-preflight")) {
          preflightCount += 1;
          return Response.json({ confirmation_id: `confirmation-${preflightCount}` });
        }
        if (url.endsWith("/api/v1/tasks/task-1")) {
          deleteCount += 1;
          if (deleteCount === 1) {
            return Response.json(
              { error: "task deletion preview is no longer current" },
              { status: 409 },
            );
          }
          expect(init?.headers).toMatchObject({
            "X-Kandev-Task-Delete-Confirmation": "confirmation-2",
          });
          return Response.json({ success: true });
        }
        throw new Error(`unexpected request: ${url}`);
      }),
    );

    await expect(
      new ApiClient("http://backend.test").deleteTask("task-1"),
    ).resolves.toBeUndefined();

    expect(preflightCount).toBe(2);
    expect(deleteCount).toBe(2);
  });

  it("does not retry deletion for an unrelated conflict", async () => {
    let preflightCount = 0;
    let deleteCount = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.endsWith("/api/v1/app-state?path=%2Fsettings%2Fagents")) {
          return Response.json({ interimSettingsInterlockToken: "test-token" });
        }
        if (url.endsWith("/api/v1/tasks/delete-preflight")) {
          preflightCount += 1;
          return Response.json({ confirmation_id: "confirmation-1" });
        }
        if (url.endsWith("/api/v1/tasks/task-1")) {
          deleteCount += 1;
          return Response.json({ error: "task cannot be deleted" }, { status: 409 });
        }
        throw new Error(`unexpected request: ${url}`);
      }),
    );

    await expect(new ApiClient("http://backend.test").deleteTask("task-1")).rejects.toThrow(
      'API DELETE /api/v1/tasks/task-1 failed (409): {"error":"task cannot be deleted"}',
    );

    expect(preflightCount).toBe(1);
    expect(deleteCount).toBe(1);
  });
});

describe("loadInterimSettingsInterlockToken", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("retries the backend startup response before reading the token", async () => {
    let attempts = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        attempts += 1;
        if (attempts < 3) return new Response(null, { status: 503 });
        return Response.json({ interimSettingsInterlockToken: "ready-token" });
      }),
    );

    await expect(loadInterimSettingsInterlockToken("http://backend.test")).resolves.toBe(
      "ready-token",
    );
    expect(attempts).toBe(3);
  });
});

describe("removeRoutingProfileReferences", () => {
  it("removes role-tier overrides whose last execution profile was deleted", () => {
    const updated = removeRoutingProfileReferences(
      {
        enabled: true,
        provider_order: ["claude-acp", "codex-acp"],
        default_tier: "balanced",
        provider_profiles: {
          "claude-acp": {
            execution_profile_ids: { balanced: "profile-1", economy: "economy-1" },
            tier_map: { balanced: "balanced-model", economy: "economy-model" },
          },
          "codex-acp": {
            execution_profile_ids: { balanced: "profile-2" },
          },
        },
        role_tiers: { ceo: "balanced", worker: "economy" },
      },
      "profile-1",
    );

    expect(updated?.role_tiers).toEqual({ ceo: "balanced", worker: "economy" });
    expect(updated?.provider_profiles["claude-acp"]).toEqual({
      execution_profile_ids: { economy: "economy-1" },
      tier_map: { economy: "economy-model" },
    });
  });

  it("drops a role-tier override when no provider still maps that tier", () => {
    const updated = removeRoutingProfileReferences(
      {
        enabled: true,
        provider_order: ["claude-acp"],
        default_tier: "balanced",
        provider_profiles: {
          "claude-acp": {
            execution_profile_ids: { balanced: "profile-1" },
          },
        },
        role_tiers: { ceo: "balanced" },
      },
      "profile-1",
    );

    expect(updated?.role_tiers).toEqual({});
  });
});

describe("ApiClient.cleanupTestProfiles", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("preserves Office-owned and seed profiles while deleting test profiles", async () => {
    const deleted: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = new URL(String(input));
        if (url.pathname === "/api/v1/app-state") {
          return Response.json({ interimSettingsInterlockToken: "test-token" });
        }
        if (url.pathname === "/api/v1/agents") {
          return Response.json({
            agents: [
              {
                id: "mock-agent",
                name: "Mock Agent",
                profiles: [
                  { id: "office-snake", workspace_id: "office-workspace" },
                  { id: "office-camel", workspaceId: "office-workspace" },
                  { id: "seed" },
                  { id: "test-profile" },
                ],
              },
            ],
            total: 1,
          });
        }
        if (init?.method === "DELETE") {
          deleted.push(url.pathname);
          return Response.json({ success: true });
        }
        throw new Error(`unexpected request: ${url}`);
      }),
    );

    await new ApiClient("http://backend.test").cleanupTestProfiles(["seed"]);
    expect(deleted).toEqual(["/api/v1/agent-profiles/test-profile"]);
  });
});

describe("ApiClient.e2eReset", () => {
  afterEach(() => vi.restoreAllMocks());

  it("repeats an idempotent reset while a session transfer owns cleanup", async () => {
    const client = new ApiClient("http://backend.test");
    const request = vi
      .spyOn(client, "rawRequest")
      .mockResolvedValueOnce(
        Response.json({ error: "session transfer in progress" }, { status: 500 }),
      )
      .mockResolvedValueOnce(Response.json({ success: true }));
    await expect(client.e2eReset("workspace-1", ["workflow-1"])).resolves.toBeUndefined();
    expect(request).toHaveBeenCalledTimes(2);
    expect(request).toHaveBeenLastCalledWith(
      "DELETE",
      "/api/v1/e2e/reset/workspace-1?keep_workflows=workflow-1",
    );
  });

  it("does not repeat reset for an unrelated server failure", async () => {
    const client = new ApiClient("http://backend.test");
    const request = vi
      .spyOn(client, "rawRequest")
      .mockResolvedValue(Response.json({ error: "database unavailable" }, { status: 500 }));
    await expect(client.e2eReset("workspace-1")).rejects.toThrow("database unavailable");
    expect(request).toHaveBeenCalledTimes(1);
  });

  it("keeps the existing four-attempt reset bound for a persistent transfer", async () => {
    const client = new ApiClient("http://backend.test");
    const request = vi
      .spyOn(client, "rawRequest")
      .mockImplementation(async () =>
        Response.json({ error: "session transfer in progress" }, { status: 500 }),
      );
    await expect(client.e2eReset("workspace-1")).rejects.toThrow("session transfer in progress");
    expect(request).toHaveBeenCalledTimes(4);
  });
});
