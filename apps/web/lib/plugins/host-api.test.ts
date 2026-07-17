import { describe, it, expect, vi, afterEach } from "vitest";
import * as React from "react";
import { createAppStore } from "@/lib/state/store";
import { buildHostApi } from "./host-api";

describe("buildHostApi", () => {
  const originalFetch = global.fetch;

  afterEach(() => {
    global.fetch = originalFetch;
    vi.unstubAllEnvs();
  });

  it("scopes api.fetch to /api/plugins/{pluginId}/... and forwards init", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 200 }));
    global.fetch = fetchMock as unknown as typeof fetch;

    const host = buildHostApi("jira", createAppStore(), "light");
    await host.api.fetch("/issues", { method: "POST" });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain("/api/plugins/jira/issues");
    expect(init).toEqual({ method: "POST" });
  });

  it("normalizes a path that doesn't start with a slash", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 200 }));
    global.fetch = fetchMock as unknown as typeof fetch;

    const host = buildHostApi("jira", createAppStore(), "light");
    await host.api.fetch("issues");

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain("/api/plugins/jira/issues");
  });

  it("exposes the host React instance and a jsx alias for React.createElement", () => {
    const host = buildHostApi("jira", createAppStore(), "dark");

    expect(host.React).toBe(React);
    expect(host.jsx).toBe(React.createElement);
  });

  it("wires store.getState/setState/subscribe to the passed StoreApi", () => {
    const store = createAppStore();
    const getStateSpy = vi.spyOn(store, "getState");
    const setStateSpy = vi.spyOn(store, "setState");
    const subscribeSpy = vi.spyOn(store, "subscribe");

    const host = buildHostApi("jira", store, "dark");

    expect(host.store.getState()).toBe(store.getState());
    host.store.setState({});
    const listener = vi.fn();
    const unsubscribe = host.store.subscribe(listener);

    expect(getStateSpy).toHaveBeenCalled();
    expect(setStateSpy).toHaveBeenCalled();
    expect(subscribeSpy).toHaveBeenCalled();
    unsubscribe();
  });

  it("exposes the requested theme and a curated ui component subset", () => {
    const host = buildHostApi("jira", createAppStore(), "dark");

    expect(host.theme).toBe("dark");
    expect(host.ui.Button).toBeDefined();
    expect(host.ui.Card).toBeDefined();
    expect(host.ui.Badge).toBeDefined();
  });

  it("sets pluginId on the returned host api", () => {
    const host = buildHostApi("jira", createAppStore(), "light");
    expect(host.pluginId).toBe("jira");
  });
});
