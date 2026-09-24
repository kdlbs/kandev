import { act, cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import { PluginSlot } from "./plugin-slot";
import {
  MobilePluginNavSection,
  type MobilePluginWorkspaceContext,
} from "./mobile-plugin-nav-section";

const HELLO_PATH = "/plugins/hello";
const HELLO_ITEM_TEST_ID = "mobile-plugin-nav-item-hello";
const MAIN_SLOT = "main-top-bar";
const SIDEBAR_SLOT = "sidebar-workspace-actions";
const WORKSPACE_USAGE = "Workspace usage";

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
    for (const name of [MAIN_SLOT, SIDEBAR_SLOT]) {
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
    for (const label of [MAIN_SLOT, SIDEBAR_SLOT, "Task control"]) {
      expect(section.contains(screen.getByRole("button", { name: label }))).toBe(true);
    }
    expect(received).toContainEqual({ ...workspaceContext, presentation: "mobile" });
    expect(received).toContainEqual({
      workspaceId: "ws-1",
      workspaceLabel: "Demo",
      presentation: "mobile",
    });
    expect(screen.queryByRole("heading", { name: "Workspace" })).toBeNull();
    expect(screen.queryByRole("heading", { name: "Task" })).toBeNull();
    expect(screen.queryByTestId(HELLO_ITEM_TEST_ID)).toBeNull();
    expect(screen.getAllByText("Plugins")).toHaveLength(1);
  });

  // @covers AC-UI-MOBILE-MENU-008.1
  it("prefers a plugin's task toolbar while retaining every task and independent workspace action", () => {
    const plugin = pluginRegistry.forPlugin("plugin-a");
    plugin.registerComponent(MAIN_SLOT, () => <button>Workspace usage</button>);
    plugin.registerComponent("chat-top-bar", () => <button>Task usage</button>);
    plugin.registerComponent("chat-top-bar", () => <button>Task details</button>);
    plugin.registerComponent(SIDEBAR_SLOT, () => <button>New workspace note</button>);
    pluginRegistry.forPlugin("plugin-b").registerComponent(MAIN_SLOT, () => <button>CPU</button>);

    renderSection(undefined, <PluginSlot name="chat-top-bar" />, false, workspaceContext);

    expect(screen.queryByRole("button", { name: WORKSPACE_USAGE })).toBeNull();
    for (const name of ["Task usage", "Task details", "New workspace note", "CPU"]) {
      expect(screen.getAllByRole("button", { name })).toHaveLength(1);
    }
  });

  // @covers AC-UI-MOBILE-MENU-008.2
  it("restores the workspace toolbar when task actions are absent and follows live registrations", () => {
    const plugin = pluginRegistry.forPlugin("plugin-a");
    plugin.registerComponent(MAIN_SLOT, () => <button>Workspace usage</button>);
    const { rerender } = renderSection(
      undefined,
      <PluginSlot name="chat-top-bar" />,
      true,
      workspaceContext,
    );
    expect(screen.getByRole("button", { name: WORKSPACE_USAGE })).toBeTruthy();

    act(() => plugin.registerComponent("chat-top-bar", () => <button>Task usage</button>));
    expect(screen.queryByRole("button", { name: WORKSPACE_USAGE })).toBeNull();
    expect(screen.getByRole("button", { name: "Task usage" })).toBeTruthy();

    rerender(<MobilePluginNavSection onNavigate={() => {}} workspaceContext={workspaceContext} />);
    expect(screen.getByRole("button", { name: WORKSPACE_USAGE })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Task usage" })).toBeNull();

    act(() => pluginRegistry.unregisterPlugin("plugin-a"));
    expect(screen.queryByTestId("mobile-plugin-nav-section")).toBeNull();
  });

  it("does not invent task context for workspace-only controls and removes unloaded contributions", () => {
    pluginRegistry
      .forPlugin("plugin-a")
      .registerComponent(MAIN_SLOT, () => <button>Workspace control</button>);
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
      .registerComponent(SIDEBAR_SLOT, () => <button>Workspace control</button>);
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
