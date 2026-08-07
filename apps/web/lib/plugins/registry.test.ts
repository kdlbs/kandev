import { describe, it, expect, afterEach, vi } from "vitest";
import { pluginRegistry } from "./registry";
import type { RepositoryProviderRegistration, ReviewProviderRegistration } from "./types";

const TASK_SIDEBAR_SLOT = "task-sidebar";
const TASK_CREATED_ACTION = "task.created";
const APP_STATUS_LEFT_SLOT = "app-status-bar-left";
const PRIMARY_PLUGIN_ID = "plugin-a";
const SECONDARY_PLUGIN_ID = "plugin-b";
const SOURCE_CONTROL_PROVIDER_ID = "source-control";
const CHANGE_URL = "https://example.test/changes/1",
  REPOSITORY_ID = "repository-a";
function cleanup(...pluginIds: string[]) {
  pluginIds.forEach((id) => pluginRegistry.unregisterPlugin(id));
}
function repositoryProvider(
  id: string,
  overrides: Partial<RepositoryProviderRegistration> = {},
): RepositoryProviderRegistration {
  return {
    id,
    label: id,
    listRepositories: async () => [],
    matchesURL: () => false,
    listBranches: async () => [],
    inspectURL: async () => null,
    ...overrides,
  };
}

function reviewProvider(
  id: string,
  overrides: Partial<ReviewProviderRegistration> = {},
): ReviewProviderRegistration {
  return {
    id,
    label: id,
    changeRequestNoun: "change request",
    order: 0,
    getSnapshot: () => [],
    subscribe: () => () => {},
    refresh: async () => {},
    ReviewPanel: () => null,
    ...overrides,
  };
}

function providerSpecificStatusSnapshot() {
  return [
    {
      providerId: SOURCE_CONTROL_PROVIDER_ID,
      reviewKey: "change-1",
      title: "Change 1",
      url: CHANGE_URL,
      repositoryId: REPOSITORY_ID,
      state: "open",
      taskStatus: {
        number: 1,
        state: "open" as const,
        pipelineState: "failure" as const,
        checks: [
          {
            id: "build",
            label: "Build",
            state: "failure" as const,
            detail: "Failed in 2m",
            url: "https://example.test/checks/build",
            providerPayload: "discard",
          },
        ],
        review: {
          state: "approved" as const,
          approved: 1,
          required: 2,
          requested: 1,
          providerPayload: "discard",
        },
        unresolvedComments: 3,
        providerPayload: "discard",
      },
    },
  ];
}

function normalizedStatusSnapshot() {
  const [{ taskStatus, ...review }] = providerSpecificStatusSnapshot();
  const [{ providerPayload: _checkPayload, ...check }] = taskStatus.checks;
  const { providerPayload: _reviewPayload, ...reviewSummary } = taskStatus.review;
  const {
    providerPayload: _statusPayload,
    checks: _checks,
    review: _review,
    ...status
  } = taskStatus;
  return [
    {
      ...review,
      taskStatus: { ...status, checks: [check], review: reviewSummary },
    },
  ];
}

