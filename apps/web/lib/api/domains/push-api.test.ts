import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import {
  deletePushSubscription,
  getPushPublicKey,
  listPushSubscriptions,
  savePushSubscription,
  sendTestPush,
} from "./push-api";

const BASE = "http://api.test/api/v1/push";
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

describe("push api", () => {
  it("gets the VAPID public key without cache", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ public_key: "key" }));
    await expect(getPushPublicKey()).resolves.toEqual({ public_key: "key" });
    expect(lastCall().url).toBe(`${BASE}/public-key`);
    expect(lastCall().init?.cache).toBe("no-store");
  });

  it("lists the current user's subscriptions without cache", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ subscriptions: [] }));
    await listPushSubscriptions();
    expect(lastCall().url).toBe(`${BASE}/subscriptions`);
    expect(lastCall().init?.cache).toBe("no-store");
  });

  it("posts a browser subscription", async () => {
    const payload = { endpoint: "https://push.example/a", keys: { p256dh: "p", auth: "a" } };
    fetchSpy.mockResolvedValueOnce(jsonResponse({ success: true }));
    await savePushSubscription(payload);
    expect(lastCall().url).toBe(`${BASE}/subscriptions`);
    expect(lastCall().init?.method).toBe("POST");
    expect(lastCall().init?.body).toBe(JSON.stringify(payload));
  });

  it("sends the endpoint in the DELETE request body", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ success: true }));
    await deletePushSubscription("https://push.example/a");
    expect(lastCall().url).toBe(`${BASE}/subscriptions`);
    expect(lastCall().init?.method).toBe("DELETE");
    expect(lastCall().init?.body).toBe(JSON.stringify({ endpoint: "https://push.example/a" }));
  });

  it("posts a test notification", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ success: true, delivered: 1 }));
    await sendTestPush();
    expect(lastCall().url).toBe(`${BASE}/test`);
    expect(lastCall().init?.method).toBe("POST");
    expect(lastCall().init?.body).toBeUndefined();
  });
});
