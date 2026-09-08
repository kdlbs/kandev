import { describe, it, expect, afterEach, vi } from "vitest";
import { pluginRegistry } from "./registry";
import type { RepositoryProviderRegistration } from "./types";
import { i18n } from "@/lib/i18n";

const TASK_SIDEBAR_SLOT = "task-sidebar";
const TASK_CREATED_ACTION = "task.created";
const APP_STATUS_LEFT_SLOT = "app-status-bar-left";
const PRIMARY_PLUGIN_ID = "plugin-a";
const SECONDARY_PLUGIN_ID = "plugin-b";
const SOURCE_CONTROL_PROVIDER_ID = "source-control";
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

  it("invalidates registration consumers when the host locale changes", async () => {
    const originalLocale = i18n.language;
    const listener = vi.fn();
    const unsubscribe = pluginRegistry.subscribe(listener);
    try {
      await i18n.changeLanguage(originalLocale === "pt-pt" ? "en" : "pt-pt");
      expect(listener).toHaveBeenCalled();
    } finally {
      unsubscribe();
      await i18n.changeLanguage(originalLocale);
    }
  });

  it("invalidates consumers when a plugin replaces or removes its translation catalog", () => {
    const listener = vi.fn();
    const unsubscribe = pluginRegistry.subscribe(listener);
    const scoped = pluginRegistry.forPlugin("plugin-a");
    try {
      scoped.registerTranslations({ en: { greeting: "Hello" } });
      expect(listener).toHaveBeenCalledTimes(1);

      scoped.registerTranslations({ en: { greeting: "Welcome" } });
      expect(listener).toHaveBeenCalledTimes(2);

      pluginRegistry.unregisterPlugin("plugin-a");
      expect(listener).toHaveBeenCalledTimes(3);
    } finally {
      unsubscribe();
    }
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

  it("reports the owning plugin for each nav item", () => {
    // Nav item ids are plugin-local, so navigation needs the owner to build a
    // unique identity (`lib/navigation/plugin-destinations.ts`).
    pluginRegistry.forPlugin("plugin-a").registerNavItem({ id: "nav", label: "A", path: "/a" });
    pluginRegistry.forPlugin("plugin-b").registerNavItem({ id: "nav", label: "B", path: "/b" });

    expect(pluginRegistry.getNavRegistrations()).toEqual([
      { pluginId: "plugin-a", id: "nav", label: "A", path: "/a" },
      { pluginId: "plugin-b", id: "nav", label: "B", path: "/b" },
    ]);
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
});

describe("pluginRegistry — slots", () => {
  afterEach(() => {
    cleanup("plugin-a", "plugin-b");
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

  it("tracks host lifecycle snapshots without extending the plugin-facing registry", () => {
    pluginRegistry.markPluginLoading("plugin-a", 3);

    expect(pluginRegistry.getPluginLifecycle("plugin-a")).toEqual({
      status: "loading",
      generation: 3,
    });
    expect("markPluginLoading" in pluginRegistry.forPlugin("plugin-a")).toBe(false);

    pluginRegistry.markPluginReady("plugin-a", 3);

    expect(pluginRegistry.getPluginLifecycle("plugin-a")).toEqual({
      status: "ready",
      generation: 3,
    });
  });

  it("does not let an older generation overwrite the current lifecycle", () => {
    pluginRegistry.markPluginLoading("plugin-a", 4);
    pluginRegistry.markPluginReady("plugin-a", 3);
    pluginRegistry.markPluginFailed("plugin-a", 3);

    expect(pluginRegistry.getPluginLifecycle("plugin-a")).toEqual({
      status: "loading",
      generation: 4,
    });
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

describe("pluginRegistry — task panels and task menu actions", () => {
  afterEach(() => {
    cleanup("plugin-a", "plugin-b");
  });

  it("registers a task panel and returns it with its owning pluginId", () => {
    const scoped = pluginRegistry.forPlugin("plugin-a");
    function Notes() {
      return null;
    }

    scoped.registerTaskPanel({ id: "notes", title: "Notes", icon: "file-text", Component: Notes });

    expect(pluginRegistry.getTaskPanels()).toEqual([
      { pluginId: "plugin-a", id: "notes", title: "Notes", icon: "file-text", Component: Notes },
    ]);
    expect(pluginRegistry.getTaskPanel("plugin-a", "notes")).toMatchObject({
      pluginId: "plugin-a",
      id: "notes",
    });
    expect(pluginRegistry.getTaskPanel("plugin-a", "missing")).toBeUndefined();
  });

  it("registers a task menu action and filters by group", () => {
    const scoped = pluginRegistry.forPlugin("plugin-a");
    const run = vi.fn();

    scoped.registerTaskMenuAction({ id: "enhance", label: "Enhance", group: "edit", run });

    expect(pluginRegistry.getTaskMenuActions("edit")).toEqual([
      { pluginId: "plugin-a", id: "enhance", label: "Enhance", group: "edit", run },
    ]);
    expect(pluginRegistry.getTaskMenuActions()).toHaveLength(1);
  });

  it("registers a 'primary' group task menu action separately from 'edit'", () => {
    const scoped = pluginRegistry.forPlugin("plugin-a");
    const editRun = vi.fn();
    const primaryRun = vi.fn();

    scoped.registerTaskMenuAction({ id: "enhance", label: "Enhance", group: "edit", run: editRun });
    scoped.registerTaskMenuAction({
      id: "quick-tag",
      label: "Tag",
      group: "primary",
      run: primaryRun,
    });

    expect(pluginRegistry.getTaskMenuActions("edit")).toEqual([
      { pluginId: "plugin-a", id: "enhance", label: "Enhance", group: "edit", run: editRun },
    ]);
    expect(pluginRegistry.getTaskMenuActions("primary")).toEqual([
      { pluginId: "plugin-a", id: "quick-tag", label: "Tag", group: "primary", run: primaryRun },
    ]);
    expect(pluginRegistry.getTaskMenuActions()).toHaveLength(2);
  });

  it("bulk-revokes task panels and task menu actions on unregisterPlugin", () => {
    const scopedA = pluginRegistry.forPlugin("plugin-a");
    const scopedB = pluginRegistry.forPlugin("plugin-b");
    function Notes() {
      return null;
    }

    scopedA.registerTaskPanel({ id: "notes", title: "Notes", Component: Notes });
    scopedA.registerTaskMenuAction({
      id: "enhance",
      label: "Enhance",
      group: "edit",
      run: () => {},
    });
    scopedB.registerTaskPanel({ id: "notes", title: "Notes", Component: Notes });

    pluginRegistry.unregisterPlugin("plugin-a");

    expect(pluginRegistry.getTaskPanels()).toEqual([
      { pluginId: "plugin-b", id: "notes", title: "Notes", icon: undefined, Component: Notes },
    ]);
    expect(pluginRegistry.getTaskMenuActions()).toEqual([]);
  });
});

describe("pluginRegistry — task filters", () => {
  afterEach(() => {
    cleanup("plugin-a", "plugin-b");
  });

  it("registers a task filter and returns it with its owning pluginId", () => {
    const scoped = pluginRegistry.forPlugin("plugin-a");
    const getOptions = () => [{ value: "bug", label: "Bug" }];
    const matches = () => true;

    scoped.registerTaskFilter({ id: "tags", label: "Tags", getOptions, matches });

    expect(pluginRegistry.getTaskFilters()).toEqual([
      { pluginId: "plugin-a", id: "tags", label: "Tags", getOptions, matches },
    ]);
  });

  it("returns task filters from multiple plugins in registration order", () => {
    const scopedA = pluginRegistry.forPlugin("plugin-a");
    const scopedB = pluginRegistry.forPlugin("plugin-b");

    scopedA.registerTaskFilter({
      id: "tags",
      label: "Tags",
      getOptions: () => [],
      matches: () => true,
    });
    scopedB.registerTaskFilter({
      id: "priority",
      label: "Priority",
      getOptions: () => [],
      matches: () => true,
    });

    expect(pluginRegistry.getTaskFilters().map((filter) => filter.id)).toEqual([
      "tags",
      "priority",
    ]);
  });

  it("bulk-revokes task filters on unregisterPlugin", () => {
    const scopedA = pluginRegistry.forPlugin("plugin-a");
    const scopedB = pluginRegistry.forPlugin("plugin-b");

    scopedA.registerTaskFilter({
      id: "tags",
      label: "Tags",
      getOptions: () => [],
      matches: () => true,
    });
    scopedB.registerTaskFilter({
      id: "priority",
      label: "Priority",
      getOptions: () => [],
      matches: () => true,
    });

    pluginRegistry.unregisterPlugin("plugin-a");

    expect(pluginRegistry.getTaskFilters().map((filter) => filter.id)).toEqual(["priority"]);
  });
});

describe("pluginRegistry — task-list facets", () => {
  afterEach(() => cleanup("plugin-a", "plugin-b"));

  it("namespaces facet identities and revokes them with their plugin", () => {
    const scoped = pluginRegistry.forPlugin("plugin-a");
    const getValues = () => [{ value: "bug", label: "Bug", color: "#f00" }];
    scoped.registerTaskListFacet({ id: "tags", label: "Tag", getValues });

    expect(pluginRegistry.getTaskListFacets()).toEqual([
      { pluginId: "plugin-a", id: "tags", label: "Tag", getValues },
    ]);
    pluginRegistry.unregisterPlugin("plugin-a");
    expect(pluginRegistry.getTaskListFacets()).toEqual([]);
  });

  it("rejects unsafe and duplicate plugin-local facet ids", () => {
    const scoped = pluginRegistry.forPlugin("plugin-a");
    expect(() =>
      scoped.registerTaskListFacet({ id: "Not safe", label: "Tag", getValues: () => [] }),
    ).toThrow("URL-safe");
    scoped.registerTaskListFacet({ id: "tags", label: "Tag", getValues: () => [] });
    expect(() =>
      scoped.registerTaskListFacet({ id: "tags", label: "Tag", getValues: () => [] }),
    ).toThrow("already registered");
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

  it("preserves lazy provider labels so locale changes do not require plugin reload", () => {
    let label = "Source control";
    const provider = repositoryProvider(SOURCE_CONTROL_PROVIDER_ID);
    Object.defineProperty(provider, "label", {
      enumerable: true,
      get: () => label,
    });
    pluginRegistry.forPlugin(PRIMARY_PLUGIN_ID).registerRepositoryProvider(provider);

    expect(pluginRegistry.getRepositoryProvider(SOURCE_CONTROL_PROVIDER_ID)?.label).toBe(
      "Source control",
    );
    label = "Controlo de código-fonte";
    expect(pluginRegistry.getRepositoryProvider(SOURCE_CONTROL_PROVIDER_ID)?.label).toBe(
      "Controlo de código-fonte",
    );
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

  it("rejects non-canonical provider IDs", () => {
    expect(() =>
      pluginRegistry
        .forPlugin(PRIMARY_PLUGIN_ID)
        .registerRepositoryProvider(repositoryProvider("Bitbucket")),
    ).toThrow('provider "Bitbucket" must be a canonical lowercase identifier');
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