describe("pluginRegistry", () => {
  afterEach(() => {
    cleanup("plugin-a", "plugin-b");
  });

  it("registers and returns a route via the scoped registry view", () => {
    const scoped = pluginRegistry.forPlugin("plugin-a");
    function Page() {
      return null;
    }

    scoped.registerRoute("/plugin-a", Page);

    const routes = pluginRegistry.getRoutes();
    expect(routes).toContainEqual({
      pluginId: "plugin-a",
      path: "/plugin-a",
      Component: Page,
      options: undefined,
    });
  });

  it("registers and returns a nav item", () => {
    const scoped = pluginRegistry.forPlugin("plugin-a");

    scoped.registerNavItem({ id: "nav-a", label: "A", path: "/plugin-a" });

    expect(pluginRegistry.getNavItems()).toContainEqual({
      id: "nav-a",
      label: "A",
      path: "/plugin-a",
    });
  });

  it("registers and returns a settings route", () => {
    const scoped = pluginRegistry.forPlugin("plugin-a");
    function Settings() {
      return null;
    }

    scoped.registerSettingsRoute("/settings/plugins/plugin-a", Settings);

    expect(pluginRegistry.getSettingsRoutes()).toContainEqual({
      path: "/settings/plugins/plugin-a",
      Component: Settings,
    });
  });

  it("registers a slot component and only returns it for the matching slot", () => {
    const scoped = pluginRegistry.forPlugin("plugin-a");
    function Sidebar() {
      return null;
    }

    scoped.registerComponent(TASK_SIDEBAR_SLOT, Sidebar);

    expect(pluginRegistry.getSlotComponents(TASK_SIDEBAR_SLOT)).toEqual([Sidebar]);
    expect(pluginRegistry.getSlotComponents("settings-nav")).toEqual([]);
  });

  it("returns slot registrations with stable owner and registration identity", () => {
    const scopedA = pluginRegistry.forPlugin("plugin-a");
    function Sidebar() {
      return null;
    }

    scopedA.registerComponent(TASK_SIDEBAR_SLOT, Sidebar);

    const [registration] = pluginRegistry.getSlotRegistrations(TASK_SIDEBAR_SLOT);

    expect(registration).toEqual({
      registrationId: expect.any(String),
      orderingId: expect.any(String),
      pluginId: "plugin-a",
      Component: Sidebar,
    });

    scopedA.registerComponent(TASK_SIDEBAR_SLOT, () => null);

    expect(pluginRegistry.getSlotRegistrations(TASK_SIDEBAR_SLOT)[0]?.registrationId).toBe(
      registration?.registrationId,
    );
  });

  it("restores deterministic ordering identities after plugin re-enable", () => {
    function First() {
      return null;
    }
    function Second() {
      return null;
    }
    const scoped = pluginRegistry.forPlugin("plugin-a");
    scoped.registerComponent(APP_STATUS_LEFT_SLOT, First);
    scoped.registerComponent(APP_STATUS_LEFT_SLOT, Second);
    const before = pluginRegistry
      .getSlotRegistrations(APP_STATUS_LEFT_SLOT)
      .map((registration) => registration.orderingId);

    pluginRegistry.unregisterPlugin("plugin-a");
    const reenabled = pluginRegistry.forPlugin("plugin-a");
    reenabled.registerComponent(APP_STATUS_LEFT_SLOT, First);
    reenabled.registerComponent(APP_STATUS_LEFT_SLOT, Second);

    expect(pluginRegistry.getSlotRegistrations(APP_STATUS_LEFT_SLOT)).toMatchObject([
      { orderingId: before[0], pluginId: "plugin-a", Component: First },
      { orderingId: before[1], pluginId: "plugin-a", Component: Second },
    ]);
    expect(before[0]).not.toBe(before[1]);
  });
});

