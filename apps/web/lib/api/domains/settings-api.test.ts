import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import {
  deleteExecutor,
  fetchMessageQueueSettings,
  fetchSleepInhibitionSettings,
  resolveAgentModelConfig,
  startHostShell,
  updateMessageQueueSettings,
  updateSleepInhibitionSettings,
} from "./settings-api";

const BASE = "http://api.test/api/v1/system/message-queue/settings";
type FetchInput = Parameters<typeof fetch>[0];
type FetchInit = Parameters<typeof fetch>[1];
const fetchSpy = vi.fn<(...args: [FetchInput, FetchInit?]) => Promise<Response>>();

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function lastCall(): { url: string; init: FetchInit | undefined } {
  const call = fetchSpy.mock.calls.at(-1);
  if (!call) throw new Error("expected fetch to have been called");
  return { url: String(call[0]), init: call[1] };
}

describe("message queue settings api", () => {
  it("fetches the current setting without cache", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        settings: { max_per_session: 10, merge_enabled: true, auto_merge_enabled: true },
        effective: {
          max_per_session: 10,
          source: "default",
          locked: false,
          merge_enabled: true,
          auto_merge_enabled: true,
        },
      }),
    );

    const response = await fetchMessageQueueSettings({ cache: "force-cache" });

    expect(lastCall().url).toBe(BASE);
    expect((lastCall().init?.method ?? "GET").toUpperCase()).toBe("GET");
    expect(lastCall().init?.cache).toBe("no-store");
    expect(response.effective.source).toBe("default");
  });

  it("PATCHes the requested per-session maximum", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        settings: { max_per_session: 25, merge_enabled: true, auto_merge_enabled: true },
        effective: {
          max_per_session: 25,
          source: "setting",
          locked: false,
          merge_enabled: true,
          auto_merge_enabled: true,
        },
      }),
    );

    await updateMessageQueueSettings(
      { max_per_session: 25 },
      { init: { method: "GET", body: JSON.stringify({ max_per_session: 2 }) } },
    );

    expect(lastCall().url).toBe(BASE);
    expect(lastCall().init?.method).toBe("PATCH");
    expect(lastCall().init?.body).toBe(JSON.stringify({ max_per_session: 25 }));
  });

  it("PATCHes automatic merge without changing other queue settings", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        settings: { max_per_session: 10, merge_enabled: true, auto_merge_enabled: false },
        effective: {
          max_per_session: 10,
          source: "default",
          locked: false,
          merge_enabled: true,
          auto_merge_enabled: false,
        },
      }),
    );

    await updateMessageQueueSettings({ auto_merge_enabled: false });

    expect(lastCall().init?.body).toBe(JSON.stringify({ auto_merge_enabled: false }));
  });
});

describe("executor settings api", () => {
  it("provides an executor delete client", async () => {
    const api = (await import("./settings-api")) as Record<string, unknown>;

    expect(typeof api.deleteExecutor).toBe("function");
  });

  it("DELETEs an encoded executor id", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ success: true }));

    const result = await deleteExecutor("executor one/primary", { init: { method: "GET" } });

    expect(lastCall().url).toBe("http://api.test/api/v1/executors/executor%20one%2Fprimary");
    expect(lastCall().init?.method).toBe("DELETE");
    expect(result).toEqual({ success: true });
  });
});

describe("sleep inhibition settings api", () => {
  it("fetches the install-wide setting without cache", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        settings: { enabled: false },
        status: { platform: "linux", supported: true, active: false },
      }),
    );

    const response = await fetchSleepInhibitionSettings();

    expect(lastCall().url).toBe("http://api.test/api/v1/system/sleep-inhibition");
    expect(lastCall().init?.cache).toBe("no-store");
    expect(response.settings.enabled).toBe(false);
  });

  it("PATCHes the configured enabled value", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        settings: { enabled: true },
        status: { platform: "linux", supported: true, active: true },
      }),
    );

    await updateSleepInhibitionSettings({ enabled: true });

    expect(lastCall().url).toBe("http://api.test/api/v1/system/sleep-inhibition");
    expect(lastCall().init?.method).toBe("PATCH");
    expect(lastCall().init?.body).toBe(JSON.stringify({ enabled: true }));
  });
});

describe("startHostShell", () => {
  it("serializes a client id alongside the terminal size", async () => {
    const clientId = "6f2d7f2d-0d0c-4c9b-8b73-1c53a5ed5b6b";
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        session_id: "session-1",
        agent_id: "_host_shell",
        cmd: ["/bin/bash"],
        running: true,
        started_at: "2026-08-04T00:00:00Z",
      }),
    );

    await startHostShell({ cols: 80, rows: 24 }, { clientId });

    expect(JSON.parse(String(lastCall().init?.body))).toEqual({
      cols: 80,
      rows: 24,
      client_id: clientId,
    });
  });

  it("preserves the legacy request shape without a client id", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({}));
    await startHostShell({ cols: 80, rows: 24 });

    const fetchCall = vi.mocked(fetch).mock.calls[0];
    expect(JSON.parse(String(fetchCall?.[1]?.body))).toEqual({ cols: 80, rows: 24 });
  });
});

describe("resolveAgentModelConfig", () => {
  it("posts the selected provider context", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        agent_name: "opencode",
        model: "opencode/gpt-5.6",
        status: "ok",
        config_options: [],
        error: null,
      }),
    );

    await resolveAgentModelConfig("opencode", {
      model: "opencode/gpt-5.6",
      mode: "build",
      config_options: { reasoning_effort: "high" },
    });

    expect(lastCall().url).toBe("http://api.test/api/v1/agent-models/opencode/resolve");
    expect(lastCall().init?.method).toBe("POST");
    expect(JSON.parse(String(lastCall().init?.body))).toEqual({
      model: "opencode/gpt-5.6",
      mode: "build",
      config_options: { reasoning_effort: "high" },
    });
  });
});
