import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useResponsiveBreakpoint } from "./use-responsive-breakpoint";

type Listener = (event: MediaQueryListEvent) => void;

function matchesWidthQuery(query: string, width: number) {
  const minWidth = query.match(/min-width:\s*(\d+)px/);
  const maxWidth = query.match(/max-width:\s*(\d+)px/);
  if (!minWidth && !maxWidth) return false;

  const minMatches = !minWidth || width >= Number(minWidth[1]);
  const maxMatches = !maxWidth || width <= Number(maxWidth[1]);
  return minMatches && maxMatches;
}

function setViewport(
  width: number,
  pointer: "fine" | "coarse" = "fine",
  hover: "hover" | "none" = pointer === "fine" ? "hover" : "none",
) {
  Object.defineProperty(window, "innerWidth", {
    configurable: true,
    writable: true,
    value: width,
  });

  window.matchMedia = vi.fn((query: string) => {
    const mql = {
      media: query,
      matches:
        (query.includes("pointer: fine") && pointer === "fine") ||
        (query.includes("hover: hover") && hover === "hover") ||
        (query.includes("pointer: coarse") && pointer === "coarse") ||
        matchesWidthQuery(query, width),
      onchange: null,
      addEventListener: vi.fn((_event: string, listener: Listener) => {
        listeners.add(listener);
      }),
      removeEventListener: vi.fn((_event: string, listener: Listener) => {
        listeners.delete(listener);
      }),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    };
    return mql as unknown as MediaQueryList;
  });
}

const listeners = new Set<Listener>();

function notifyResize(width: number, pointer: "fine" | "coarse" = "fine") {
  setViewport(width, pointer);
  act(() => {
    for (const listener of listeners) {
      listener({ matches: true } as MediaQueryListEvent);
    }
  });
}

describe("useResponsiveBreakpoint", () => {
  beforeEach(() => {
    listeners.clear();
    setViewport(1024, "fine");
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("treats half-screen fine-pointer widths as compact desktop workbench", () => {
    setViewport(900, "fine");

    const { result } = renderHook(() => useResponsiveBreakpoint());

    expect(result.current.breakpoint).toBe("compactDesktop");
    expect(result.current.isDesktop).toBe(true);
    expect(result.current.isTablet).toBe(false);
    expect(result.current.usesDesktopWorkbench).toBe(true);
  });

  it("keeps coarse-pointer half-screen widths in tablet fallback", () => {
    setViewport(900, "coarse");

    const { result } = renderHook(() => useResponsiveBreakpoint());

    expect(result.current.breakpoint).toBe("tablet");
    expect(result.current.isTablet).toBe(true);
    expect(result.current.usesDesktopWorkbench).toBe(false);
  });

  it("keeps hover-only hybrid half-screen widths in tablet fallback", () => {
    setViewport(900, "coarse", "hover");

    const { result } = renderHook(() => useResponsiveBreakpoint());

    expect(result.current.breakpoint).toBe("tablet");
    expect(result.current.isFinePointer).toBe(false);
    expect(result.current.usesDesktopWorkbench).toBe(false);
  });

  it("updates workbench mode when crossing compact desktop boundary", () => {
    setViewport(640, "fine");
    const { result } = renderHook(() => useResponsiveBreakpoint());

    expect(result.current.usesDesktopWorkbench).toBe(false);

    notifyResize(768, "fine");

    expect(result.current.breakpoint).toBe("compactDesktop");
    expect(result.current.usesDesktopWorkbench).toBe(true);
  });
});