describe("pluginRegistry — lifecycle", () => {
  afterEach(() => {
    cleanup("plugin-a", "plugin-b");
  });

  it("registers a WS handler and only returns it for the matching action", () => {
    const scoped = pluginRegistry.forPlugin("plugin-a");
    const handler = () => {};

    scoped.registerWsHandler(TASK_CREATED_ACTION, handler);

    expect(pluginRegistry.getWsHandlers(TASK_CREATED_ACTION)).toEqual([handler]);
    expect(pluginRegistry.getWsHandlers("task.deleted")).toEqual([]);
  });

  it("bulk-revokes every registration owned by a plugin on unregisterPlugin", () => {
    const scopedA = pluginRegistry.forPlugin("plugin-a");
    const scopedB = pluginRegistry.forPlugin("plugin-b");
    function PageA() {
      return null;
    }
    function PageB() {
      return null;
    }

    scopedA.registerRoute("/plugin-a", PageA);
    scopedA.registerNavItem({ id: "nav-a", label: "A", path: "/plugin-a" });
    scopedA.registerComponent(TASK_SIDEBAR_SLOT, PageA);
    scopedA.registerWsHandler(TASK_CREATED_ACTION, () => {});
    scopedB.registerRoute("/plugin-b", PageB);

    pluginRegistry.unregisterPlugin("plugin-a");

    expect(pluginRegistry.getRoutes()).toEqual([
      { pluginId: "plugin-b", path: "/plugin-b", Component: PageB, options: undefined },
    ]);
    expect(pluginRegistry.getNavItems().find((item) => item.id === "nav-a")).toBeUndefined();
    expect(pluginRegistry.getSlotComponents(TASK_SIDEBAR_SLOT)).toEqual([]);
    expect(pluginRegistry.getWsHandlers(TASK_CREATED_ACTION)).toEqual([]);
  });

  it("notifies subscribers when a registration is added", () => {
    const scoped = pluginRegistry.forPlugin("plugin-a");
    let notified = 0;
    const unsubscribe = pluginRegistry.subscribe(() => {
      notified += 1;
    });

    scoped.registerNavItem({ id: "nav-a", label: "A", path: "/plugin-a" });

    unsubscribe();
    expect(notified).toBe(1);
  });

  it("does not notify subscribers when unregistering a plugin with no registrations", () => {
    let notified = 0;
    const unsubscribe = pluginRegistry.subscribe(() => {
      notified += 1;
    });

    pluginRegistry.unregisterPlugin("plugin-with-nothing-registered");

    unsubscribe();
    expect(notified).toBe(0);
  });
});

describe("pluginRegistry — route options and plugin names", () => {
  afterEach(() => {
    cleanup("plugin-a");
  });

  it("stores route options and the plugin display name for page chrome", () => {
    const scoped = pluginRegistry.forPlugin("plugin-a", "Plugin A");
    function Page() {
      return null;
    }

    scoped.registerRoute("/plugin-a", Page, { topbar: { title: "Custom", icon: "ticket" } });

    const route = pluginRegistry.getRoutes().find((entry) => entry.path === "/plugin-a");
    expect(route?.options).toEqual({ topbar: { title: "Custom", icon: "ticket" } });
    expect(pluginRegistry.getPluginName("plugin-a")).toBe("Plugin A");
  });

  it("clears the plugin display name on unregisterPlugin", () => {
    pluginRegistry.forPlugin("plugin-a", "Plugin A");
    expect(pluginRegistry.getPluginName("plugin-a")).toBe("Plugin A");

    pluginRegistry.unregisterPlugin("plugin-a");

    expect(pluginRegistry.getPluginName("plugin-a")).toBeUndefined();
  });
});

