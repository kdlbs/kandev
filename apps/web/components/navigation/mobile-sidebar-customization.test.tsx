import type { ReactNode } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { IconPlug } from "@tabler/icons-react";
import { afterEach, expect, it, vi } from "vitest";
import { MobileSidebarCustomization } from "./mobile-sidebar-customization";
const mocks = vi.hoisted(() => ({ mutate: vi.fn() }));
vi.mock("@/lib/routing/client-router", () => ({ useRouter: () => ({ push: vi.fn() }) }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (s: unknown) => unknown) =>
    select({ userSettings: { sidebarFastActionsEnabled: false, sidebarNewTaskStyle: "simple" } }),
}));
vi.mock("@/hooks/domains/sidebar/use-sidebar-customization", () => ({
  useSidebarCustomization: () => ({
    mutate: mocks.mutate,
    preferences: vi.fn(),
    enabled: true,
    status: null,
  }),
}));
vi.mock("@kandev/ui/drawer", () => {
  const Wrapper = ({ children }: { children: ReactNode }) => <div>{children}</div>;
  return {
    DrawerNested: Wrapper,
    DrawerTrigger: Wrapper,
    DrawerContent: Wrapper,
    DrawerHeader: Wrapper,
    DrawerTitle: Wrapper,
    DrawerClose: Wrapper,
  };
});
afterEach(() => {
  cleanup();
  mocks.mutate.mockReset();
});
// @covers AC-UI-SIDEBAR-CUSTOMIZATION-001.11, AC-UI-SIDEBAR-CUSTOMIZATION-006.7
it("moves integration siblings from phone controls without moving top-level entries", () => {
  const nodes = [
    {
      id: "a",
      kind: "plugin" as const,
      pluginSection: "integrations" as const,
      visible: true,
      label: "A",
      icon: IconPlug,
      shortcuts: [],
    },
    {
      id: "home",
      kind: "builtin" as const,
      visible: true,
      label: "Home",
      icon: IconPlug,
      shortcuts: [],
    },
    {
      id: "b",
      kind: "plugin" as const,
      pluginSection: "integrations" as const,
      visible: true,
      label: "B",
      icon: IconPlug,
      shortcuts: [],
    },
  ];
  render(<MobileSidebarCustomization nodes={nodes} catalog={[]} onNavigate={() => {}} />);
  const down = screen.getByRole("button", { name: "Move A down" });
  fireEvent.click(down);
  expect(mocks.mutate).toHaveBeenCalledOnce();
  const layout = { version: 1, revision: 0, nodes };
  expect(
    mocks.mutate.mock.calls[0][0](layout).nodes.map((node: { id: string }) => node.id),
  ).toEqual(["b", "home", "a"]);
  expect(screen.getByRole("button", { name: "Move A up" }).hasAttribute("disabled")).toBe(true);
  expect(screen.getByRole("button", { name: "Move Home down" }).hasAttribute("disabled")).toBe(
    true,
  );
});
