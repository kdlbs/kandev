import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MobileUtilityActions } from "./mobile-menu-utility-actions";

const mocks = vi.hoisted(() => ({
  setTheme: vi.fn(),
  theme: "light" as "light" | "dark",
}));

vi.mock("@/components/theme/app-theme", () => ({
  useTheme: () => ({ theme: mocks.theme, setTheme: mocks.setTheme }),
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
    expect(screen.getByTestId("mobile-theme-toggle-button")).toBeTruthy();
  });

  it("switches from light to dark on click", () => {
    renderComponent();
    fireEvent.click(screen.getByTestId("mobile-theme-toggle-button"));
    expect(mocks.setTheme).toHaveBeenCalledWith("dark");
  });

  it("switches from dark to light on click", () => {
    mocks.theme = "dark";
    renderComponent();
    fireEvent.click(screen.getByTestId("mobile-theme-toggle-button"));
    expect(mocks.setTheme).toHaveBeenCalledWith("light");
  });
});
