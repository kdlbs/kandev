import { createPortal } from "react-dom";
import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { defaultSidebarLayout } from "@/lib/sidebar/layout-types";

const DIVIDER_TEST_ID = "sidebar-navigation-divider";
const mocks = vi.hoisted(() => ({ mutate: vi.fn() }));
let saved = { navigation_height: 90, navigation_expanded: false };
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (state: unknown) => unknown) =>
    select({
      workspaces: { activeId: "a" },
      userSettings: { sidebarLayoutsByWorkspace: { a: saved } },
    }),
}));
vi.mock("@/hooks/domains/sidebar/use-sidebar-customization", () => ({
  useSidebarCustomization: () => ({ mutate: mocks.mutate, status: null }),
}));
import { SidebarNavigationSplit } from "./sidebar-navigation-split";

beforeEach(() => {
  mocks.mutate.mockClear();
  saved = { navigation_height: 90, navigation_expanded: false };
  vi.spyOn(HTMLElement.prototype, "clientHeight", "get").mockReturnValue(600);
  vi.spyOn(HTMLElement.prototype, "scrollHeight", "get").mockReturnValue(240);
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      disconnect() {}
    },
  );
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function renderSplit() {
  return render(
    <SidebarNavigationSplit navigation={<div>Navigation</div>} tasks={<div>Tasks</div>} />,
  );
}

it("retains child drafts while the rail hides split layout and controls", () => {
  const navigation = <input aria-label="Navigation draft" defaultValue="initial" />;
  const tasks = <div>Tasks</div>;
  const view = render(<SidebarNavigationSplit collapsed navigation={navigation} tasks={tasks} />);
  const draft = screen.getByRole("textbox", { name: "Navigation draft" }) as HTMLInputElement;
  fireEvent.change(draft, { target: { value: "unsaved" } });
  expect(screen.queryByTestId(DIVIDER_TEST_ID)).toBeNull();
  expect(screen.getByTestId("sidebar-navigation-split").className).toBe("contents");
  view.rerender(<SidebarNavigationSplit navigation={navigation} tasks={tasks} />);
  expect(screen.getByRole("textbox", { name: "Navigation draft" })).toBe(draft);
  expect(draft.value).toBe("unsaved");
  expect(screen.getByTestId(DIVIDER_TEST_ID)).toBeTruthy();
  view.rerender(<SidebarNavigationSplit collapsed navigation={navigation} tasks={tasks} />);
  expect(screen.getByRole("textbox", { name: "Navigation draft" })).toBe(draft);
  expect(draft.value).toBe("unsaved");
  expect(screen.queryByTestId(DIVIDER_TEST_ID)).toBeNull();
  expect(mocks.mutate).not.toHaveBeenCalled();
});

describe("sidebar split persistence", () => {
  it("expands without discarding the compressed height and restores it", () => {
    const view = renderSplit();
    expect(screen.getByTestId("sidebar-navigation-fade")).toBeTruthy();
    fireEvent.click(screen.getByTestId("sidebar-navigation-expand"));
    const operation = mocks.mutate.mock.calls[0][0];
    expect(operation({ ...defaultSidebarLayout(), navigationHeight: 90 })).toMatchObject({
      navigationHeight: 90,
      navigationExpanded: true,
    });
    saved = { navigation_height: 90, navigation_expanded: true };
    view.rerender(
      <SidebarNavigationSplit navigation={<div>Navigation</div>} tasks={<div>Tasks</div>} />,
    );
    expect(screen.queryByTestId("sidebar-navigation-fade")).toBeNull();
    fireEvent.click(screen.getByTestId("sidebar-navigation-expand"));
    expect(
      mocks.mutate.mock.calls[1][0]({
        ...defaultSidebarLayout(),
        navigationHeight: 90,
        navigationExpanded: true,
      }),
    ).toMatchObject({ navigationHeight: 90, navigationExpanded: false });
  });
  it("commits keyboard resizing but never persists measurements", () => {
    renderSplit();
    expect(mocks.mutate).not.toHaveBeenCalled();
    fireEvent.keyDown(screen.getByTestId(DIVIDER_TEST_ID), { key: "ArrowDown" });
    expect(mocks.mutate.mock.calls[0][0](defaultSidebarLayout())).toMatchObject({
      navigationHeight: 106,
      navigationExpanded: false,
    });
    expect(screen.getByTestId(DIVIDER_TEST_ID).getAttribute("aria-orientation")).toBe("horizontal");
  });
});

