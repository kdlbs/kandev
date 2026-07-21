import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { AppStatusDrawerTrigger, AppStatusSurfaceProvider } from "./app-status-surface-provider";

const responsiveState = vi.hoisted(() => ({ isMobile: false }));
const featureState = vi.hoisted(() => ({ appStatusBar: true }));
const STATUS_DRAWER_TEST_ID = "app-status-drawer";

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

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: (name: string) => (name === "appStatusBar" ? featureState.appStatusBar : true),
}));

vi.mock("./app-status-bar", () => ({
  AppStatusBar: () => <div data-testid="app-status-bar" />,
}));

vi.mock("./app-status-drawer", () => ({
  AppStatusDrawer: ({ open }: { open: boolean }) => (
    <div data-testid={STATUS_DRAWER_TEST_ID}>{String(open)}</div>
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
    featureState.appStatusBar = true;
  });

  afterEach(cleanup);

  it("mounts only desktop status bar outside phone breakpoint", () => {
    renderSurface();

    expect(screen.getByTestId("app-status-bar")).toBeTruthy();
    expect(screen.queryByTestId(STATUS_DRAWER_TEST_ID)).toBeNull();
  });

  it("mounts only phone drawer and opens it from native trigger", () => {
    responsiveState.isMobile = true;
    renderSurface();

    expect(screen.queryByTestId("app-status-bar")).toBeNull();
    expect(screen.getByTestId(STATUS_DRAWER_TEST_ID).textContent).toBe("false");

    fireEvent.click(screen.getByRole("button", { name: "Open status" }));
    expect(screen.getByTestId(STATUS_DRAWER_TEST_ID).textContent).toBe("true");
  });

  it("hides both presentations when the app-status-bar feature is disabled", () => {
    responsiveState.isMobile = true;
    featureState.appStatusBar = false;
    renderSurface();

    expect(screen.queryByTestId("app-status-bar")).toBeNull();
    expect(screen.queryByTestId(STATUS_DRAWER_TEST_ID)).toBeNull();
    expect(screen.queryByTestId("app-status-drawer-trigger")).toBeNull();
  });
});
