import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const breakpoint = { isMobile: false };
const router = { replace: vi.fn(), push: vi.fn() };

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => router,
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => breakpoint,
}));

// The tree is store- and plugin-heavy; this test is about the routing decision.
vi.mock("@/components/settings/settings-page-nav", () => ({
  SettingsPageNav: ({
    pathname,
    defaultOpenGroup,
  }: {
    pathname: string;
    defaultOpenGroup?: string;
  }) => <div data-testid="mock-page-nav" data-pathname={pathname} data-group={defaultOpenGroup} />,
}));

import { SettingsIndex } from "./settings-index";
import { DEFAULT_SETTINGS_PATH, rememberSettingsPath } from "@/lib/settings/last-settings-page";

describe("SettingsIndex", () => {
  beforeEach(() => {
    window.localStorage.clear();
    router.replace.mockClear();
    router.push.mockClear();
    breakpoint.isMobile = false;
  });
  afterEach(cleanup);

  it("is the settings index on a phone, opened on the General group", () => {
    breakpoint.isMobile = true;

    render(<SettingsIndex />);

    expect(screen.getByTestId("settings-index")).not.toBeNull();
    const nav = screen.getByTestId("mock-page-nav");
    expect(nav.getAttribute("data-pathname")).toBe("/settings");
    // Without this the tree would open DEFAULT_OPEN_GROUP (Workspaces), because
    // "/settings" belongs to no group prefix.
    expect(nav.getAttribute("data-group")).toBe("general");
    expect(router.replace).not.toHaveBeenCalled();
  });

  it("hands off to the last visited page on desktop, where the sidebar shows the tree", () => {
    rememberSettingsPath("/settings/general/terminal");

    render(<SettingsIndex />);

    expect(router.replace).toHaveBeenCalledWith("/settings/general/terminal");
    expect(screen.queryByTestId("settings-index")).toBeNull();
  });

  it("falls back to the default page when this device has no history", () => {
    render(<SettingsIndex />);

    expect(router.replace).toHaveBeenCalledWith(DEFAULT_SETTINGS_PATH);
  });

  it("replaces rather than pushes, so Back does not land here and redirect again", () => {
    render(<SettingsIndex />);

    expect(router.push).not.toHaveBeenCalled();
    expect(router.replace).toHaveBeenCalledTimes(1);
  });

  it("decides once at mount: a later viewport change does not navigate", () => {
    breakpoint.isMobile = true;
    const { rerender } = render(<SettingsIndex />);

    breakpoint.isMobile = false;
    rerender(<SettingsIndex />);

    expect(router.replace).not.toHaveBeenCalled();
    expect(screen.getByTestId("settings-index")).not.toBeNull();
  });
});
