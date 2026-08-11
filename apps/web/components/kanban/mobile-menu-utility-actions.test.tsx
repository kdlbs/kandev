import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MobileUtilityActions } from "./mobile-menu-utility-actions";

const THEME_TOGGLE_TEST_ID = "mobile-theme-toggle-button";

const mocks = vi.hoisted(() => ({
  setTheme: vi.fn(),
  theme: "light" as "light" | "dark" | "system",
  resolvedTheme: "light" as "light" | "dark",
}));

vi.mock("@/components/theme/app-theme", () => ({
  useTheme: () => ({
    theme: mocks.theme,
    resolvedTheme: mocks.resolvedTheme,
    setTheme: mocks.setTheme,
  }),
}));
vi.mock("@/hooks/use-app-destinations", () => ({
  useStaticDestinations: () => [],
}));
vi.mock("@/components/app-status-bar/app-status-surface-provider", () => ({
  useAppStatusDrawer: () => ({
    enabled: false,
    issueSeverity: "none",
    openStatusDrawer: vi.fn(),
  }),
}));
vi.mock("@/components/app-status-bar/connection-status-item", () => ({
  useConnectionIssueCopy: () => null,
}));

afterEach(cleanup);

describe("MobileUtilityActions", () => {
  beforeEach(() => {
    mocks.setTheme.mockClear();
    mocks.theme = "light";
    mocks.resolvedTheme = "light";
  });

  function renderComponent() {
    return render(
      <MobileUtilityActions
        showHealthIndicator={false}
        onOpenHealthDialog={vi.fn()}
        onOpenImproveKandev={vi.fn()}
        onOpenChange={vi.fn()}
      />,
    );
  }

  it("renders a theme toggle button", () => {
    renderComponent();
    expect(screen.getByTestId(THEME_TOGGLE_TEST_ID)).toBeTruthy();
  });

  it("switches from light to dark on click", () => {
    renderComponent();
    fireEvent.click(screen.getByTestId(THEME_TOGGLE_TEST_ID));
    expect(mocks.setTheme).toHaveBeenCalledWith("dark");
  });

  it("switches from dark to light on click", () => {
    mocks.theme = "dark";
    mocks.resolvedTheme = "dark";
    renderComponent();
    fireEvent.click(screen.getByTestId(THEME_TOGGLE_TEST_ID));
    expect(mocks.setTheme).toHaveBeenCalledWith("light");
  });

  it("uses resolvedTheme when theme is 'system' and the OS prefers dark", () => {
    mocks.theme = "system";
    mocks.resolvedTheme = "dark";
    renderComponent();
    fireEvent.click(screen.getByTestId(THEME_TOGGLE_TEST_ID));
    expect(mocks.setTheme).toHaveBeenCalledWith("light");
  });

  it("uses resolvedTheme when theme is 'system' and the OS prefers light", () => {
    mocks.theme = "system";
    mocks.resolvedTheme = "light";
    renderComponent();
    fireEvent.click(screen.getByTestId(THEME_TOGGLE_TEST_ID));
    expect(mocks.setTheme).toHaveBeenCalledWith("dark");
  });

  it("exposes the target theme and current state to assistive technology", () => {
    renderComponent();

    const toggle = screen.getByTestId(THEME_TOGGLE_TEST_ID);
    expect(toggle.getAttribute("aria-label")).toBe("Switch to Dark Mode");
    expect(toggle.getAttribute("aria-pressed")).toBe("false");
  });

  it("updates the accessible target and pressed state for a resolved dark theme", () => {
    mocks.theme = "system";
    mocks.resolvedTheme = "dark";
    renderComponent();

    const toggle = screen.getByTestId(THEME_TOGGLE_TEST_ID);
    expect(toggle.getAttribute("aria-label")).toBe("Switch to Light Mode");
    expect(toggle.getAttribute("aria-pressed")).toBe("true");
  });
});
