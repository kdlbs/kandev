import { describe, it, expect, afterEach } from "vitest";
import { pluginRegistry } from "./registry";

const TASK_SIDEBAR_SLOT = "task-sidebar";
const TASK_CREATED_ACTION = "task.created";

function cleanup(...pluginIds: string[]) {
  pluginIds.forEach((id) => pluginRegistry.unregisterPlugin(id));
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
});