describe("pluginRegistry — keybinding handlers", () => {
  afterEach(() => {
    cleanup("plugin-a", "plugin-b");
  });

  it("registers a keybinding handler scoped to the owning plugin", () => {
    const scopedA = pluginRegistry.forPlugin("plugin-a");
    const scopedB = pluginRegistry.forPlugin("plugin-b");
    const handlerA = () => {};
    const handlerB = () => {};

    scopedA.registerKeybinding("open", handlerA);
    scopedB.registerKeybinding("open", handlerB);

    expect(pluginRegistry.getKeybindingHandler("plugin-a", "open")).toBe(handlerA);
    expect(pluginRegistry.getKeybindingHandler("plugin-b", "open")).toBe(handlerB);
    expect(pluginRegistry.getKeybindingHandlers()).toEqual([
      { id: "open", handler: handlerA, pluginId: "plugin-a" },
      { id: "open", handler: handlerB, pluginId: "plugin-b" },
    ]);
  });

  it("revokes keybinding handlers on unregisterPlugin", () => {
    const scoped = pluginRegistry.forPlugin("plugin-a");
    scoped.registerKeybinding("open", () => {});

    pluginRegistry.unregisterPlugin("plugin-a");

    expect(pluginRegistry.getKeybindingHandlers()).toEqual([]);
    expect(pluginRegistry.getKeybindingHandler("plugin-a", "open")).toBeUndefined();
  });

  it("warns when registering a handler for an id not declared in the manifest", () => {
    const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
    pluginRegistry.setDeclaredKeybindingIds("plugin-a", ["open"]);
    const scoped = pluginRegistry.forPlugin("plugin-a");

    scoped.registerKeybinding("not-declared", () => {});

    expect(warnSpy).toHaveBeenCalledWith(expect.stringContaining("not-declared"));
    // The handler is still stored despite the warning.
    expect(pluginRegistry.getKeybindingHandler("plugin-a", "not-declared")).toBeDefined();
    warnSpy.mockRestore();
  });

  it("does not warn when the declared id list has not been synced yet", () => {
    const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
    const scoped = pluginRegistry.forPlugin("plugin-a");

    scoped.registerKeybinding("anything", () => {});

    expect(warnSpy).not.toHaveBeenCalled();
    warnSpy.mockRestore();
  });

  it("does not warn when the id is declared in the manifest", () => {
    const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
    pluginRegistry.setDeclaredKeybindingIds("plugin-a", ["open"]);
    const scoped = pluginRegistry.forPlugin("plugin-a");

    scoped.registerKeybinding("open", () => {});

    expect(warnSpy).not.toHaveBeenCalled();
    warnSpy.mockRestore();
  });
});

function cleanupProviderContracts() {
  cleanup(PRIMARY_PLUGIN_ID, SECONDARY_PLUGIN_ID);
}

describe("pluginRegistry — repository provider contracts", () => {
  afterEach(cleanupProviderContracts);

  it("keeps repository provider ownership with its registering plugin", () => {
    const provider = repositoryProvider(SOURCE_CONTROL_PROVIDER_ID);
    pluginRegistry.forPlugin(PRIMARY_PLUGIN_ID).registerRepositoryProvider(provider);

    expect(pluginRegistry.getRepositoryProvider(SOURCE_CONTROL_PROVIDER_ID)).toMatchObject({
      pluginId: PRIMARY_PLUGIN_ID,
      id: SOURCE_CONTROL_PROVIDER_ID,
      label: SOURCE_CONTROL_PROVIDER_ID,
    });
  });

  it("rejects a repository provider not declared for its plugin when declarations are available", () => {
    pluginRegistry.setDeclaredRepositoryProviderIds(PRIMARY_PLUGIN_ID, ["declared-source-control"]);

    expect(() =>
      pluginRegistry
        .forPlugin(PRIMARY_PLUGIN_ID)
        .registerRepositoryProvider(repositoryProvider("other-source-control")),
    ).toThrow('does not declare repository provider "other-source-control"');
  });

  it("rejects duplicate active provider ownership deterministically", () => {
    pluginRegistry
      .forPlugin(PRIMARY_PLUGIN_ID)
      .registerRepositoryProvider(repositoryProvider(SOURCE_CONTROL_PROVIDER_ID));

    expect(() =>
      pluginRegistry
        .forPlugin(SECONDARY_PLUGIN_ID)
        .registerRepositoryProvider(repositoryProvider(SOURCE_CONTROL_PROVIDER_ID)),
    ).toThrow(
      `provider "${SOURCE_CONTROL_PROVIDER_ID}" is already owned by "${PRIMARY_PLUGIN_ID}"`,
    );

    expect(pluginRegistry.getRepositoryProvider(SOURCE_CONTROL_PROVIDER_ID)?.pluginId).toBe(
      PRIMARY_PLUGIN_ID,
    );
  });

  it("rejects provider IDs owned by first-party integrations", () => {
    expect(() =>
      pluginRegistry
        .forPlugin(PRIMARY_PLUGIN_ID)
        .registerRepositoryProvider(repositoryProvider("github")),
    ).toThrow('provider "github" is reserved by the host');
  });
});

