import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import {
  MobilePluginNavSection,
  type MobilePluginWorkspaceContext,
} from "./mobile-plugin-nav-section";

const HELLO_PATH = "/plugins/hello";
const HELLO_ITEM_TEST_ID = "mobile-plugin-nav-item-hello";

vi.mock("@/lib/routing/client-router", () => ({
  usePathname: () => "/",
}));

function renderSection(
  onNavigate = () => {},
  actions?: ReactNode,
  includeDestinations = true,
  workspaceContext?: MobilePluginWorkspaceContext,
) {
  return render(
    <MobilePluginNavSection
      actions={actions}
      includeDestinations={includeDestinations}
      workspaceContext={workspaceContext}
      onNavigate={onNavigate}
    />,
  );
}

afterEach(() => {
  cleanup();
  ["plugin-a", "plugin-b"].forEach((id) => pluginRegistry.unregisterPlugin(id));
  window.history.pushState({}, "", "/");
});

describe("MobilePluginNavSection workspace grouping", () => {
  const workspaceContext = {
    workspaceId: "ws-1",
    workspaceLabel: "Demo",
    currentPage: "tasks",
  } as const;

  it("omits the section when the phone has no contributions", () => {
    const { container } = renderSection(undefined, undefined, true, workspaceContext);
    expect(container.innerHTML).toBe("");
  });

  it("keeps workspace slots with task controls even when a saved layout owns destinations", () => {
    const received: unknown[] = [];
    for (const name of ["main-top-bar", "sidebar-workspace-actions"]) {
      pluginRegistry.forPlugin("plugin-a").registerComponent(name, ({ slotProps }) => {
        received.push(slotProps);
        return <button data-testid={name}>{name}</button>;
      });
    }
    pluginRegistry
      .forPlugin("plugin-a")
      .registerNavItem({ id: "hello", label: "Hello", path: HELLO_PATH });
    renderSection(undefined, <button>Task control</button>, false, workspaceContext);

    const section = screen.getByRole("region", { name: "Plugins" });
    for (const label of ["main-top-bar", "sidebar-workspace-actions", "Task control"]) {
      expect(section.contains(screen.getByRole("button", { name: label }))).toBe(true);
    }
    expect(received).toContainEqual({ ...workspaceContext, presentation: "mobile" });
    expect(received).toContainEqual({
      workspaceId: "ws-1",
      workspaceLabel: "Demo",
      presentation: "mobile",
    });
    expect(screen.getByRole("heading", { name: "Workspace" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Task" })).toBeTruthy();
    expect(screen.queryByTestId(HELLO_ITEM_TEST_ID)).toBeNull();
    expect(screen.getAllByText("Plugins")).toHaveLength(1);
  });

  it("does not invent task context for workspace-only controls and removes unloaded contributions", () => {
    pluginRegistry
      .forPlugin("plugin-a")
      .registerComponent("main-top-bar", () => <button>Workspace control</button>);
    const { rerender, container } = renderSection(undefined, undefined, true, workspaceContext);
    expect(screen.getByRole("button", { name: "Workspace control" })).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Task" })).toBeNull();
    pluginRegistry.unregisterPlugin("plugin-a");
    rerender(<MobilePluginNavSection onNavigate={() => {}} workspaceContext={workspaceContext} />);
    expect(container.innerHTML).toBe("");
  });

  it("omits sidebar actions when there is no active workspace", () => {
    pluginRegistry
      .forPlugin("plugin-a")
      .registerComponent("sidebar-workspace-actions", () => <button>Workspace control</button>);
    const { container } = renderSection(undefined, undefined, true, { currentPage: "kanban" });
    expect(container.innerHTML).toBe("");
  });
});

describe("MobilePluginNavSection actions", () => {
  it("renders nothing when no plugin has registered a nav item", () => {
    const { container } = renderSection();
    expect(container.innerHTML).toBe("");
  });

  it("renders page actions without requiring a plugin destination", () => {
    const onNavigate = vi.fn();
    renderSection(
      onNavigate,
      <button type="button" data-testid="session-plugin-action">
        Session action
      </button>,
    );

    const section = screen.getByTestId("mobile-plugin-nav-section");
    const action = screen.getByTestId("session-plugin-action");
    expect(section.contains(action)).toBe(true);
    action.click();
    expect(onNavigate).not.toHaveBeenCalled();
  });

  it("combines page actions and destinations under one Plugins heading", () => {
    pluginRegistry
      .forPlugin("plugin-a")
      .registerNavItem({ id: "hello", label: "Hello", path: HELLO_PATH });

    renderSection(() => {}, <span data-testid="session-plugin-status">Connected</span>);

    expect(screen.getAllByText("Plugins")).toHaveLength(1);
    const action = screen.getByTestId("session-plugin-status");
    const destination = screen.getByTestId(HELLO_ITEM_TEST_ID);
    expect(
      action.compareDocumentPosition(destination) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });
});

describe("MobilePluginNavSection destinations", () => {
  it("renders main-section nav items so plugin pages are reachable on a phone", () => {
    pluginRegistry
      .forPlugin("plugin-a")
      .registerNavItem({ id: "hello", label: "Hello", path: HELLO_PATH });

    renderSection();

    expect(screen.getByTestId("mobile-plugin-nav-section")).not.toBeNull();
    expect(screen.getByTestId(HELLO_ITEM_TEST_ID)).not.toBeNull();
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
      screen.getByTestId(HELLO_ITEM_TEST_ID).querySelector("svg.tabler-icon-ticket"),
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
    screen.getByTestId(HELLO_ITEM_TEST_ID).click();

    expect(window.location.pathname).toBe(HELLO_PATH);
    expect(onNavigate).toHaveBeenCalledTimes(1);
  });
});
