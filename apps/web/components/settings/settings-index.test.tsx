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
  SettingsPageNav: ({ pathname }: { pathname: string }) => (
    <div data-testid="mock-page-nav" data-pathname={pathname} />
  ),
}));

import { SettingsIndex } from "./settings-index";

const RESTORE_TO = "/settings/preferences/terminal-editors";

describe("SettingsIndex", () => {
  beforeEach(() => {
    router.replace.mockClear();
    router.push.mockClear();
    breakpoint.isMobile = false;
    window.history.replaceState({}, "", "/settings");
  });
  afterEach(cleanup);

  it("is the settings index on a phone", () => {
    breakpoint.isMobile = true;

    render(<SettingsIndex restoreTo={RESTORE_TO} />);

    expect(screen.getByTestId("settings-index")).not.toBeNull();
    const nav = screen.getByTestId("mock-page-nav");
    expect(nav.getAttribute("data-pathname")).toBe("/settings");
    expect(router.replace).not.toHaveBeenCalled();
  });

  it("hands off on desktop, where the sidebar shows the tree", () => {
    render(<SettingsIndex restoreTo={RESTORE_TO} />);

    expect(router.replace).toHaveBeenCalledWith(RESTORE_TO);
    expect(screen.queryByTestId("settings-index")).toBeNull();
  });

  it("replaces rather than pushes, so Back does not land here and redirect again", () => {
    render(<SettingsIndex restoreTo={RESTORE_TO} />);

    expect(router.push).not.toHaveBeenCalled();
    expect(router.replace).toHaveBeenCalledTimes(1);
  });

  it("hands off when the viewport grows past the sidebar boundary", () => {
    breakpoint.isMobile = true;
    const { rerender } = render(<SettingsIndex restoreTo={RESTORE_TO} />);
    expect(router.replace).not.toHaveBeenCalled();

    // From md up the sidebar renders this same tree, so staying would show two
    // identical menus side by side.
    breakpoint.isMobile = false;
    rerender(<SettingsIndex restoreTo={RESTORE_TO} />);

    expect(router.replace).toHaveBeenCalledWith(RESTORE_TO);
    expect(screen.queryByTestId("settings-index")).toBeNull();
  });

  it("does not hand off once the user has already navigated away", () => {
    // The sidebar is clickable while this route's chunk loads, so the effect can
    // run after a tap has moved on. Dragging them back would be worse than not
    // restoring at all.
    window.history.replaceState({}, "", "/?home=overview");

    render(<SettingsIndex restoreTo={RESTORE_TO} />);

    expect(router.replace).not.toHaveBeenCalled();
  });

  it("keeps the target it mounted with", () => {
    const { rerender } = render(<SettingsIndex restoreTo={RESTORE_TO} />);

    rerender(<SettingsIndex restoreTo="/settings/prompts" />);

    expect(router.replace).toHaveBeenCalledTimes(1);
    expect(router.replace).toHaveBeenCalledWith(RESTORE_TO);
  });
});
