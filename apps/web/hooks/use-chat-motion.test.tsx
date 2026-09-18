import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useChatMotion } from "./use-chat-motion";
const state = vi.hoisted(() => ({ chatMotion: { enabled: true } }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (value: typeof state) => unknown) => select(state),
}));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  state.chatMotion.enabled = true;
});

// @covers AC-UI-CHAT-MOTION-002.3
describe("effective chat motion", () => {
  it("reacts to OS changes without changing the saved preference and cleans up", () => {
    const listeners = new Set<() => void>();
    const media = {
      matches: false,
      addEventListener: (_: string, cb: () => void) => listeners.add(cb),
      removeEventListener: (_: string, cb: () => void) => listeners.delete(cb),
    };
    vi.stubGlobal("matchMedia", () => media);
    const hook = renderHook(useChatMotion);
    expect(hook.result.current).toBe(true);
    act(() => {
      media.matches = true;
      listeners.forEach((cb) => cb());
    });
    expect(hook.result.current).toBe(false);
    expect(state.chatMotion.enabled).toBe(true);
    act(() => {
      media.matches = false;
      listeners.forEach((cb) => cb());
    });
    expect(hook.result.current).toBe(true);
    state.chatMotion.enabled = false;
    hook.rerender();
    expect(hook.result.current).toBe(false);
    hook.unmount();
    expect(listeners.size).toBe(0);
  });
  it("renders statically when media preference cannot be read", () => {
    vi.stubGlobal("matchMedia", undefined);
    expect(renderHook(useChatMotion).result.current).toBe(false);
  });
});
