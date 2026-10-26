import { IconPlug } from "@tabler/icons-react";
import { createPortal } from "react-dom";
import { act, cleanup, createEvent, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { SidebarCustomizeMenu } from "./sidebar-customize-menu";

vi.mock("@/lib/routing/client-router", () => ({ useRouter: () => ({ push: vi.fn() }) }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (state: unknown) => unknown) =>
    select({ userSettings: { sidebarFastActionsEnabled: true, sidebarNewTaskStyle: "simple" } }),
}));
const customization = vi.hoisted(() => ({ mutate: vi.fn() }));
vi.mock("@/hooks/domains/sidebar/use-sidebar-customization", () => ({
  useSidebarCustomization: () => ({
    mutate: customization.mutate,
    preferences: vi.fn(),
    enabled: true,
    status: null,
  }),
}));

const MENU_TEST_ID = "sidebar-customize-menu";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

function mount() {
  render(
    <SidebarCustomizeMenu nodes={[]} catalog={[]}>
      <button>Navigation entry</button>
      {createPortal(
        <div contentEditable suppressContentEditableWarning data-testid="prompt">
          Draft
        </div>,
        document.body,
      )}
    </SidebarCustomizeMenu>,
  );
  const prompt = screen.getByTestId("prompt");
  const region = screen.getByTestId("sidebar-customize-region");
  expect(region.contains(prompt)).toBe(false);
  return { prompt, region };
}

// @covers AC-UI-SIDEBAR-CUSTOMIZATION-006.9
it("preserves a portaled editor context event without opening sidebar settings", () => {
  const { prompt } = mount();
  const event = createEvent.contextMenu(prompt);
  fireEvent(prompt, event);
  expect(screen.queryByTestId(MENU_TEST_ID)).toBeNull();
  expect(event.defaultPrevented).toBe(false);
});

// @covers AC-UI-SIDEBAR-CUSTOMIZATION-006.9
it.each(["touch", "pen"])("ignores held %s input in a portaled editor", (pointerType) => {
  vi.useFakeTimers();
  const { prompt } = mount();
  const event = createEvent.pointerDown(prompt, { pointerType, button: 0 });
  fireEvent(prompt, event);
  act(() => vi.advanceTimersByTime(701));
  expect(screen.queryByTestId(MENU_TEST_ID)).toBeNull();
  expect(event.defaultPrevented).toBe(false);
});

it("keeps a short prompt tap available for editing", () => {
  vi.useFakeTimers();
  const { prompt } = mount();
  const event = createEvent.pointerDown(prompt, { pointerType: "touch", button: 0 });
  fireEvent(prompt, event);
  fireEvent.pointerUp(prompt, { pointerType: "touch", button: 0 });
  act(() => vi.advanceTimersByTime(701));
  expect(screen.queryByTestId(MENU_TEST_ID)).toBeNull();
  expect(event.defaultPrevented).toBe(false);
});

// @covers AC-UI-SIDEBAR-CUSTOMIZATION-006.1
it.each(["entry", "empty space"])("opens sidebar settings from its own %s", (target) => {
  const { region } = mount();
  fireEvent.contextMenu(target === "entry" ? screen.getByRole("button") : region);
  expect(screen.getByTestId(MENU_TEST_ID)).toBeTruthy();
});

it("retains held-touch activation inside the sidebar", () => {
  vi.useFakeTimers();
  mount();
  fireEvent.pointerDown(screen.getByRole("button"), { pointerType: "touch", button: 0 });
  act(() => vi.advanceTimersByTime(701));
  expect(screen.getByTestId(MENU_TEST_ID)).toBeTruthy();
});

it.each(["pointerUp", "pointerCancel", "pointerMove"] as const)(
  "clears sidebar touch activation on %s",
  (event) => {
    vi.useFakeTimers();
    mount();
    const entry = screen.getByRole("button");
    fireEvent.pointerDown(entry, { pointerType: "touch", button: 0 });
    fireEvent[event](entry, { pointerType: "touch", button: 0 });
    act(() => vi.advanceTimersByTime(701));
    expect(screen.queryByTestId(MENU_TEST_ID)).toBeNull();
  },
);

// @covers AC-UI-SIDEBAR-CUSTOMIZATION-001.11
it("targets the integration child and limits its menu moves to siblings", () => {
  customization.mutate.mockReset();
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
  render(
    <SidebarCustomizeMenu nodes={nodes} catalog={[]}>
      <div data-sidebar-node-id="integrations">
        <a href="/a" data-sidebar-node-id="a">
          A
        </a>
      </div>
    </SidebarCustomizeMenu>,
  );
  fireEvent.contextMenu(screen.getByRole("link", { name: "A" }));
  expect(
    screen.getByRole("menuitem", { name: "Move up" }).getAttribute("data-disabled"),
  ).not.toBeNull();
  fireEvent.click(screen.getByRole("menuitem", { name: "Move down" }));
  expect(customization.mutate).toHaveBeenCalledOnce();
  const layout = { version: 1, revision: 0, nodes };
  expect(
    customization.mutate.mock.calls[0][0](layout).nodes.map((node: { id: string }) => node.id),
  ).toEqual(["b", "home", "a"]);
});
