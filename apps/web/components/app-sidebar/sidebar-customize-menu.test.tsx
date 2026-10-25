import { createPortal } from "react-dom";
import { act, cleanup, createEvent, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { SidebarCustomizeMenu } from "./sidebar-customize-menu";

vi.mock("@/lib/routing/client-router", () => ({ useRouter: () => ({ push: vi.fn() }) }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (state: unknown) => unknown) =>
    select({ userSettings: { sidebarFastActionsEnabled: true, sidebarNewTaskStyle: "simple" } }),
}));
vi.mock("@/hooks/domains/sidebar/use-sidebar-customization", () => ({
  useSidebarCustomization: () => ({
    mutate: vi.fn(),
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
