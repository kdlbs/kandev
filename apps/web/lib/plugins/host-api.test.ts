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

describe("buildHostApi — api.fetch", () => {
  const originalFetch = global.fetch;

  afterEach(() => {
    global.fetch = originalFetch;
  });

  it("scopes api.fetch to /api/plugins/{pluginId}/... and forwards init", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 200 }));
    global.fetch = fetchMock as unknown as typeof fetch;

    const host = buildHostApi("jira", createAppStore(), "light");
    await host.api.fetch("/issues", { method: "POST" });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain("/api/plugins/jira/issues");
    expect(init).toEqual({ method: "POST", credentials: "include" });
  });

  it("forces credentials: include even when init omits it, so the session cookie survives a split origin", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 200 }));
    global.fetch = fetchMock as unknown as typeof fetch;

    const host = buildHostApi("jira", createAppStore(), "light");
    await host.api.fetch("/issues");

    const [, init] = fetchMock.mock.calls[0];
    expect(init).toMatchObject({ credentials: "include" });
  });

  it("normalizes a path that doesn't start with a slash", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 200 }));
    global.fetch = fetchMock as unknown as typeof fetch;

    const host = buildHostApi("jira", createAppStore(), "light");
    await host.api.fetch("issues");

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain("/api/plugins/jira/issues");
  });
});

describe("buildHostApi", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
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

  it("routes openModal to the modal manager, scoped to this plugin's id", async () => {
    const { pluginModalManager } = await import("./modal-manager");
    const host = buildHostApi("jira", createAppStore(), "light");

    const before = pluginModalManager.getSnapshot().length;
    const handle = host.openModal({ content: () => null, title: "Test" });

    const snapshot = pluginModalManager.getSnapshot();
    expect(snapshot).toHaveLength(before + 1);
    expect(snapshot[snapshot.length - 1]).toMatchObject({
      pluginId: "jira",
      options: { title: "Test" },
    });

    handle.close();
    expect(pluginModalManager.getSnapshot()).toHaveLength(before);
  });
});

describe("buildHostApi — ui", () => {
  it("exposes RichTextEditor/RichTextReadOnly for plugin notes-style UIs", () => {
    const host = buildHostApi("jira", createAppStore(), "light");
    expect(host.ui.RichTextEditor).toBeDefined();
    expect(host.ui.RichTextReadOnly).toBeDefined();
  });
});

const NOTES_PLUGIN_ID = "kandev-plugin-notes";
const TEST_UPDATED_AT = "2026-01-01T00:00:00Z";

describe("buildHostApi — host.storage", () => {
  const originalFetch = global.fetch;

  afterEach(() => {
    global.fetch = originalFetch;
    vi.unstubAllEnvs();
  });

  it("get() targets the user-state route and returns {key, value, updatedAt}", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify({ value: "hi", updatedAt: TEST_UPDATED_AT }), { status: 200 }),
      );
    global.fetch = fetchMock as unknown as typeof fetch;

    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore(), "light");
    const entry = await host.storage.get("task", "task_1", "note");

    expect(entry).toEqual({ key: "note", value: "hi", updatedAt: TEST_UPDATED_AT });
    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain("/api/plugins/kandev-plugin-notes/user-state/task/task_1/note");
  });

  it("get() returns undefined on 404", async () => {
    global.fetch = vi
      .fn()
      .mockResolvedValue(new Response(null, { status: 404 })) as unknown as typeof fetch;

    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore(), "light");
    expect(await host.storage.get("task", "task_1", "note")).toBeUndefined();
  });

  it("get() throws on a non-2xx, non-404 status", async () => {
    global.fetch = vi
      .fn()
      .mockResolvedValue(new Response(null, { status: 500 })) as unknown as typeof fetch;

    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore(), "light");
    await expect(host.storage.get("task", "task_1", "note")).rejects.toThrow();
  });
});

