import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const storeState = vi.hoisted(() => ({ enabled: true }));

vi.mock("@/components/state-provider", () => ({
  useOptionalAppStore: (selector: (state: { richOutputMotion: { enabled: boolean } }) => unknown) =>
    selector({ richOutputMotion: storeState }),
}));

import { useRichOutputChartAnimations } from "./chart-motion";

type MediaListener = (event: MediaQueryListEvent) => void;

let reducedMotion = false;
let listeners: MediaListener[] = [];

function installMatchMedia() {
  window.matchMedia = vi.fn(
    () =>
      ({
        get matches() {
          return reducedMotion;
        },
        media: "(prefers-reduced-motion: reduce)",
        onchange: null,
        addEventListener: (_type: string, listener: MediaListener) => listeners.push(listener),
        removeEventListener: (_type: string, listener: MediaListener) => {
          listeners = listeners.filter((candidate) => candidate !== listener);
        },
        addListener: vi.fn(),
        removeListener: vi.fn(),
        dispatchEvent: vi.fn(),
      }) as MediaQueryList,
  );
}

function setReducedMotion(matches: boolean) {
  reducedMotion = matches;
  const event = { matches, media: "(prefers-reduced-motion: reduce)" } as MediaQueryListEvent;
  for (const listener of listeners) listener(event);
}

beforeEach(() => {
  reducedMotion = false;
  listeners = [];
  storeState.enabled = true;
  installMatchMedia();
});

afterEach(cleanup);

describe("useRichOutputChartAnimations", () => {
  it("uses the enabled device preference when OS motion is allowed", () => {
    const { result } = renderHook(() => useRichOutputChartAnimations());

    expect(result.current).toBe(true);
  });

  it("disables animations for the explicit device opt-out", () => {
    storeState.enabled = false;

    const { result } = renderHook(() => useRichOutputChartAnimations());

    expect(result.current).toBe(false);
  });

  it("honors live reduced-motion changes without reloading", () => {
    const { result } = renderHook(() => useRichOutputChartAnimations());

    act(() => setReducedMotion(true));
    expect(result.current).toBe(false);

    act(() => setReducedMotion(false));
    expect(result.current).toBe(true);
  });
});
