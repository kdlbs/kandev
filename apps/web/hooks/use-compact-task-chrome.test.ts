import { describe, expect, it } from "vitest";

import type { ResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { shouldUseCompactTaskChrome, shouldUseTouchDrawer } from "@/hooks/use-compact-task-chrome";

function breakpoint(overrides: Partial<ResponsiveBreakpoint>): ResponsiveBreakpoint {
  return {
    breakpoint: "desktop",
    isMobile: false,
    isTablet: false,
    isDesktop: true,
    isCompactDesktop: false,
    isFullDesktop: true,
    isFinePointer: true,
    usesDesktopWorkbench: true,
    ...overrides,
  };
}

describe("compact task chrome breakpoint", () => {
  it("uses compact task chrome on mobile and tablet layouts", () => {
    expect(shouldUseCompactTaskChrome(breakpoint({ breakpoint: "mobile", isMobile: true }))).toBe(
      true,
    );
    expect(shouldUseCompactTaskChrome(breakpoint({ breakpoint: "tablet", isTablet: true }))).toBe(
      true,
    );
  });

  it("keeps full task chrome on compact and full desktop layouts", () => {
    expect(
      shouldUseCompactTaskChrome(
        breakpoint({
          breakpoint: "compactDesktop",
          isDesktop: true,
          isCompactDesktop: true,
          isFullDesktop: false,
        }),
      ),
    ).toBe(false);
    expect(shouldUseCompactTaskChrome(breakpoint({ breakpoint: "desktop" }))).toBe(false);
  });
});

describe("touch drawer breakpoint", () => {
  it("uses drawers on coarse-pointer layouts", () => {
    expect(shouldUseTouchDrawer(breakpoint({ isFinePointer: false }))).toBe(true);
  });

  it("keeps hover surfaces on fine-pointer layouts", () => {
    expect(shouldUseTouchDrawer(breakpoint({ breakpoint: "tablet", isTablet: true }))).toBe(false);
  });
});
