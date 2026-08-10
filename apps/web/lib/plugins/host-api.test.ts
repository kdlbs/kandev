import { describe, it, expect, vi, afterEach } from "vitest";
import * as React from "react";
import { render } from "@testing-library/react";
import { createAppStore } from "@/lib/state/store";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { reviewItemId } from "@/components/task/review-selection";
import { buildHostApi } from "./host-api";

/** Curated primitives a plugin needs to build a full native-feeling page. */
const EXPECTED_UI_PRIMITIVES = [
  "Accordion",
  "AccordionContent",
  "AccordionItem",
  "AccordionTrigger",
  "Alert",
  "Badge",
  "Button",
  "Card",
  "ChartContainer",
  "ChartLegend",
  "ChartLegendContent",
  "ChartStyle",
  "ChartTooltip",
  "ChartTooltipContent",
  "Checkbox",
  "Collapsible",
  "CollapsibleContent",
  "CollapsibleTrigger",
  "Dialog",
  "Drawer",
  "DrawerClose",
  "DrawerContent",
  "DrawerDescription",
  "DrawerFooter",
  "DrawerHeader",
  "DrawerOverlay",
  "DrawerPortal",
  "DrawerTitle",
  "DrawerTrigger",
  "DropdownMenu",
  "Empty",
  "EmptyContent",
  "EmptyDescription",
  "EmptyHeader",
  "EmptyMedia",
  "EmptyTitle",
  "Input",
  "Kbd",
  "KbdGroup",
  "Label",
  "Pagination",
  "Popover",
  "PopoverAnchor",
  "PopoverContent",
  "PopoverDescription",
  "PopoverHeader",
  "PopoverTitle",
  "PopoverTrigger",
  "Progress",
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
  "TooltipProvider",
];

const originalFetch = global.fetch;

afterEach(() => {
  global.fetch = originalFetch;
  vi.unstubAllEnvs();
  vi.restoreAllMocks();
});

describe("buildHostApi — API", () => {
  it("scopes api.fetch to /api/plugins/{pluginId}/... and forwards init", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 200 }));
    global.fetch = fetchMock as unknown as typeof fetch;

    const host = buildHostApi("jira", createAppStore());
    await host.api.fetch("/issues", { method: "POST" });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain("/api/plugins/jira/issues");
    expect(init).toEqual({ method: "POST", credentials: "include" });
  });

  it("forces credentials: include even when init omits it, so the session cookie survives a split origin", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 200 }));
    global.fetch = fetchMock as unknown as typeof fetch;

    const host = buildHostApi("jira", createAppStore());
    await host.api.fetch("/issues");

    const [, init] = fetchMock.mock.calls[0];
    expect(init).toMatchObject({ credentials: "include" });
  });

  it("normalizes a path that doesn't start with a slash", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 200 }));
    global.fetch = fetchMock as unknown as typeof fetch;

    const host = buildHostApi("jira", createAppStore());
    await host.api.fetch("issues");

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain("/api/plugins/jira/issues");
  });
});

describe("buildHostApi", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it("invokes declared authenticated actions with scoped resources and parses their response", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response(JSON.stringify({ connected: true }), { status: 200 }));
    global.fetch = fetchMock as unknown as typeof fetch;

    const host = buildHostApi("jira", createAppStore());
    const controller = new AbortController();
    await expect(
      host.api.invokeAction<{ connected: boolean }>(
        "connection.save",
        {
          workspaceId: "workspace-a",
          taskId: "task-a",
          repositoryId: "repository-a",
          body: { token: "redacted" },
        },
        { signal: controller.signal },
      ),
    ).resolves.toEqual({ connected: true });

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain("/api/plugins/jira/actions/connection.save");
    expect(init).toMatchObject({
      method: "POST",
      credentials: "include",
      signal: controller.signal,
    });
    expect(JSON.parse(init.body)).toEqual({
      workspaceId: "workspace-a",
      taskId: "task-a",
      repositoryId: "repository-a",
      body: { token: "redacted" },
    });
  });
});

