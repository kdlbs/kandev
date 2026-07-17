import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import { PluginNavItems } from "./plugin-nav-items";

let pathname = "/";
let pluginsEnabled = true;
const HELLO_PATH = "/plugins/hello";

function cleanupPlugins(...pluginIds: string[]) {
  pluginIds.forEach((id) => pluginRegistry.unregisterPlugin(id));
}

vi.mock("@/lib/routing/client-router", () => ({
  usePathname: () => pathname,
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => pluginsEnabled,
}));

function renderNavItems(collapsed = false) {
  return render(
    <TooltipProvider>
      <PluginNavItems collapsed={collapsed} />
    </TooltipProvider>,
  );
}

describe("PluginNavItems", () => {
  afterEach(() => {
    cleanup();
    cleanupPlugins("plugin-a", "plugin-b");
    pathname = "/";
    pluginsEnabled = true;
    window.history.pushState({}, "", "/");
  });

  it("renders nothing when no plugin has registered a nav item", () => {
    const { container } = renderNavItems();
    expect(container.innerHTML).toBe("");
  });

  it("renders a registered main-section nav item", () => {
    pluginRegistry
      .forPlugin("plugin-a")
      .registerNavItem({ id: "hello", label: "Hello", path: HELLO_PATH });

    renderNavItems();

    expect(screen.getByTestId("plugin-nav-item-hello")).not.toBeNull();
    expect(screen.getByText("Hello")).not.toBeNull();
  });

  it("omits a nav item registered for a non-main section", () => {
    pluginRegistry.forPlugin("plugin-a").registerNavItem({
      id: "settings-item",
      label: "Settings Item",
      path: "/settings/plugins/plugin-a",
      section: "settings",
    });

    renderNavItems();

    expect(screen.queryByTestId("plugin-nav-item-settings-item")).toBeNull();
  });

  it("renders nothing when the plugins feature flag is off, even with a registered nav item", () => {
    pluginsEnabled = false;
    pluginRegistry
      .forPlugin("plugin-a")
      .registerNavItem({ id: "hello", label: "Hello", path: HELLO_PATH });

    const { container } = renderNavItems();

    expect(container.innerHTML).toBe("");
  });

  it("navigates to item.path when clicked", () => {
    pluginRegistry
      .forPlugin("plugin-a")
      .registerNavItem({ id: "hello", label: "Hello", path: HELLO_PATH });

    renderNavItems();
    screen.getByTestId("plugin-nav-item-hello").click();

    expect(window.location.pathname).toBe(HELLO_PATH);
  });
});
