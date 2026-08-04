import { renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, expectTypeOf, it, vi } from "vitest";

const availabilitySpy = vi.hoisted(() => vi.fn(() => ({ github: true })));

vi.mock("@/hooks/use-nav-availability", () => ({
  useNavAvailability: availabilitySpy,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (
    selector: (state: {
      workspaces: { activeId: string | null };
      features: Record<string, boolean>;
    }) => unknown,
  ) => selector({ workspaces: { activeId: "ws-1" }, features: { office: false } }),
}));

vi.mock("@/lib/routing/client-router", () => ({
  usePathname: () => "/",
}));

vi.mock("@/lib/plugins/registry", () => ({
  usePluginRegistry: () => ({ getNavRegistrations: () => [] }),
}));

import { useAppDestinations, useStaticDestinations } from "./use-app-destinations";

describe("useStaticDestinations", () => {
  beforeEach(() => availabilitySpy.mockClear());
  afterEach(() => vi.clearAllMocks());

  it("rejects availability-gated sections at compile time", () => {
    type StaticSection = NonNullable<Parameters<typeof useStaticDestinations>[1]>;

    expectTypeOf<"insights">().toExtend<StaticSection>();
    expectTypeOf<"integrations">().not.toExtend<StaticSection>();
  });

  it("requires an explicit section on every surface but the palette", () => {
    // Omitting the section resolves gated sections too, against an empty
    // availability map — harmless for the palette (its gated entry opts out),
    // but on any other surface it would silently drop unconfigured integrations.
    expectTypeOf(useStaticDestinations).toBeCallableWith("palette");
    const offPalette = () => {
      // @ts-expect-error - a non-palette surface must name its sections
      useStaticDestinations("sidebar");
    };

    expect(offPalette).toBeTypeOf("function");
  });

  /**
   * The regression this guards: every per-integration auth probe runs its own
   * 90s `setInterval` per consumer, so a surface that renders no gated
   * destination must not subscribe to availability just to draw a static link.
   */
  it("does not subscribe to integration availability", () => {
    renderHook(() => useStaticDestinations("sidebar", "insights"));

    expect(availabilitySpy).not.toHaveBeenCalled();
  });

  it("still resolves the ungated destinations it is asked for", () => {
    const { result } = renderHook(() => useStaticDestinations("sidebar", "insights"));

    expect(result.current.map((destination) => destination.id)).toEqual(["stats"]);
  });

  it("resolves the palette without availability, including the opted-out GitHub entry", () => {
    const { result } = renderHook(() => useStaticDestinations("palette"));

    expect(availabilitySpy).not.toHaveBeenCalled();
    expect(result.current.map((destination) => destination.palette?.id)).toContain("nav-github");
  });
});

describe("useAppDestinations", () => {
  beforeEach(() => availabilitySpy.mockClear());
  afterEach(() => vi.clearAllMocks());

  it("only accepts availability-gated sections at compile time", () => {
    type GatedSection = Parameters<typeof useAppDestinations>[1];

    expectTypeOf<"integrations">().toExtend<GatedSection>();
    expectTypeOf<"insights">().not.toExtend<GatedSection>();
  });

  it("subscribes to availability so gated sections can be filtered", () => {
    const { result } = renderHook(() => useAppDestinations("sidebar", "integrations"));

    expect(availabilitySpy).toHaveBeenCalled();
    // Only GitHub is configured in the mocked availability map.
    expect(result.current.map((destination) => destination.id)).toEqual(["github"]);
  });
});