it("does not save a divider drag that returns to its starting height", () => {
  renderSplit();
  const divider = screen.getByTestId(DIVIDER_TEST_ID);
  divider.setPointerCapture = vi.fn();
  divider.releasePointerCapture = vi.fn();
  fireEvent.pointerDown(divider, { button: 0, isPrimary: true, pointerId: 1, clientY: 100 });
  expect(divider.setPointerCapture).toHaveBeenCalledOnce();
  fireEvent.pointerMove(divider, { pointerId: 1, clientY: 120 });
  fireEvent.pointerUp(divider, { pointerId: 1, clientY: 100 });
  expect(mocks.mutate).not.toHaveBeenCalled();
  expect(divider.releasePointerCapture).toHaveBeenCalledOnce();
  expect(document.documentElement.style.cursor).not.toBe("row-resize");
});

// @covers AC-UI-SIDEBAR-CUSTOMIZATION-007.1 AC-UI-SIDEBAR-CUSTOMIZATION-007.3
it("collapses to zero using the keyboard and preserves it across expansion", () => {
  const view = renderSplit();
  const divider = screen.getByTestId(DIVIDER_TEST_ID);
  fireEvent.keyDown(divider, { key: "Home" });
  expect(mocks.mutate.mock.calls[0][0](defaultSidebarLayout())).toMatchObject({
    navigationHeight: 0,
    navigationExpanded: false,
  });
  saved = { navigation_height: 0, navigation_expanded: false };
  view.rerender(
    <SidebarNavigationSplit navigation={<div>Navigation</div>} tasks={<div>Tasks</div>} />,
  );
  expect(divider.getAttribute("aria-valuenow")).toBe("0");
  expect(divider.getAttribute("aria-valuemin")).toBe("0");
  fireEvent.click(screen.getByTestId("sidebar-navigation-expand"));
  expect(
    mocks.mutate.mock.calls[1][0]({ ...defaultSidebarLayout(), navigationHeight: 0 }),
  ).toMatchObject({
    navigationHeight: 0,
    navigationExpanded: true,
  });
});

it("clamps pointer resizing to zero and persists the fully collapsed height", () => {
  renderSplit();
  const divider = screen.getByTestId(DIVIDER_TEST_ID);
  divider.setPointerCapture = vi.fn();
  divider.releasePointerCapture = vi.fn();
  fireEvent.pointerDown(divider, { button: 0, isPrimary: true, pointerId: 1, clientY: 200 });
  fireEvent.pointerMove(divider, { pointerId: 1, clientY: 0 });
  expect(divider.getAttribute("aria-valuenow")).toBe("0");
  fireEvent.pointerUp(divider, { pointerId: 1, clientY: 0 });
  expect(mocks.mutate.mock.calls[0][0](defaultSidebarLayout())).toMatchObject({
    navigationHeight: 0,
    navigationExpanded: false,
  });
});

it("expands for clipped navigation focus but ignores portaled menu focus", () => {
  render(
    <SidebarNavigationSplit
      navigation={
        <>
          <button>Navigation item</button>
          {createPortal(<button>Menu item</button>, document.body)}
        </>
      }
      tasks={<div>Tasks</div>}
    />,
  );
  const navigation = screen.getByRole("button", { name: "Navigation item" });
  const menu = screen.getByRole("button", { name: "Menu item" });
  vi.spyOn(navigation, "getBoundingClientRect").mockReturnValue({ bottom: 200 } as DOMRect);
  vi.spyOn(menu, "getBoundingClientRect").mockReturnValue({ bottom: 200 } as DOMRect);
  fireEvent.focus(navigation);
  expect(mocks.mutate).toHaveBeenCalledOnce();
  mocks.mutate.mockClear();
  fireEvent.focus(menu);
  expect(mocks.mutate).not.toHaveBeenCalled();
});
