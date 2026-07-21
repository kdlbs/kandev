import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { AppStatusDrawerTrigger, AppStatusSurfaceProvider } from "./app-status-surface-provider";

const responsiveState = vi.hoisted(() => ({ isMobile: false }));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    breakpoint: responsiveState.isMobile ? "mobile" : "desktop",
    isMobile: responsiveState.isMobile,
    isTablet: false,
    isDesktop: !responsiveState.isMobile,
    isCompactDesktop: false,
    isFullDesktop: !responsiveState.isMobile,
    isFinePointer: !responsiveState.isMobile,
    usesDesktopWorkbench: !responsiveState.isMobile,
  }),
}));

vi.mock("./app-status-bar", () => ({
  AppStatusBar: () => <div data-testid="app-status-bar" />,
}));

vi.mock("./app-status-drawer", () => ({
  AppStatusDrawer: ({ open }: { open: boolean }) => (
    <div data-testid="app-status-drawer">{String(open)}</div>
  ),
}));

function renderSurface() {
  return render(
    <StateProvider>
      <AppStatusSurfaceProvider>
        <AppStatusDrawerTrigger />
      </AppStatusSurfaceProvider>
    </StateProvider>,
  );
}

describe("AppStatusSurfaceProvider", () => {
  beforeEach(() => {
    responsiveState.isMobile = false;
  });

  afterEach(cleanup);

  it("mounts only desktop status bar outside phone breakpoint", () => {
    renderSurface();

    expect(screen.getByTestId("app-status-bar")).toBeTruthy();
    expect(screen.queryByTestId("app-status-drawer")).toBeNull();
  });

  it("mounts only phone drawer and opens it from native trigger", () => {
    responsiveState.isMobile = true;
    renderSurface();

    expect(screen.queryByTestId("app-status-bar")).toBeNull();
    expect(screen.getByTestId("app-status-drawer").textContent).toBe("false");

    fireEvent.click(screen.getByRole("button", { name: "Open status" }));
    expect(screen.getByTestId("app-status-drawer").textContent).toBe("true");
  });
});