describe("pluginRegistry — repository provider lifecycle", () => {
  afterEach(cleanupProviderContracts);

  it("aborts in-flight repository work when its owner unloads", async () => {
    const aborted = vi.fn();
    let markStarted: () => void;
    const started = new Promise<void>((resolve) => {
      markStarted = resolve;
    });
    const provider = repositoryProvider(SOURCE_CONTROL_PROVIDER_ID, {
      listRepositories: ({ signal }) =>
        new Promise((_, reject) => {
          markStarted();
          signal.addEventListener(
            "abort",
            () => {
              aborted();
              reject(new Error("provider request aborted"));
            },
            { once: true },
          );
        }),
    });
    pluginRegistry.forPlugin(PRIMARY_PLUGIN_ID).registerRepositoryProvider(provider);

    const request = pluginRegistry
      .getRepositoryProvider(SOURCE_CONTROL_PROVIDER_ID)
      ?.listRepositories({ workspaceId: "workspace-a", signal: new AbortController().signal });
    await started;
    pluginRegistry.unregisterPlugin(PRIMARY_PLUGIN_ID);

    await expect(request).rejects.toThrow("provider request aborted");
    expect(aborted).toHaveBeenCalledOnce();
  });

  it("keeps re-enabled provider work tracked after an older request settles", async () => {
    let rejectFirst!: (error: Error) => void;
    let markFirstStarted!: () => void;
    const firstStarted = new Promise<void>((resolve) => {
      markFirstStarted = resolve;
    });
    const first = repositoryProvider(SOURCE_CONTROL_PROVIDER_ID, {
      listRepositories: () =>
        new Promise((_, reject) => {
          rejectFirst = reject;
          markFirstStarted();
        }),
    });
    pluginRegistry.forPlugin(PRIMARY_PLUGIN_ID).registerRepositoryProvider(first);
    const firstRequest = pluginRegistry
      .getRepositoryProvider(SOURCE_CONTROL_PROVIDER_ID)!
      .listRepositories({ workspaceId: "workspace-a", signal: new AbortController().signal });
    await firstStarted;

    pluginRegistry.unregisterPlugin(PRIMARY_PLUGIN_ID);
    const secondAborted = vi.fn();
    let markSecondStarted!: () => void;
    const secondStarted = new Promise<void>((resolve) => {
      markSecondStarted = resolve;
    });
    const second = repositoryProvider(SOURCE_CONTROL_PROVIDER_ID, {
      listRepositories: ({ signal }) =>
        new Promise((_, reject) => {
          markSecondStarted();
          signal.addEventListener(
            "abort",
            () => {
              secondAborted();
              reject(new Error("second provider request aborted"));
            },
            { once: true },
          );
        }),
    });
    pluginRegistry.forPlugin(PRIMARY_PLUGIN_ID).registerRepositoryProvider(second);
    const secondRequest = pluginRegistry
      .getRepositoryProvider(SOURCE_CONTROL_PROVIDER_ID)!
      .listRepositories({ workspaceId: "workspace-a", signal: new AbortController().signal });
    await secondStarted;
    rejectFirst(new Error("first provider request stopped"));
    await expect(firstRequest).rejects.toThrow();

    pluginRegistry.unregisterPlugin(PRIMARY_PLUGIN_ID);

    await expect(secondRequest).rejects.toThrow("second provider request aborted");
    expect(secondAborted).toHaveBeenCalledOnce();
  });
});

describe("pluginRegistry — task action contracts", () => {
  afterEach(cleanupProviderContracts);

  it("registers placement-aware task actions and revokes them with their owner", () => {
    const action = {
      id: "link-change",
      label: "Link change request",
      placement: "link" as const,
      group: "Link",
      run: async () => {},
    };
    pluginRegistry.forPlugin(PRIMARY_PLUGIN_ID).registerTaskAction(action);

    expect(pluginRegistry.getTaskActions("link")).toEqual([
      { ...action, pluginId: PRIMARY_PLUGIN_ID },
    ]);

    pluginRegistry.unregisterPlugin(PRIMARY_PLUGIN_ID);
    expect(pluginRegistry.getTaskActions("link")).toEqual([]);
  });
});

