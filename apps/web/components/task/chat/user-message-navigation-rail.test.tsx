import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  USER_MESSAGE_NAVIGATION_MOBILE_CLEARANCE_CLASS,
  UserMessageNavigationRail,
} from "./user-message-navigation-rail";

let isFinePointer = true;
let isMobile = false;
const PREVIOUS_LABEL = "Previous user message";
const NEXT_LABEL = "Next user message";
const RAIL_TEST_ID = "user-message-navigation-rail";

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer, isMobile }),
}));

afterEach(cleanup);

beforeEach(() => {
  isFinePointer = true;
  isMobile = false;
});

function renderRail(
  overrides: Partial<React.ComponentProps<typeof UserMessageNavigationRail>> = {},
) {
  const onPrevious = vi.fn();
  const onNext = vi.fn();
  render(
    <UserMessageNavigationRail
      canNavigatePrevious={true}
      canNavigateNext={true}
      isBusy={false}
      onPrevious={onPrevious}
      onNext={onNext}
      {...overrides}
    />,
  );
  return { onPrevious, onNext };
}

describe("UserMessageNavigationRail", () => {
  it("exposes directional controls and activates them", () => {
    const { onPrevious, onNext } = renderRail();

    fireEvent.click(screen.getByRole("button", { name: PREVIOUS_LABEL }));
    fireEvent.click(screen.getByRole("button", { name: NEXT_LABEL }));

    expect(onPrevious).toHaveBeenCalledOnce();
    expect(onNext).toHaveBeenCalledOnce();
    expect(screen.getByTestId(RAIL_TEST_ID)).not.toBeNull();
    expect(screen.getByTestId("previous-user-message")).not.toBeNull();
    expect(screen.getByTestId("next-user-message")).not.toBeNull();
  });

  it("reports busy state and blocks duplicate activation", () => {
    const { onPrevious, onNext } = renderRail({ isBusy: true });
    const rail = screen.getByRole("navigation", { name: "User message navigation" });
    const previous = screen.getByRole("button", { name: PREVIOUS_LABEL });
    const next = screen.getByRole("button", { name: NEXT_LABEL });

    fireEvent.click(previous);
    fireEvent.click(next);

    expect(rail.getAttribute("aria-busy")).toBe("true");
    expect((previous as HTMLButtonElement).disabled).toBe(true);
    expect((next as HTMLButtonElement).disabled).toBe(true);
    expect(onPrevious).not.toHaveBeenCalled();
    expect(onNext).not.toHaveBeenCalled();
  });

  it("reflects known navigation boundaries", () => {
    renderRail({ canNavigatePrevious: false, canNavigateNext: false });

    expect(
      (screen.getByRole("button", { name: PREVIOUS_LABEL }) as HTMLButtonElement).disabled,
    ).toBe(true);
    expect((screen.getByRole("button", { name: NEXT_LABEL }) as HTMLButtonElement).disabled).toBe(
      true,
    );
  });

  it("discloses on chat hover or focus for a fine pointer", () => {
    renderRail();

    const rail = screen.getByTestId(RAIL_TEST_ID);
    expect(rail.className).toContain("opacity-0");
    expect(rail.className).toContain("group-hover/chat:opacity-100");
    expect(rail.className).toContain("group-focus-within/chat:opacity-100");
  });

  it("stays visible with safe-area-aware 44px controls for a coarse pointer", () => {
    isFinePointer = false;
    renderRail();

    const rail = screen.getByTestId(RAIL_TEST_ID);
    const previous = screen.getByRole("button", { name: PREVIOUS_LABEL });
    const next = screen.getByRole("button", { name: NEXT_LABEL });

    expect(rail.className).toContain("opacity-100");
    expect(rail.className).toContain("env(safe-area-inset-right)");
    expect(USER_MESSAGE_NAVIGATION_MOBILE_CLEARANCE_CLASS).toContain("env(safe-area-inset-right)");
    expect(previous.className).toContain("h-11");
    expect(previous.className).toContain("w-11");
    expect(next.className).toContain("h-11");
    expect(next.className).toContain("w-11");
  });

  it("stays visible with touch-sized controls on a mobile viewport with a fine pointer", () => {
    isMobile = true;
    renderRail();

    const rail = screen.getByTestId(RAIL_TEST_ID);
    const previous = screen.getByRole("button", { name: PREVIOUS_LABEL });

    expect(rail.className).toContain("opacity-100");
    expect(previous.className).toContain("h-11");
    expect(previous.className).toContain("w-11");
  });
});
