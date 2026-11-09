import { cleanup, render, screen, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { projectSidebarLayout } from "@/lib/sidebar/layout-projection";
import { defaultSidebarLayout } from "@/lib/sidebar/layout-types";
import { pluginDestinations } from "@/lib/navigation/plugin-destinations";
import { pluginRegistry } from "@/lib/plugins/registry";

const state = vi.hoisted(() => ({
  workspaces: { activeId: "ws" },
  userSettings: { sidebarFastActionsEnabled: true },
}));
const registry = pluginRegistry;
registry
  .forPlugin("youtrack")
  .registerNavItem({ id: "issues", label: "YouTrack", path: "/youtrack", section: "integrations" });
const destinations = pluginDestinations(registry.getNavRegistrations());
const catalog = destinations.map((entry) => ({
  ...entry,
  href: typeof entry.href === "string" ? entry.href : undefined,
  label: entry.label ?? entry.id,
  target: { kind: "destination" as const, id: entry.id },
  available: true,
}));
const projection = projectSidebarLayout(defaultSidebarLayout(), catalog);
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));
vi.mock("@/lib/routing/client-router", () => ({ usePathname: () => "/" }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false, isFinePointer: true }),
}));
vi.mock("@/hooks/use-quick-chat-launcher", () => ({ useQuickChatLauncher: () => vi.fn() }));
vi.mock("@/hooks/use-quick-terminal-launcher", () => ({ useQuickTerminalLauncher: () => vi.fn() }));
vi.mock("@/hooks/domains/sidebar/use-sidebar-layout-navigation", () => ({
  useSidebarLayoutNavigation: () => ({
    workspaceId: "ws",
    catalog: { catalog },
    projection,
    activity: { getActivity: vi.fn() },
  }),
}));
vi.mock("@/hooks/use-app-destinations", () => ({ useAppDestinations: () => destinations }));
vi.mock("./sidebar-customize-menu", () => ({
  SidebarCustomizeMenu: ({ children }: { children: ReactNode }) => <>{children}</>,
}));
vi.mock("./sidebar-draggable-navigation", () => ({
  SidebarDraggableNavigation: ({ rows }: { rows: { id: string; content: ReactNode }[] }) => (
    <>
      {rows.map((row) => (
        <div key={row.id} data-testid={`sidebar-node-${row.id}`}>
          {row.content}
        </div>
      ))}
    </>
  ),
}));
vi.mock("./app-sidebar-section", () => ({
  AppSidebarSection: ({ children }: { children: ReactNode }) => (
    <section data-testid="integration-body">{children}</section>
  ),
}));
vi.mock("./app-sidebar-primary-nav", () => ({
  AppSidebarFixedNav: () => null,
  AppSidebarHomeItem: () => null,
}));
vi.mock("./app-sidebar-new-task-item", () => ({ AppSidebarNewTaskItem: () => null }));
vi.mock("./sections/automations-section", () => ({ AutomationsSection: () => null }));
vi.mock("./sections/coordinators-section", () => ({ CoordinatorsSection: () => null }));
vi.mock("./sections/canvases-section", () => ({ CanvasesSection: () => null }));
vi.mock("./shortcut-section", () => ({ ShortcutSection: () => null }));
import { SidebarLayoutNavigation } from "./sidebar-layout-navigation";

afterEach(() => {
  cleanup();
  projection.nodes.forEach((node) => {
    node.visible = true;
  });
});
describe("integration plugin layout wiring", () => {
  it("hides children with their parent while retaining the child choice", () => {
    projection.nodes.find((node) => node.destinationId === "integrations")!.visible = false;
    render(<SidebarLayoutNavigation collapsed={false} inOffice={false} />);
    expect(screen.queryByRole("link", { name: "YouTrack" })).toBeNull();
    expect(projection.nodes.find((node) => node.kind === "plugin")!.visible).toBe(true);
  });
  it("preserves standalone plugin access in Office mode", () => {
    render(<SidebarLayoutNavigation collapsed={false} inOffice />);
    expect(screen.getAllByRole("link", { name: "YouTrack" })).toHaveLength(1);
    expect(screen.queryByTestId("integration-body")).toBeNull();
  });
  // @covers AC-UI-SIDEBAR-CUSTOMIZATION-001.10
  it("renders a registered integration inside Integrations exactly once", () => {
    state.userSettings.sidebarFastActionsEnabled = false;
    render(<SidebarLayoutNavigation collapsed={false} inOffice={false} />);
    expect(
      within(screen.getByTestId("integration-body")).getByRole("link", { name: "YouTrack" }),
    ).toBeTruthy();
    expect(screen.getAllByRole("link", { name: "YouTrack" })).toHaveLength(1);
  });
});
