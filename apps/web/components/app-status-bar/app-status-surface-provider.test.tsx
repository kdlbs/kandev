import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import { AppStatusDrawerTrigger, AppStatusSurfaceProvider } from "./app-status-surface-provider";

const responsiveState = vi.hoisted(() => ({
  breakpoint: "desktop" as "mobile" | "tablet" | "desktop",
}));
const featureState = vi.hoisted(() => ({ appStatusBar: true }));
const STATUS_BAR_TEST_ID = "app-status-bar";
const STATUS_DRAWER_TEST_ID = "app-status-drawer";
const STATUS_DRAWER_TRIGGER_TEST_ID = "app-status-drawer-trigger";

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
  AppStatusDrawer: ({ open, connectionOnly }: { open: boolean; connectionOnly?: boolean }) => (
    <div data-testid={STATUS_DRAWER_TEST_ID} data-connection-only={connectionOnly}>
      {String(open)}
    </div>
  ),
}));

function renderSurface(initialState?: Partial<AppState>, children = <AppStatusDrawerTrigger />) {
  return render(
    <StateProvider initialState={initialState}>
      <AppStatusSurfaceProvider>{children}</AppStatusSurfaceProvider>
    </StateProvider>,
  );
}

function ConnectionIssueControls() {
  const store = useAppStoreApi();
  return (
    <>
      <button
        data-testid="recover-connection"
        onClick={() => {
          store.getState().setConnectionStatus("connected");
          store.getState().setConnectionIssueSeverity("none");
        }}
      />
      <button
        data-testid="restore-connection-issue"
        onClick={() => {
          store.getState().setConnectionStatus("reconnecting");
          store.getState().setConnectionIssueSeverity("unstable");
        }}
      />
    </>
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
    expect(screen.queryByTestId(STATUS_DRAWER_TRIGGER_TEST_ID)).toBeNull();
  });

  it("exposes a connection-only Status drawer for an active phone warning when disabled", () => {
    responsiveState.breakpoint = "mobile";
    featureState.appStatusBar = false;
    renderSurface({
      connection: { status: "reconnecting", error: null, issueSeverity: "unstable" },
    });

    expect(screen.getByTestId(STATUS_DRAWER_TEST_ID).getAttribute("data-connection-only")).toBe(
      "true",
    );
    expect(screen.getByTestId(STATUS_DRAWER_TRIGGER_TEST_ID).getAttribute("aria-label")).toBe(
      "Connection unstable. Reconnecting to Kandev.",
    );

    fireEvent.click(screen.getByTestId(STATUS_DRAWER_TRIGGER_TEST_ID));
    expect(screen.getByTestId(STATUS_DRAWER_TEST_ID).textContent).toBe("true");
  });

  it("does not expose a drawer trigger at the tablet breakpoint", () => {
    responsiveState.breakpoint = "tablet";
    renderSurface();

    expect(screen.getByTestId(STATUS_BAR_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId(STATUS_DRAWER_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(STATUS_DRAWER_TRIGGER_TEST_ID)).toBeNull();
  });

  it("keeps an active connection warning reachable at the tablet breakpoint", () => {
    responsiveState.breakpoint = "tablet";
    featureState.appStatusBar = false;
    renderSurface({
      connection: { status: "reconnecting", error: null, issueSeverity: "unstable" },
    });

    expect(screen.getByTestId(STATUS_DRAWER_TEST_ID).getAttribute("data-connection-only")).toBe(
      "true",
    );
    const trigger = screen.getByTestId(STATUS_DRAWER_TRIGGER_TEST_ID);
    expect(trigger.className).toContain("lg:hidden");
    expect(trigger.className).not.toContain("md:hidden");
  });

  it("closes a connection-only drawer when the warning recovers", () => {
    responsiveState.breakpoint = "mobile";
    featureState.appStatusBar = false;
    renderSurface(
      { connection: { status: "reconnecting", error: null, issueSeverity: "unstable" } },
      <>
        <AppStatusDrawerTrigger />
        <ConnectionIssueControls />
      </>,
    );

    fireEvent.click(screen.getByTestId(STATUS_DRAWER_TRIGGER_TEST_ID));
    expect(screen.getByTestId(STATUS_DRAWER_TEST_ID).textContent).toBe("true");

    fireEvent.click(screen.getByTestId("recover-connection"));

    expect(screen.queryByTestId(STATUS_DRAWER_TEST_ID)).toBeNull();

    fireEvent.click(screen.getByTestId("restore-connection-issue"));

    expect(screen.getByTestId(STATUS_DRAWER_TEST_ID).textContent).toBe("false");
  });
});
