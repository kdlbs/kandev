import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import { PluginNavItems } from "./plugin-nav-items";

let pathname = "/";
const HELLO_PATH = "/plugins/hello";

function cleanupPlugins(...pluginIds: string[]) {
  pluginIds.forEach((id) => pluginRegistry.unregisterPlugin(id));
}

vi.mock("@/lib/routing/client-router", () => ({
  usePathname: () => pathname,
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

  it("keeps integration items out of the main plugin rail", () => {
    pluginRegistry.forPlugin("plugin-a").registerNavItem({
      id: "integration-item",
      label: "Integration Item",
      path: "/plugins/integration-item",
      section: "integrations",
    });

    renderNavItems();

    expect(screen.queryByTestId("plugin-nav-item-integration-item")).toBeNull();
  });

  it("renders the named curated icon, falling back to the puzzle glyph", () => {
    pluginRegistry
      .forPlugin("plugin-a")
      .registerNavItem({ id: "hello", label: "Hello", path: HELLO_PATH, icon: "ticket" });
    pluginRegistry
      .forPlugin("plugin-b")
      .registerNavItem({ id: "other", label: "Other", path: "/plugins/other", icon: "nope" });

    renderNavItems();

    expect(
      screen.getByTestId("plugin-nav-item-hello").querySelector("svg.tabler-icon-ticket"),
    ).not.toBeNull();
    expect(
      screen.getByTestId("plugin-nav-item-other").querySelector("svg.tabler-icon-puzzle"),
    ).not.toBeNull();
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
