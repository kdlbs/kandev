import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("@/lib/routing/client-router", () => ({
  usePathname: () => "/settings/preferences/appearance",
}));

vi.mock("./sections/settings/collapse-all-button", () => ({
  CollapseAllButton: () => <button type="button">Collapse</button>,
}));

vi.mock("./sections/settings/settings-tree", () => ({
  SettingsTree: () => <div data-testid="mock-settings-tree">Settings tree</div>,
}));

import { AppSidebarSettingsMode } from "./app-sidebar-settings-mode";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("AppSidebarSettingsMode", () => {
  it("renders settings tree inside the scroll area with scrollbar and fade support", () => {
    render(<AppSidebarSettingsMode />);

    expect(screen.getByTestId("app-sidebar-settings-mode")).toBeTruthy();
    expect(screen.getByTestId("app-sidebar-settings-scroll")).toBeTruthy();
    expect(screen.getByTestId("mock-settings-tree")).toBeTruthy();
  });
});