describe("buildHostApi — host contract", () => {
  it("exposes the host React instance and a jsx alias for React.createElement", () => {
    const host = buildHostApi("jira", createAppStore());

    expect(host.React).toBe(React);
    expect(host.jsx).toBe(React.createElement);
  });

  it("wires store.getState/setState/subscribe to the passed StoreApi", () => {
    const store = createAppStore();
    const getStateSpy = vi.spyOn(store, "getState");
    const setStateSpy = vi.spyOn(store, "setState");
    const subscribeSpy = vi.spyOn(store, "subscribe");

    const host = buildHostApi("jira", store);

    expect(host.store.getState()).toBe(store.getState());
    host.store.setState({});
    const listener = vi.fn();
    const unsubscribe = host.store.subscribe(listener);

    expect(getStateSpy).toHaveBeenCalled();
    expect(setStateSpy).toHaveBeenCalled();
    expect(subscribeSpy).toHaveBeenCalled();
    unsubscribe();
  });

  it("exposes a curated ui component subset", () => {
    const host = buildHostApi("jira", createAppStore());

    // Expanded primitive set for full native-feeling plugin pages.
    for (const name of EXPECTED_UI_PRIMITIVES) {
      expect(host.ui[name], `host.ui.${name}`).toBeDefined();
    }
  });

  it("exports the supported responsive breakpoint hook", () => {
    const host = buildHostApi("jira", createAppStore());

    expect(host.useResponsiveBreakpoint).toBeTypeOf("function");
  });

  it("exposes first-party app components for native flows and page chrome", () => {
    const host = buildHostApi("jira", createAppStore());
    expect(host.ui.PageTopbar).toBeDefined();
    expect(host.ui.TaskCreateDialog).toBeDefined();
    expect(host.ui.Combobox).toBeDefined();
    expect(host.ui.ChangeRequestList).toBeDefined();
    expect(host.ui.ChangeRequestRow).toBeDefined();
    expect(host.ui.IntegrationStartTaskMenu).toBeDefined();
    expect(host.ui.IntegrationListToolbar).toBeDefined();
    expect(host.ui.IntegrationChangeRequestStatus).toBeDefined();
    expect(host.ui.ChangeRequestDetail).toBeTypeOf("function");
    expect(host.ui.IntegrationScopeBar).toBeDefined();
    const IntegrationIcon = host.ui.IntegrationIcon as React.ComponentType<{
      name: string;
    }>;
    expect(IntegrationIcon).toBeTypeOf("function");
    const rendered = render(React.createElement(IntegrationIcon, { name: "merged" }));
    expect(rendered.container.querySelector('[data-integration-icon="merged"]')).not.toBeNull();
    expect(host.ui.TaskRowIndicator).toBeDefined();
  });
});