describe("buildHostApi — host.storage set/delete/list/subscribe", () => {
  const originalFetch = global.fetch;

  afterEach(() => {
    global.fetch = originalFetch;
    vi.unstubAllEnvs();
  });

  it("set() PUTs a JSON envelope with value and a stamped writerId", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify({ updatedAt: TEST_UPDATED_AT }), { status: 200 }),
      );
    global.fetch = fetchMock as unknown as typeof fetch;

    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore(), "light");
    const result = await host.storage.set("task", "task_1", "note", "hello");

    expect(result).toEqual({ updatedAt: TEST_UPDATED_AT });
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain("/api/plugins/kandev-plugin-notes/user-state/task/task_1/note");
    expect(init.method).toBe("PUT");
    const body = JSON.parse(init.body as string);
    expect(body.value).toBe("hello");
    expect(typeof body.writerId).toBe("string");
    expect(body.writerId.length).toBeGreaterThan(0);
  });

  it("set() stamps the same writerId across multiple calls (per-tab, not per-call)", async () => {
    const fetchMock = vi
      .fn()
      .mockImplementation(
        () => new Response(JSON.stringify({ updatedAt: TEST_UPDATED_AT }), { status: 200 }),
      );
    global.fetch = fetchMock as unknown as typeof fetch;

    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore(), "light");
    await host.storage.set("task", "task_1", "note", "a");
    await host.storage.set("task", "task_1", "note", "b");

    const writerId1 = JSON.parse(fetchMock.mock.calls[0][1].body as string).writerId;
    const writerId2 = JSON.parse(fetchMock.mock.calls[1][1].body as string).writerId;
    expect(writerId1).toBe(writerId2);
  });

  it("set() forwards ifUnmodifiedSince and throws PluginStorageConflictError on 409", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 409 }));
    global.fetch = fetchMock as unknown as typeof fetch;

    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore(), "light");
    await expect(
      host.storage.set("task", "task_1", "note", "hello", { ifUnmodifiedSince: TEST_UPDATED_AT }),
    ).rejects.toThrow(/modified since/i);

    const body = JSON.parse(fetchMock.mock.calls[0][1].body as string);
    expect(body.ifUnmodifiedSince).toBe(TEST_UPDATED_AT);
  });

  it("delete() sends a DELETE with writerId as a query param", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 200 }));
    global.fetch = fetchMock as unknown as typeof fetch;

    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore(), "light");
    await host.storage.delete("task", "task_1", "note");

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain("/api/plugins/kandev-plugin-notes/user-state/task/task_1/note?writerId=");
    expect(init.method).toBe("DELETE");
  });

  it("list() returns the entries array in order (AC27)", async () => {
    const entries = [
      { key: "alpha", value: 1, updatedAt: TEST_UPDATED_AT },
      { key: "zeta", value: 2, updatedAt: "2026-01-01T00:00:01Z" },
    ];
    global.fetch = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify({ entries }), { status: 200 }),
      ) as unknown as typeof fetch;

    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore(), "light");
    expect(await host.storage.list("task", "task_1")).toEqual(entries);
  });

  it("subscribe() returns an unsubscribe function wired through the plugin registry", async () => {
    const { pluginRegistry } = await import("./registry");
    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore(), "light");

    const before = pluginRegistry.getWsHandlers("plugin.user-state.updated").length;
    const unsubscribe = host.storage.subscribe({}, () => {});
    expect(pluginRegistry.getWsHandlers("plugin.user-state.updated").length).toBe(before + 1);

    unsubscribe();
    expect(pluginRegistry.getWsHandlers("plugin.user-state.updated").length).toBe(before);
  });
});

describe("buildHostApi — host.storage writerId scoping", () => {
  const originalFetch = global.fetch;

  afterEach(() => {
    global.fetch = originalFetch;
  });

  // Appending, not replacing, matters: a surface id like panelId is a
  // static string shared by every tab with that panel open. Replacing the
  // tab-unique default with it would make two different tabs look like
  // the same writer to each other and break cross-tab sync (AC24).
  it("set() appends options.writerId to the tab default rather than replacing it", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify({ updatedAt: TEST_UPDATED_AT }), { status: 200 }),
      );
    global.fetch = fetchMock as unknown as typeof fetch;

    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore(), "light");
    await host.storage.set("task", "task_1", "note", "hello", { writerId: "panel-xyz" });

    const body = JSON.parse(fetchMock.mock.calls[0][1].body as string);
    expect(body.writerId).toMatch(/^.+:panel-xyz$/);
    expect(body.writerId).not.toBe("panel-xyz");
  });

  it("delete() appends options.writerId to the tab default rather than replacing it", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 200 }));
    global.fetch = fetchMock as unknown as typeof fetch;

    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore(), "light");
    await host.storage.delete("task", "task_1", "note", { writerId: "panel-xyz" });

    const [url] = fetchMock.mock.calls[0];
    expect(url).toMatch(/writerId=[^&]+%3Apanel-xyz$/);
    expect(url).not.toContain("writerId=panel-xyz");
  });
});

/**
 * The per-tab writer id is derived at MODULE SCOPE, so anything that throws
 * while computing it breaks the whole module — and `host-api` is on the
 * plugin boot path (`lib/plugins/boot.ts`), so the blast radius is every
 * plugin, not just storage.
 *
 * `crypto.randomUUID` is a secure-context-only API: on an http:// origin that
 * is not localhost (a shared VPS/homelab instance, which the auth spec
 * explicitly targets) it is undefined. Regression test for that environment.
 */
describe("buildHostApi — non-secure context (http:// on a non-localhost origin)", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.resetModules();
  });

  it("still loads and stamps a writerId when crypto.randomUUID is unavailable", async () => {
    // A non-secure context exposes `crypto` but not `randomUUID`.
    vi.stubGlobal("crypto", {});
    vi.resetModules();

    const freshHostApi = await import("./host-api");
    const { createAppStore: freshCreateStore } = await import("@/lib/state/store");

    global.fetch = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify({ updatedAt: TEST_UPDATED_AT }), { status: 200 }),
      ) as unknown as typeof fetch;

    const host = freshHostApi.buildHostApi(NOTES_PLUGIN_ID, freshCreateStore(), "light");
    await host.storage.set("task", "task_1", "note", { body: "hi" });

    const body = JSON.parse(
      (global.fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0][1].body as string,
    );
    expect(typeof body.writerId).toBe("string");
    expect(body.writerId.length).toBeGreaterThan(0);
  });
});
