import { describe, it, expect, vi, afterEach } from "vitest";
import * as React from "react";
import { createAppStore } from "@/lib/state/store";
import { buildHostApi } from "./host-api";

/** Curated primitives a plugin needs to build a full native-feeling page. */
const EXPECTED_UI_PRIMITIVES = [
  "Alert",
  "Badge",
  "Button",
  "Card",
  "Checkbox",
  "Dialog",
  "DropdownMenu",
  "Input",
  "Label",
  "Pagination",
  "ScrollArea",
  "Select",
  "Sheet",
  "SheetClose",
  "SheetContent",
  "SheetDescription",
  "SheetFooter",
  "SheetHeader",
  "SheetTitle",
  "SheetTrigger",
  "Spinner",
  "Switch",
  "Table",
  "Tabs",
  "Textarea",
  "Tooltip",
];

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
    // Expanded primitive set for full native-feeling plugin pages.
    for (const name of EXPECTED_UI_PRIMITIVES) {
      expect(host.ui[name], `host.ui.${name}`).toBeDefined();
    }
  });

  it("exposes first-party app components for native flows and page chrome", () => {
    const host = buildHostApi("jira", createAppStore(), "light");
    expect(host.ui.PageTopbar).toBeDefined();
    expect(host.ui.TaskCreateDialog).toBeDefined();
    expect(host.ui.Combobox).toBeDefined();
  });

  it("exposes navigate() that soft-navigates via history push/replace", () => {
    const host = buildHostApi("jira", createAppStore(), "light");
    const pushSpy = vi.spyOn(window.history, "pushState");
    const replaceSpy = vi.spyOn(window.history, "replaceState");

    host.navigate("/somewhere");
    expect(pushSpy).toHaveBeenCalledWith(
      expect.objectContaining({ __kandevNavigationPosition: expect.any(Number) }),
      "",
      "/somewhere",
    );

    host.navigate("/elsewhere", { replace: true });
    expect(replaceSpy).toHaveBeenCalledWith(
      expect.objectContaining({ __kandevNavigationPosition: expect.any(Number) }),
      "",
      "/elsewhere",
    );

    pushSpy.mockRestore();
    replaceSpy.mockRestore();
  });

  it("exposes the backend API origin on api.baseUrl", () => {
    const host = buildHostApi("jira", createAppStore(), "light");
    expect(typeof host.api.baseUrl).toBe("string");
  });

  it("sets pluginId on the returned host api", () => {
    const host = buildHostApi("jira", createAppStore(), "light");
    expect(host.pluginId).toBe("jira");
  });
});
