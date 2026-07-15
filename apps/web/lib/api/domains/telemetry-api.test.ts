import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Pin the backend config to a deterministic base so URL assertions don't
// depend on whatever environment the tests inherit.
vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import {
  fetchTelemetryConsent,
  sendTelemetryEvents,
  updateTelemetryConsent,
} from "./telemetry-api";

const CONSENT_URL = "http://api.test/api/v1/telemetry/consent";
const EVENTS_URL = "http://api.test/api/v1/telemetry/events";

type FetchInput = Parameters<typeof fetch>[0];
type FetchInit = Parameters<typeof fetch>[1];

const fetchSpy = vi.fn<(...args: [FetchInput, FetchInit?]) => Promise<Response>>();

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function jsonResponse(body: unknown, init?: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
    ...init,
  });
}

describe("fetchTelemetryConsent", () => {
  it("GETs the consent endpoint", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ status: "unasked", env_disabled: false }));
    const state = await fetchTelemetryConsent();
    expect(state.status).toBe("unasked");
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(CONSENT_URL);
    expect(init?.method).toBeUndefined();
  });
});

describe("updateTelemetryConsent", () => {
  it("PUTs the new status", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({ status: "granted", install_id: "abc", env_disabled: false }),
    );
    const state = await updateTelemetryConsent("granted");
    expect(state.install_id).toBe("abc");
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(CONSENT_URL);
    expect(init?.method).toBe("PUT");
    expect(JSON.parse(String(init?.body))).toEqual({ status: "granted" });
  });
});

describe("sendTelemetryEvents", () => {
  it("POSTs the event batch", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ accepted: 1 }, { status: 202 }));
    const res = await sendTelemetryEvents([
      { name: "ui_page_viewed", properties: { page: "settings.system.telemetry" } },
    ]);
    expect(res.accepted).toBe(1);
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(EVENTS_URL);
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual({
      events: [{ name: "ui_page_viewed", properties: { page: "settings.system.telemetry" } }],
    });
  });
});
