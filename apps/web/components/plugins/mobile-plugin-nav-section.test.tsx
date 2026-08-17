import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import { MobilePluginNavSection } from "./mobile-plugin-nav-section";

const HELLO_PATH = "/plugins/hello";

vi.mock("@/lib/routing/client-router", () => ({
  usePathname: () => "/",
}));

function renderSection(onNavigate = () => {}) {
  return render(<MobilePluginNavSection onNavigate={onNavigate} />);
}

describe("MobilePluginNavSection", () => {
  afterEach(() => {
    cleanup();
    ["plugin-a", "plugin-b"].forEach((id) => pluginRegistry.unregisterPlugin(id));
    window.history.pushState({}, "", "/");
  });

  it("renders nothing when no plugin has registered a nav item", () => {
    const { container } = renderSection();
    expect(container.innerHTML).toBe("");
  });

  it("renders main-section nav items so plugin pages are reachable on a phone", () => {
    pluginRegistry
      .forPlugin("plugin-a")
      .registerNavItem({ id: "hello", label: "Hello", path: HELLO_PATH });

    renderSection();

    expect(screen.getByTestId("mobile-plugin-nav-section")).not.toBeNull();
    expect(screen.getByTestId("mobile-plugin-nav-item-hello")).not.toBeNull();
    expect(screen.getByText("Hello")).not.toBeNull();
  });

  it("treats an omitted section as main, matching the desktop sidebar", () => {
    pluginRegistry
      .forPlugin("plugin-a")
      .registerNavItem({ id: "implicit", label: "Implicit", path: "/plugins/implicit" });
    pluginRegistry.forPlugin("plugin-b").registerNavItem({
      id: "explicit",
      label: "Explicit",
      path: "/plugins/explicit",
      section: "main",
    });

    renderSection();

    expect(screen.getByTestId("mobile-plugin-nav-item-implicit")).not.toBeNull();
    expect(screen.getByTestId("mobile-plugin-nav-item-explicit")).not.toBeNull();
  });

  it("omits non-main sections: integrations belongs to MobileIntegrationsSection, and settings is rendered nowhere", () => {
    pluginRegistry.forPlugin("plugin-a").registerNavItem({
      id: "tracker",
      label: "Tracker",
      path: "/plugins/tracker",
      section: "integrations",
    });
    pluginRegistry.forPlugin("plugin-b").registerNavItem({
      id: "prefs",
      label: "Prefs",
      path: "/settings/plugins/plugin-b",
      section: "settings",
    });

    const { container } = renderSection();

    expect(container.innerHTML).toBe("");
  });

  it("also excludes sidebar-footer items: they belong to the Utilities group, not here", () => {
    pluginRegistry.forPlugin("plugin-a").registerNavItem({
      id: "board",
      label: "Board",
      path: "/plugins/board",
      section: "sidebar-footer",
    });

    const { container } = renderSection();

    expect(container.innerHTML).toBe("");
  });

  it("renders the named curated icon, falling back to the puzzle glyph", () => {
    pluginRegistry
      .forPlugin("plugin-a")
      .registerNavItem({ id: "hello", label: "Hello", path: HELLO_PATH, icon: "ticket" });
    pluginRegistry
      .forPlugin("plugin-b")
      .registerNavItem({ id: "other", label: "Other", path: "/plugins/other", icon: "nope" });

    renderSection();

    expect(
      screen.getByTestId("mobile-plugin-nav-item-hello").querySelector("svg.tabler-icon-ticket"),
    ).not.toBeNull();
    expect(
      screen.getByTestId("mobile-plugin-nav-item-other").querySelector("svg.tabler-icon-puzzle"),
    ).not.toBeNull();
  });

  it("navigates to item.path and closes the menu when tapped", () => {
    const onNavigate = vi.fn();
    pluginRegistry
      .forPlugin("plugin-a")
      .registerNavItem({ id: "hello", label: "Hello", path: HELLO_PATH });

    renderSection(onNavigate);
    screen.getByTestId("mobile-plugin-nav-item-hello").click();

    expect(window.location.pathname).toBe(HELLO_PATH);
    expect(onNavigate).toHaveBeenCalledTimes(1);
  });
});
