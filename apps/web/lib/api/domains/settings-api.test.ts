import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import {
  fetchMessageQueueSettings,
  fetchSleepInhibitionSettings,
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
        settings: { max_per_session: 10 },
        effective: { max_per_session: 10, source: "default", locked: false },
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
        settings: { max_per_session: 25 },
        effective: { max_per_session: 25, source: "setting", locked: false },
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
