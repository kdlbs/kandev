import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useComposerDisclosure } from "./use-composer-disclosure";

beforeEach(() => vi.useFakeTimers());
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

const advance = (ms: number) => act(() => vi.advanceTimersByTime(ms));

function setup() {
  return renderHook((props) => useComposerDisclosure(props), {
    initialProps: { enabled: true, sessionId: "a" as string | null },
  });
}

describe("Threads composer disclosure entry and focus", () => {
  // @covers AC-UI-THREADS-DECK-005.1
  it("starts collapsed only when auto-hide is effective", () => {
    const { result } = renderHook(() => useComposerDisclosure({ enabled: true, sessionId: "a" }));
    expect(result.current.expanded).toBe(false);
  });

  // @covers AC-UI-THREADS-DECK-005.2
  it("reveals after 150ms dwell and hides 300ms after leaving", () => {
    const { result } = setup();
    act(() => result.current.pointerEnter("mouse"));
    advance(149);
    expect(result.current.expanded).toBe(false);
    advance(1);
    expect(result.current.expanded).toBe(true);
    act(() => result.current.pointerLeave("mouse"));
    advance(299);
    expect(result.current.expanded).toBe(true);
    advance(1);
    expect(result.current.expanded).toBe(false);
  });

  it("cancels short entries and does not reveal on touch compatibility events", () => {
    const { result } = setup();
    act(() => result.current.pointerEnter("mouse"));
    advance(100);
    act(() => result.current.pointerLeave("mouse"));
    advance(400);
    expect(result.current.expanded).toBe(false);
    act(() => result.current.pointerEnter("touch"));
    advance(1000);
    expect(result.current.expanded).toBe(false);
  });

  // @covers AC-UI-THREADS-DECK-005.3/.4
  it("reveals on keyboard focus and waits until focus and all owned overlays leave", () => {
    const { result } = setup();
    act(() => result.current.focus());
    expect(result.current.expanded).toBe(true);
    act(() => {
      result.current.reportActivity("model", { overlay: true });
      result.current.reportActivity("context", { overlay: true });
      result.current.blur();
    });
    advance(1000);
    act(() => result.current.reportActivity("model", null));
    advance(1000);
    expect(result.current.expanded).toBe(true);
    act(() => result.current.reportActivity("context", null));
    advance(299);
    expect(result.current.expanded).toBe(true);
    advance(1);
    expect(result.current.expanded).toBe(false);
  });
});

describe("Threads composer disclosure holds and lifecycle", () => {
  // @covers AC-UI-THREADS-DECK-005.4/.7
  it("preserves a draft hold but permits explicit collapse and Enter-style reveal", () => {
    const { result } = setup();
    act(() => result.current.reportActivity("editor", { draft: true }));
    expect(result.current.expanded).toBe(true);
    act(() => {
      result.current.collapse();
      result.current.focus(true);
    });
    advance(1000);
    expect(result.current.expanded).toBe(false);
    act(() => result.current.reveal());
    expect(result.current.expanded).toBe(true);
  });

  it.each(["busy", "required"] as const)("cannot collapse during %s activity", (kind) => {
    const { result } = setup();
    act(() => result.current.reportActivity("editor", { [kind]: true }));
    expect(result.current.expanded).toBe(true);
    expect(result.current.canCollapse).toBe(false);
    act(() => result.current.collapse());
    expect(result.current.expanded).toBe(true);
  });

  it("explicit collapse cancels an entry dwell until the pointer enters again", () => {
    const { result } = setup();
    act(() => result.current.focus());
    act(() => result.current.pointerEnter("mouse"));
    advance(50);
    act(() => {
      result.current.collapse();
      result.current.focus(true);
    });
    advance(1000);
    expect(result.current.expanded).toBe(false);
    act(() => result.current.pointerLeave("mouse"));
    act(() => result.current.pointerEnter("mouse"));
    advance(149);
    expect(result.current.expanded).toBe(false);
    advance(1);
    expect(result.current.expanded).toBe(true);
  });

  it("a required action overrides manual collapse and remains open until it settles", () => {
    const { result } = setup();
    act(() => result.current.reveal());
    act(() => result.current.collapse());
    act(() => result.current.reportActivity("recovery", { required: true }));
    expect(result.current.expanded).toBe(true);
    act(() => result.current.reportActivity("recovery", null));
    advance(300);
    expect(result.current.expanded).toBe(false);
  });

  // @covers AC-UI-THREADS-DECK-005.7/.9/.10
  it("keeps the composer visible in effective fallback and resets released session state", () => {
    const { result, rerender } = setup();
    act(() => result.current.reportActivity("editor", { draft: true }));
    const stale = result.current;
    rerender({ enabled: true, sessionId: "b" });
    act(() => stale.reportActivity("editor", { busy: true }));
    advance(1000);
    expect(result.current.expanded).toBe(false);
    rerender({ enabled: false, sessionId: "b" });
    expect(result.current.expanded).toBe(true);
    expect(result.current.canCollapse).toBe(false);
    rerender({ enabled: true, sessionId: "b" });
    expect(result.current.expanded).toBe(false);
  });

  it("cancels pending timers on identity change and unmount", () => {
    const { result, rerender, unmount } = setup();
    act(() => result.current.pointerEnter("pen"));
    advance(100);
    rerender({ enabled: true, sessionId: null });
    advance(1000);
    expect(result.current.expanded).toBe(false);
    act(() => result.current.pointerEnter("mouse"));
    unmount();
    expect(vi.getTimerCount()).toBe(0);
  });
});
