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
