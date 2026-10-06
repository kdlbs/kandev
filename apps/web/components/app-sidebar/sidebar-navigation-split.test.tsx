import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { defaultSidebarLayout } from "@/lib/sidebar/layout-types";

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
    fireEvent.keyDown(screen.getByTestId("sidebar-navigation-divider"), { key: "ArrowDown" });
    expect(mocks.mutate.mock.calls[0][0](defaultSidebarLayout())).toMatchObject({
      navigationHeight: 106,
      navigationExpanded: false,
    });
    expect(screen.getByTestId("sidebar-navigation-divider").getAttribute("aria-orientation")).toBe(
      "horizontal",
    );
  });
});

it("does not save a divider drag that returns to its starting height", () => {
  renderSplit();
  const divider = screen.getByTestId("sidebar-navigation-divider");
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
