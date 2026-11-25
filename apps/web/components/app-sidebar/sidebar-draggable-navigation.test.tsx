import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { defaultSidebarLayout } from "@/lib/sidebar/layout-types";

const mocks = vi.hoisted(() => ({ mutate: vi.fn() }));
vi.mock("@/hooks/domains/sidebar/use-sidebar-customization", () => ({
  useSidebarCustomization: () => ({ mutate: mocks.mutate, status: null, enabled: true }),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (state: { workspaces: { activeId: string } }) => unknown) =>
    select({ workspaces: { activeId: "workspace" } }),
}));
import { SidebarDraggableNavigation } from "./sidebar-draggable-navigation";
const rows = [
  { id: "new-task", content: <button>New Task</button> },
  { id: "home", content: <button>Home</button> },
];
beforeEach(() => mocks.mutate.mockReset());
afterEach(cleanup);

it("reorders the focused primary entry with Alt and arrow keys", () => {
  render(<SidebarDraggableNavigation rows={rows} catalog={[]} />);
  fireEvent.keyDown(screen.getByRole("button", { name: "Home" }), { key: "ArrowUp", altKey: true });
  expect(mocks.mutate).toHaveBeenCalledOnce();
  const next = mocks.mutate.mock.calls[0][0](defaultSidebarLayout());
  expect(next.nodes[0].id).toBe("home");
});

it("keeps the collapsed rail order unchanged", () => {
  render(<SidebarDraggableNavigation rows={rows} catalog={[]} disabled />);
  fireEvent.keyDown(screen.getByRole("button", { name: "Home" }), { key: "ArrowUp", altKey: true });
  expect(mocks.mutate).not.toHaveBeenCalled();
});

// @covers AC-UI-SIDEBAR-CUSTOMIZATION-001.11
it("moves a nested integration once without moving its parent", () => {
  const children = [
    { id: "plugin-a", content: <a href="/a">Plugin A</a> },
    { id: "plugin-b", content: <a href="/b">Plugin B</a> },
  ];
  render(
    <SidebarDraggableNavigation
      catalog={[]}
      rows={[
        {
          id: "integrations",
          content: (
            <>
              <button>Integrations</button>
              <SidebarDraggableNavigation catalog={[]} rows={children} />
            </>
          ),
        },
        { id: "home", content: <button>Home</button> },
      ]}
    />,
  );
  fireEvent.keyDown(screen.getByRole("link", { name: "Plugin B" }), {
    key: "ArrowUp",
    altKey: true,
  });
  expect(mocks.mutate).toHaveBeenCalledOnce();
  const layout = {
    ...defaultSidebarLayout(),
    nodes: [
      {
        id: "integrations",
        kind: "builtin" as const,
        visible: true,
        destinationId: "integrations",
      },
      {
        id: "plugin-a",
        kind: "plugin" as const,
        visible: true,
        pluginSection: "integrations" as const,
      },
      {
        id: "plugin-b",
        kind: "plugin" as const,
        visible: true,
        pluginSection: "integrations" as const,
      },
      { id: "home", kind: "builtin" as const, visible: true, destinationId: "home" },
    ],
  };
  const next = mocks.mutate.mock.calls[0][0](layout);
  expect(next.nodes.map((node: { id: string }) => node.id)).toEqual([
    "integrations",
    "plugin-b",
    "plugin-a",
    "home",
  ]);
});

// Touch and pen long-press events must reach the outer customization trigger.
it.each(["touch", "pen"])("forwards %s pointer input through a draggable row", (pointerType) => {
  const onPointerDown = vi.fn();
  render(
    <div onPointerDown={onPointerDown}>
      <SidebarDraggableNavigation rows={rows} catalog={[]} />
    </div>,
  );
  fireEvent.pointerDown(screen.getByRole("button", { name: "Home" }), {
    pointerType,
    button: 0,
    isPrimary: true,
  });
  expect(onPointerDown).toHaveBeenCalledOnce();
});
