import { describe, it, expect, vi, afterEach } from "vitest";
import * as React from "react";
import { render } from "@testing-library/react";
import { createAppStore } from "@/lib/state/store";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { reviewItemId } from "@/components/task/review-selection";
import { buildHostApi } from "./host-api";

/** Curated primitives a plugin needs to build a full native-feeling page. */
const EXPECTED_UI_PRIMITIVES = [
  "Alert",
  "Badge",
  "Button",
  "Card",
  "Checkbox",
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

  it("invokes declared authenticated actions with scoped resources and parses their response", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(new Response(JSON.stringify({ connected: true }), { status: 200 }));
    global.fetch = fetchMock as unknown as typeof fetch;

    const host = buildHostApi("jira", createAppStore(), "light");
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

  it("exports the supported responsive breakpoint hook", () => {
    const host = buildHostApi("jira", createAppStore(), "dark");

    expect(host.useResponsiveBreakpoint).toBeTypeOf("function");
  });

  it("exposes first-party app components for native flows and page chrome", () => {
    const host = buildHostApi("jira", createAppStore(), "light");
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

  it("opens a host-owned task link dialog instead of requiring plugin form markup", async () => {
    const { pluginModalManager } = await import("./modal-manager");
    const host = buildHostApi("jira", createAppStore(), "light");
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
    const host = buildHostApi("bitbucket", store, "light");

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
