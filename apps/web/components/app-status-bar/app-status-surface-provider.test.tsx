import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { AppStatusDrawerTrigger, AppStatusSurfaceProvider } from "./app-status-surface-provider";

const responsiveState = vi.hoisted(() => ({
  breakpoint: "desktop" as "mobile" | "tablet" | "desktop",
}));
const featureState = vi.hoisted(() => ({ appStatusBar: true }));
const STATUS_BAR_TEST_ID = "app-status-bar";
const STATUS_DRAWER_TEST_ID = "app-status-drawer";

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    breakpoint: responsiveState.breakpoint,
    isMobile: responsiveState.breakpoint === "mobile",
    isTablet: responsiveState.breakpoint === "tablet",
    isDesktop: responsiveState.breakpoint === "desktop",
    isCompactDesktop: false,
    isFullDesktop: responsiveState.breakpoint === "desktop",
    isFinePointer: responsiveState.breakpoint !== "mobile",
    usesDesktopWorkbench: responsiveState.breakpoint === "desktop",
  }),
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: (name: string) => (name === "appStatusBar" ? featureState.appStatusBar : true),
}));

vi.mock("./app-status-bar", () => ({
  AppStatusBar: () => <div data-testid={STATUS_BAR_TEST_ID} />,
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
    responsiveState.breakpoint = "desktop";
    featureState.appStatusBar = true;
  });

  afterEach(cleanup);

  it("mounts only desktop status bar outside phone breakpoint", () => {
    renderSurface();

    expect(screen.getByTestId(STATUS_BAR_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId(STATUS_DRAWER_TEST_ID)).toBeNull();
  });

  it("mounts only phone drawer and opens it from native trigger", () => {
    responsiveState.breakpoint = "mobile";
    renderSurface();

    expect(screen.queryByTestId(STATUS_BAR_TEST_ID)).toBeNull();
    expect(screen.getByTestId(STATUS_DRAWER_TEST_ID).textContent).toBe("false");

    fireEvent.click(screen.getByRole("button", { name: "Open status" }));
    expect(screen.getByTestId(STATUS_DRAWER_TEST_ID).textContent).toBe("true");
  });

  it("hides both presentations when the app-status-bar feature is disabled", () => {
    responsiveState.breakpoint = "mobile";
    featureState.appStatusBar = false;
    renderSurface();

    expect(screen.queryByTestId(STATUS_BAR_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(STATUS_DRAWER_TEST_ID)).toBeNull();
    expect(screen.queryByTestId("app-status-drawer-trigger")).toBeNull();
  });

  it("does not expose a drawer trigger at the tablet breakpoint", () => {
    responsiveState.breakpoint = "tablet";
    renderSurface();

    expect(screen.getByTestId(STATUS_BAR_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId(STATUS_DRAWER_TEST_ID)).toBeNull();
    expect(screen.queryByTestId("app-status-drawer-trigger")).toBeNull();
  });
});