describe("buildHostApi — navigation and modal contract", () => {
  it("exposes navigate() that soft-navigates via history push/replace", () => {
    const host = buildHostApi("jira", createAppStore());
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
    const host = buildHostApi("jira", createAppStore());
    expect(typeof host.api.baseUrl).toBe("string");
  });

  it("sets pluginId on the returned host api", () => {
    const host = buildHostApi("jira", createAppStore());
    expect(host.pluginId).toBe("jira");
  });

  it("routes openModal to the modal manager, scoped to this plugin's id", async () => {
    const { pluginModalManager } = await import("./modal-manager");
    const host = buildHostApi("jira", createAppStore());

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

  it("opens a host-owned task link dialog instead of requiring plugin form markup", async () => {
    const { pluginModalManager } = await import("./modal-manager");
    const host = buildHostApi("jira", createAppStore());
    const before = pluginModalManager.getSnapshot().length;

    const handle = host.openTaskLinkDialog({
      title: "Link Acme pull request",
      description: "Use an Acme pull request URL for this task.",
      inputLabel: "Pull request",
      emptyError: "Enter a pull request.",
      failureMessage: "Failed to link pull request.",
      successMessage: "Pull request linked",
      onSubmit: vi.fn().mockResolvedValue(undefined),
    });

    expect(pluginModalManager.getSnapshot().at(-1)).toMatchObject({
      pluginId: "jira",
      layout: "task-link",
      options: {
        title: "Link Acme pull request",
        description: "Use an Acme pull request URL for this task.",
        presentation: "dialog",
      },
    });
    handle.close();
    expect(pluginModalManager.getSnapshot()).toHaveLength(before);
  });

  it("opens provider-neutral desktop and mobile task review surfaces", () => {
    const reviewKey = "cloud|workspace/repo|42";
    const reviewTitle = "Review #42";
    const store = createAppStore();
    const addReviewPanel = vi.spyOn(useDockviewStore.getState(), "addReviewPanel");
    const setMobileSessionReview = vi.spyOn(store.getState(), "setMobileSessionReview");
    const host = buildHostApi("bitbucket", store);

    host.openTaskReview({
      providerId: "bitbucket",
      reviewKey,
      title: reviewTitle,
      presentation: "desktop",
    });
    expect(addReviewPanel).toHaveBeenCalledWith("bitbucket", reviewKey, reviewTitle);

    host.openTaskReview({
      providerId: "bitbucket",
      reviewKey,
      title: reviewTitle,
      presentation: "mobile",
      sessionId: "session-1",
    });
    expect(setMobileSessionReview).toHaveBeenCalledWith(
      "session-1",
      reviewItemId({ providerId: "bitbucket", reviewKey }),
    );
  });
});

/**
 * jsdom delivers MutationObserver records on a microtask, so awaiting an
 * already-resolved promise is enough to let a pending class mutation land.
 */
function flushMutationObservers(): Promise<void> {
  return Promise.resolve();
}

/**
 * `host` is built once per plugin load, so a theme captured into it at boot
 * can never follow a light/dark switch. These pin the live-read contract and
 * the change notification a canvas-painting plugin needs on top of it.
 */
describe("buildHostApi — host.theme / host.onThemeChange", () => {
  afterEach(() => {
    document.documentElement.classList.remove("dark", "light");
  });

  it("reads the resolved theme live rather than freezing it at build time", () => {
    document.documentElement.classList.remove("dark");
    const host = buildHostApi("jira", createAppStore());
    expect(host.theme).toBe("light");

    document.documentElement.classList.add("dark");
    expect(host.theme).toBe("dark");

    document.documentElement.classList.remove("dark");
    expect(host.theme).toBe("light");
  });

  it("notifies onThemeChange subscribers once per flip and stops after unsubscribe", async () => {
    document.documentElement.classList.remove("dark");
    const host = buildHostApi("jira", createAppStore());
    const listener = vi.fn();
    const unsubscribe = host.onThemeChange(listener);

    document.documentElement.classList.add("dark");
    await flushMutationObservers();
    expect(listener).toHaveBeenCalledExactlyOnceWith("dark");

    unsubscribe();
    document.documentElement.classList.remove("dark");
    await flushMutationObservers();
    expect(listener).toHaveBeenCalledTimes(1);
  });

  // Cross-plugin fan-out: one buggy plugin must not silently stop theme
  // updates for every other plugin. The throwing listener is registered
  // FIRST so an unguarded loop would abort before reaching the healthy one.
  it("keeps notifying later listeners when an earlier one throws", async () => {
    document.documentElement.classList.remove("dark");
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const host = buildHostApi("jira", createAppStore());

    const boom = vi.fn(() => {
      throw new Error("plugin theme listener blew up");
    });
    const healthy = vi.fn();
    const unsubscribeBoom = host.onThemeChange(boom);
    const unsubscribeHealthy = host.onThemeChange(healthy);

    document.documentElement.classList.add("dark");
    await flushMutationObservers();

    expect(boom).toHaveBeenCalledTimes(1);
    expect(healthy).toHaveBeenCalledExactlyOnceWith("dark");
    expect(consoleError).toHaveBeenCalledWith(
      "[plugins] theme change listener threw:",
      expect.any(Error),
    );

    unsubscribeBoom();
    unsubscribeHealthy();
    consoleError.mockRestore();
  });

  it("ignores class mutations that leave the resolved theme unchanged", async () => {
    document.documentElement.classList.add("dark");
    const host = buildHostApi("jira", createAppStore());
    const listener = vi.fn();
    const unsubscribe = host.onThemeChange(listener);

    // Unrelated class churn on <html> — the rendering-engine marker, a
    // transition-suppression class, etc. — must not wake every plugin.
    document.documentElement.classList.add("some-unrelated-class");
    await flushMutationObservers();
    expect(listener).not.toHaveBeenCalled();

    unsubscribe();
    document.documentElement.classList.remove("some-unrelated-class");
  });
});

/**
 * `toast` and `utils` are functions, so they sit beside `navigate`/`openModal`
 * rather than in `ui`, which is a component map.
 */
describe("buildHostApi — host.toast / host.utils", () => {
  it("exposes sonner's imperative toast, which needs no host rendering", () => {
    const host = buildHostApi("jira", createAppStore());

    expect(typeof host.toast).toBe("function");
    for (const method of ["success", "error", "warning", "info", "dismiss"] as const) {
      expect(typeof host.toast[method], `host.toast.${method}`).toBe("function");
    }
  });

  // Scoped per plugin: an unattributed plugin error toast would land in
  // kandev's own frontend error log as an application error.
  it("scopes toast.error to the calling plugin rather than the app reporting seam", () => {
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const host = buildHostApi("kandev-plugin-github-status", createAppStore());

    host.toast.error("Poll failed");

    expect(consoleError).toHaveBeenCalledWith(
      '[plugins] toast.error from "kandev-plugin-github-status":',
      "Poll failed",
    );
    consoleError.mockRestore();
  });

  it("exposes cn, merging conflicting tailwind classes the way host components do", () => {
    const host = buildHostApi("jira", createAppStore());

    expect(typeof host.utils.cn).toBe("function");
    // tailwind-merge semantics, not plain concatenation — the later padding wins.
    expect(host.utils.cn("p-2", "p-4")).toBe("p-4");
    const hidden = false;
    expect(host.utils.cn("text-sm", hidden && "hidden", "font-bold")).toBe("text-sm font-bold");
  });

  it("exposes formatRelativeTime rather than leaving plugins to hand-roll one", () => {
    const host = buildHostApi("jira", createAppStore());

    expect(typeof host.utils.formatRelativeTime).toBe("function");
    const threeHoursAgo = new Date(Date.now() - 3 * 60 * 60 * 1000);
    expect(host.utils.formatRelativeTime(threeHoursAgo)).toContain("3");
  });
});

describe("buildHostApi — ui", () => {
  it("exposes RichTextEditor/RichTextReadOnly for plugin notes-style UIs", () => {
    const host = buildHostApi("jira", createAppStore());
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

    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore());
    const entry = await host.storage.get("task", "task_1", "note");

    expect(entry).toEqual({ key: "note", value: "hi", updatedAt: TEST_UPDATED_AT });
    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain("/api/plugins/kandev-plugin-notes/user-state/task/task_1/note");
  });

  it("get() returns undefined on 404", async () => {
    global.fetch = vi
      .fn()
      .mockResolvedValue(new Response(null, { status: 404 })) as unknown as typeof fetch;

    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore());
    expect(await host.storage.get("task", "task_1", "note")).toBeUndefined();
  });

  it("get() throws on a non-2xx, non-404 status", async () => {
    global.fetch = vi
      .fn()
      .mockResolvedValue(new Response(null, { status: 500 })) as unknown as typeof fetch;

    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore());
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

    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore());
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

    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore());
    await host.storage.set("task", "task_1", "note", "a");
    await host.storage.set("task", "task_1", "note", "b");

    const writerId1 = JSON.parse(fetchMock.mock.calls[0][1].body as string).writerId;
    const writerId2 = JSON.parse(fetchMock.mock.calls[1][1].body as string).writerId;
    expect(writerId1).toBe(writerId2);
  });

  it("set() forwards ifUnmodifiedSince and throws PluginStorageConflictError on 409", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 409 }));
    global.fetch = fetchMock as unknown as typeof fetch;

    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore());
    await expect(
      host.storage.set("task", "task_1", "note", "hello", { ifUnmodifiedSince: TEST_UPDATED_AT }),
    ).rejects.toThrow(/modified since/i);

    const body = JSON.parse(fetchMock.mock.calls[0][1].body as string);
    expect(body.ifUnmodifiedSince).toBe(TEST_UPDATED_AT);
  });

  it("delete() sends a DELETE with writerId as a query param", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 200 }));
    global.fetch = fetchMock as unknown as typeof fetch;

    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore());
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

    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore());
    expect(await host.storage.list("task", "task_1")).toEqual(entries);
  });

  it("subscribe() returns an unsubscribe function wired through the plugin registry", async () => {
    const { pluginRegistry } = await import("./registry");
    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore());

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

    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore());
    await host.storage.set("task", "task_1", "note", "hello", { writerId: "panel-xyz" });

    const body = JSON.parse(fetchMock.mock.calls[0][1].body as string);
    expect(body.writerId).toMatch(/^.+:panel-xyz$/);
    expect(body.writerId).not.toBe("panel-xyz");
  });

  it("delete() appends options.writerId to the tab default rather than replacing it", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 200 }));
    global.fetch = fetchMock as unknown as typeof fetch;

    const host = buildHostApi(NOTES_PLUGIN_ID, createAppStore());
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

    const host = freshHostApi.buildHostApi(NOTES_PLUGIN_ID, freshCreateStore());
    await host.storage.set("task", "task_1", "note", { body: "hi" });

    const body = JSON.parse(
      (global.fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0][1].body as string,
    );
    expect(typeof body.writerId).toBe("string");
    expect(body.writerId.length).toBeGreaterThan(0);
  });
});