describe("pluginRegistry — review provider contracts", () => {
  afterEach(cleanupProviderContracts);

  it("normalizes review items, cleans subscriptions, and aborts refresh on owner removal", async () => {
    const unsubscribe = vi.fn();
    const refreshAborted = vi.fn();
    let markRefreshStarted: () => void;
    const refreshStarted = new Promise<void>((resolve) => {
      markRefreshStarted = resolve;
    });
    const provider = reviewProvider(SOURCE_CONTROL_PROVIDER_ID, {
      getSnapshot: () => [
        {
          providerId: SOURCE_CONTROL_PROVIDER_ID,
          reviewKey: "change-1",
          title: "Change 1",
          url: CHANGE_URL,
          repositoryId: REPOSITORY_ID,
          state: "open",
          statusBadge: { label: "Checks passing", tone: "success" },
        },
        {
          providerId: "another-provider",
          reviewKey: "spoofed-change",
          title: "Spoofed",
          url: "https://example.test/changes/2",
          repositoryId: REPOSITORY_ID,
          state: "open",
        },
      ],
      subscribe: () => unsubscribe,
      refresh: (_taskId, signal) =>
        new Promise((_, reject) => {
          markRefreshStarted();
          signal.addEventListener(
            "abort",
            () => {
              refreshAborted();
              reject(new Error("review refresh aborted"));
            },
            { once: true },
          );
        }),
    });
    pluginRegistry.forPlugin(PRIMARY_PLUGIN_ID).registerReviewProvider(provider);
    const registration = pluginRegistry.getReviewProvider(SOURCE_CONTROL_PROVIDER_ID);
    if (!registration) throw new Error("review provider registration missing");

    expect(registration.getSnapshot("task-a")).toEqual([
      {
        providerId: SOURCE_CONTROL_PROVIDER_ID,
        reviewKey: "change-1",
        title: "Change 1",
        url: CHANGE_URL,
        repositoryId: REPOSITORY_ID,
        state: "open",
        statusBadge: { label: "Checks passing", tone: "success" },
      },
    ]);
    registration.subscribe("task-a", () => {});
    const refresh = registration.refresh("task-a", new AbortController().signal);
    await refreshStarted;

    pluginRegistry.unregisterPlugin(PRIMARY_PLUGIN_ID);

    await expect(refresh).rejects.toThrow("review refresh aborted");
    expect(unsubscribe).toHaveBeenCalledOnce();
    expect(refreshAborted).toHaveBeenCalledOnce();
    expect(pluginRegistry.getReviewProvider(SOURCE_CONTROL_PROVIDER_ID)).toBeUndefined();
  });

  it("preserves normalized task status and strips provider-specific fields", () => {
    pluginRegistry.forPlugin(PRIMARY_PLUGIN_ID).registerReviewProvider(
      reviewProvider(SOURCE_CONTROL_PROVIDER_ID, {
        getSnapshot: providerSpecificStatusSnapshot,
      }),
    );

    expect(
      pluginRegistry.getReviewProvider(SOURCE_CONTROL_PROVIDER_ID)?.getSnapshot("task-a"),
    ).toEqual(normalizedStatusSnapshot());
  });

  it("rejects a review provider claimed by another active plugin", () => {
    pluginRegistry
      .forPlugin(PRIMARY_PLUGIN_ID)
      .registerReviewProvider(reviewProvider(SOURCE_CONTROL_PROVIDER_ID));

    expect(() =>
      pluginRegistry
        .forPlugin(SECONDARY_PLUGIN_ID)
        .registerReviewProvider(reviewProvider(SOURCE_CONTROL_PROVIDER_ID)),
    ).toThrow(
      `provider "${SOURCE_CONTROL_PROVIDER_ID}" is already owned by "${PRIMARY_PLUGIN_ID}"`,
    );
  });
});
