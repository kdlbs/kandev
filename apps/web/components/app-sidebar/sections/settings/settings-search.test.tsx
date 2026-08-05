import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ResolvedSettingsDiscoveryItem } from "@/lib/settings-discovery/types";
import { SettingsSearch } from "./settings-search";

const EASE_OUT = "cubic-bezier(0.2, 0, 0, 1)";

function item(id: string, label: string, order: number): ResolvedSettingsDiscoveryItem {
  return {
    id,
    kind: "control",
    label,
    aliases: [],
    breadcrumb: ["General", "Appearance"],
    groupId: "general",
    groupLabel: "General",
    href: `/settings/general/appearance#setting-${id}`,
    targetId: `setting-${id}`,
    order,
  };
}

const ITEMS = [item("alpha-layout", "Alpha Layout", 0), item("beta-layout", "Beta Layout", 1)];

function installLayoutMocks(reducedMotion = false) {
  const originalAnimate = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "animate");
  const animate = vi.fn(
    () =>
      ({
        addEventListener: vi.fn(),
        cancel: vi.fn(),
      }) as unknown as Animation,
  );
  Object.defineProperty(HTMLElement.prototype, "animate", {
    configurable: true,
    value: animate,
  });
  vi.stubGlobal(
    "matchMedia",
    vi.fn((media: string) => ({
      matches: reducedMotion,
      media,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  );
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (
    this: HTMLElement,
  ) {
    const nodes = [...document.querySelectorAll<HTMLElement>("[data-settings-search-motion-key]")];
    const top = Math.max(0, nodes.indexOf(this)) * 44;
    return {
      x: 0,
      y: top,
      top,
      bottom: top + 44,
      left: 0,
      right: 240,
      width: 240,
      height: 44,
      toJSON: () => ({}),
    };
  });
  return {
    animate,
    restore: () => {
      if (originalAnimate) {
        Object.defineProperty(HTMLElement.prototype, "animate", originalAnimate);
      } else {
        Reflect.deleteProperty(HTMLElement.prototype, "animate");
      }
    },
  };
}

describe("SettingsSearch result motion", () => {
  let restoreAnimate: (() => void) | undefined;

  afterEach(() => {
    cleanup();
    restoreAnimate?.();
    restoreAnimate = undefined;
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("softly introduces results and moves a surviving row from its previous position", () => {
    const mocks = installLayoutMocks();
    restoreAnimate = mocks.restore;
    const props = { items: ITEMS, onQueryChange: vi.fn(), onSelect: vi.fn() };
    const { rerender } = render(<SettingsSearch {...props} query="layout" />);

    expect(mocks.animate).toHaveBeenCalledWith(
      [
        { opacity: 0, transform: "translateY(2px)" },
        { opacity: 1, transform: "translateY(0)" },
      ],
      { duration: 140, easing: EASE_OUT },
    );

    mocks.animate.mockClear();
    rerender(<SettingsSearch {...props} query="beta layout" />);

    expect(mocks.animate).toHaveBeenCalledWith(
      [{ transform: "translateY(44px)" }, { transform: "translateY(0)" }],
      { duration: 160, easing: EASE_OUT },
    );
  });

  it("updates immediately when reduced motion is requested", () => {
    const mocks = installLayoutMocks(true);
    restoreAnimate = mocks.restore;
    const props = { items: ITEMS, onQueryChange: vi.fn(), onSelect: vi.fn() };
    const { rerender } = render(<SettingsSearch {...props} query="layout" />);

    rerender(<SettingsSearch {...props} query="beta layout" />);

    expect(mocks.animate).not.toHaveBeenCalled();
  });
});
