import { describe, it, expect, vi, afterEach } from "vitest";
import * as React from "react";
import { render } from "@testing-library/react";
import { createAppStore } from "@/lib/state/store";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { reviewItemId } from "@/components/task/review-selection";
import { buildHostApi } from "./host-api";
import { registerPluginTranslations, unregisterPluginTranslations } from "./plugin-translations";

const { changeRequestDetailModuleLoaded, translate } = vi.hoisted(() => ({
  changeRequestDetailModuleLoaded: vi.fn(),
  translate: vi.fn((key: string) => `translated:${key}`),
}));

vi.mock("@/lib/i18n", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/i18n")>()),
  t: translate,
}));

vi.mock("@/components/integrations/change-request-detail", () => {
  changeRequestDetailModuleLoaded();
  return { ChangeRequestDetail: () => null };
});

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
const I18N_PLUGIN_ID = "translation-host-test";

afterEach(() => {
  global.fetch = originalFetch;
  unregisterPluginTranslations(I18N_PLUGIN_ID);
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
          sessionId: "session-a",
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
      sessionId: "session-a",
      repositoryId: "repository-a",
      body: { token: "redacted" },
    });
  });
});

describe("buildHostApi — host contract", () => {
  it("exposes a plugin-scoped localization API", () => {
    const host = buildHostApi("jira", createAppStore());

    expect(host.i18n).toBeDefined();
  });

  it("translates plugin messages through both imperative and reactive APIs", () => {
    const greeting = "Hello {{name}}";
    registerPluginTranslations(I18N_PLUGIN_ID, {
      en: { greeting },
      "pt-pt": { greeting },
      "zh-cn": { greeting },
      pseudo: { greeting },
    });
    const host = buildHostApi(I18N_PLUGIN_ID, createAppStore());
    function Greeting() {
      const translation = host.i18n.useTranslation();
      return React.createElement(
        "span",
        { "data-testid": "plugin-greeting" },
        `${translation.locale}:${translation.t("greeting", { values: { name: "Ada" } })}`,
      );
    }

    const view = render(React.createElement(Greeting));

    expect(host.i18n.t("greeting", { values: { name: "Ada" } })).toBe("Hello Ada");
    expect(view.getByTestId("plugin-greeting").textContent).toBe(`${host.i18n.locale}:Hello Ada`);
  });

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
    expect(changeRequestDetailModuleLoaded).not.toHaveBeenCalled();
    expect(host.ui.IntegrationScopeBar).toBeDefined();
    expect(host.ui.IntegrationSaveQueryDialog).toBeDefined();
    expect(host.ui.IntegrationRepositoryFilter).toBeDefined();
    expect(host.ui.IntegrationCursorPagination).toBeDefined();
    const IntegrationIcon = host.ui.IntegrationIcon as React.ComponentType<{
      name: string;
    }>;
    expect(IntegrationIcon).toBeTypeOf("function");
    const rendered = render(React.createElement(IntegrationIcon, { name: "merged" }));
    expect(rendered.container.querySelector('[data-integration-icon="merged"]')).not.toBeNull();
    expect(host.ui.TaskRowIndicator).toBeDefined();
  });

  it("localizes the lazy change-request loading state", () => {
    const host = buildHostApi("jira", createAppStore());
    const ChangeRequestDetail = host.ui.ChangeRequestDetail as React.ComponentType<object>;

    const rendered = render(React.createElement(ChangeRequestDetail, {}));

    expect(rendered.getByLabelText("translated:integrations:loadingChangeRequest")).toBeTruthy();
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
    const repositoryId = "repository-uuid";
    const connectionScope = "https://bitbucket.example.test";
    const changeRequestNumber = 42;
    const reviewTitle = "Review #42";
    const review = {
      providerId: "bitbucket",
      reviewKey,
      connectionScope,
      repositoryId,
      changeRequestNumber,
      title: reviewTitle,
    };
    const store = createAppStore();
    const addReviewPanel = vi.spyOn(useDockviewStore.getState(), "addReviewPanel");
    const setMobileSessionReview = vi.spyOn(store.getState(), "setMobileSessionReview");
    const host = buildHostApi("bitbucket", store);

    host.openTaskReview({
      ...review,
      presentation: "desktop",
    });
    expect(addReviewPanel).toHaveBeenCalledWith(expect.objectContaining(review));

    host.openTaskReview({
      ...review,
      presentation: "mobile",
      sessionId: "session-1",
    });
    expect(setMobileSessionReview).toHaveBeenCalledWith("session-1", reviewItemId(review));
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

  it("exposes the host UUID fallback for plugins on insecure origins", () => {
    const host = buildHostApi("jira", createAppStore());
    const utils = host.utils as unknown as Record<string, unknown>;

    expect(utils.generateUUID).toBeTypeOf("function");
  });
});

describe("buildHostApi — ui", () => {
  it("exposes RichTextEditor/RichTextReadOnly for plugin notes-style UIs", () => {
    const host = buildHostApi("jira", createAppStore());
    expect(host.ui.RichTextEditor).toBeDefined();
    expect(host.ui.RichTextReadOnly).toBeDefined();
  });
});
