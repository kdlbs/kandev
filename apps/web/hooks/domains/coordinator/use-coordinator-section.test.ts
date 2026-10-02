import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const nav = vi.hoisted(() => ({
  search: "",
  replace: vi.fn(),
}));

vi.mock("@/lib/routing/client-router", () => ({
  usePathname: () => "/settings/workspaces/ws/coordinators/c1",
  useSearchParams: () => new URLSearchParams(nav.search),
  useRouter: () => ({ replace: nav.replace }),
}));

import { useCoordinatorSection } from "./use-coordinator-section";

const SLUGS = ["identity", "standing-orders", "goal"];

beforeEach(() => {
  nav.replace.mockReset();
  nav.search = "";
  window.history.replaceState(null, "", "/settings/workspaces/ws/coordinators/c1");
});

describe("useCoordinatorSection", () => {
  it("defaults to identity for a missing, empty, unknown or unavailable value", () => {
    for (const search of ["", "?section=", "?section=nope", "?section=watches"]) {
      nav.search = search;
      const { result } = renderHook(() => useCoordinatorSection(SLUGS));
      expect(result.current.value).toBe("identity");
    }
  });

  it("uses the first of a repeated parameter", () => {
    nav.search = "?section=goal&section=standing-orders";
    const { result } = renderHook(() => useCoordinatorSection(SLUGS));
    expect(result.current.value).toBe("goal");
  });

  it("replaces the section, keeps other parameters, and adds no history entry", () => {
    window.history.replaceState(
      null,
      "",
      "/settings/workspaces/ws/coordinators/c1?x=1&section=goal",
    );
    nav.search = "?x=1&section=goal";
    const { result } = renderHook(() => useCoordinatorSection(SLUGS));
    act(() => result.current.selectSection("standing-orders"));
    expect(nav.replace).toHaveBeenCalledTimes(1);
    expect(nav.replace).toHaveBeenCalledWith(
      "/settings/workspaces/ws/coordinators/c1?x=1&section=standing-orders",
      { scroll: false },
    );
  });

  it("ignores a selection outside the registered sections", () => {
    const { result } = renderHook(() => useCoordinatorSection(SLUGS));
    act(() => result.current.selectSection("may-do"));
    expect(nav.replace).not.toHaveBeenCalled();
  });
});
