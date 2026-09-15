import { act, cleanup, fireEvent, render, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createElement, type PointerEvent } from "react";
import { createPortal } from "react-dom";
import { useSidebarHoverReveal } from "./use-sidebar-hover-reveal";

const pointer = (pointerType = "mouse") => ({ pointerType }) as PointerEvent<HTMLElement>;

// @covers AC-UI-SIDEBAR-HOVER-001.1, AC-UI-SIDEBAR-HOVER-001.3,
// @covers AC-UI-SIDEBAR-HOVER-001.4, AC-UI-SIDEBAR-HOVER-001.5
beforeEach(() => {
  vi.useFakeTimers();
  vi.stubGlobal(
    "matchMedia",
    vi.fn(() => ({
      matches: true,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  );
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

function setup() {
  return renderHook(
    ({ collapsed, pathname }) =>
      useSidebarHoverReveal({ enabled: true, delayMs: 500, collapsed, pathname }),
    {
      initialProps: { collapsed: true, pathname: "/" },
    },
  );
}

it("requires a continuous full dwell and cancels early exits", () => {
  const { result } = setup();
  act(() => result.current.handlers.onPointerEnter(pointer()));
  act(() => vi.advanceTimersByTime(499));
  expect(result.current.revealed).toBe(false);
  act(() => result.current.handlers.onPointerLeave());
  act(() => vi.advanceTimersByTime(500));
  expect(result.current.revealed).toBe(false);
  act(() => result.current.handlers.onPointerEnter(pointer()));
  act(() => vi.advanceTimersByTime(500));
  expect(result.current.revealed).toBe(true);
  act(() => result.current.handlers.onPointerLeave());
  expect(result.current.revealed).toBe(false);
});

it("retains focused content until both focus and pointer leave", async () => {
  const { result } = setup();
  act(() => result.current.handlers.onPointerEnter(pointer()));
  act(() => vi.advanceTimersByTime(500));
  act(() => result.current.handlers.onFocusCapture());
  act(() => result.current.handlers.onPointerLeave());
  expect(result.current.revealed).toBe(true);
  await act(async () => result.current.handlers.onBlurCapture());
  expect(result.current.revealed).toBe(false);
});

it("ignores touch and already expanded sidebars", () => {
  const { result, rerender } = setup();
  act(() => result.current.handlers.onPointerEnter(pointer("touch")));
  act(() => vi.advanceTimersByTime(500));
  expect(result.current.revealed).toBe(false);
  rerender({ collapsed: false, pathname: "/" });
  act(() => result.current.handlers.onPointerEnter(pointer()));
  act(() => vi.advanceTimersByTime(500));
  expect(result.current.revealed).toBe(false);
});

it("cancels pending and visible reveals on route and saved-state changes", () => {
  const { result, rerender } = setup();
  act(() => result.current.handlers.onPointerEnter(pointer()));
  rerender({ collapsed: true, pathname: "/tasks" });
  act(() => vi.advanceTimersByTime(500));
  expect(result.current.revealed).toBe(false);
  act(() => result.current.handlers.onPointerLeave());
  act(() => result.current.handlers.onPointerEnter(pointer()));
  act(() => vi.advanceTimersByTime(500));
  expect(result.current.revealed).toBe(true);
  rerender({ collapsed: false, pathname: "/tasks" });
  expect(result.current.revealed).toBe(false);
});

it("dismisses until a fresh entry and cleans up timers on unmount", () => {
  const { result, unmount } = setup();
  act(() => result.current.handlers.onPointerEnter(pointer()));
  act(() => vi.advanceTimersByTime(500));
  act(() => result.current.dismiss());
  act(() => result.current.handlers.onPointerEnter(pointer()));
  act(() => vi.advanceTimersByTime(500));
  expect(result.current.revealed).toBe(false);
  act(() => result.current.handlers.onPointerLeave());
  act(() => result.current.handlers.onPointerEnter(pointer()));
  unmount();
  expect(vi.getTimerCount()).toBe(0);
});
it("dismisses on Escape even when keyboard focus is outside the sidebar", () => {
  const { result } = setup();
  act(() => result.current.handlers.onPointerEnter(pointer()));
  act(() => vi.advanceTimersByTime(500));
  fireEvent.keyDown(document.body, { key: "Escape" });
  expect(result.current.revealed).toBe(false);
});

it("does not dismiss for an Escape consumed by a child", () => {
  const { result } = setup();
  act(() => result.current.handlers.onPointerEnter(pointer()));
  act(() => vi.advanceTimersByTime(500));
  const event = new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true });
  event.preventDefault();
  act(() => document.body.dispatchEvent(event));
  expect(result.current.revealed).toBe(true);
});

it("cancels a pending dwell and a visible reveal when hover capability is lost", () => {
  let changed = () => {};
  let eligible = true;
  vi.stubGlobal(
    "matchMedia",
    vi.fn(() => ({
      get matches() {
        return eligible;
      },
      addEventListener: (_name: string, callback: () => void) => {
        changed = callback;
      },
      removeEventListener: vi.fn(),
    })),
  );
  const { result } = setup();
  act(() => result.current.handlers.onPointerEnter(pointer()));
  act(() => {
    eligible = false;
    changed();
    vi.advanceTimersByTime(500);
  });
  expect(result.current.revealed).toBe(false);
  act(() => {
    eligible = true;
    changed();
    result.current.handlers.onPointerLeave();
  });
  act(() => result.current.handlers.onPointerEnter(pointer()));
  act(() => vi.advanceTimersByTime(500));
  expect(result.current.revealed).toBe(true);
  act(() => {
    eligible = false;
    changed();
  });
  expect(result.current.revealed).toBe(false);
});

it("retains an owned portal across pointer gaps and closes when it dismisses", async () => {
  let current: ReturnType<typeof useSidebarHoverReveal>;
  function Harness() {
    current = useSidebarHoverReveal({
      enabled: true,
      delayMs: 500,
      collapsed: true,
      pathname: "/",
    });
    return createElement(
      "aside",
      { ref: current.ref, ...current.handlers },
      createElement("button", { "aria-expanded": true, "aria-controls": "owned-menu" }),
      createPortal(createElement("div", { id: "owned-menu", role: "menu" }), document.body),
    );
  }
  const { container } = render(createElement(Harness));
  act(() => current.handlers.onPointerEnter(pointer()));
  act(() => vi.advanceTimersByTime(500));
  act(() => current.handlers.onPointerLeave());
  expect(current!.revealed).toBe(true);
  await act(async () => {
    container.querySelector("button")!.setAttribute("aria-expanded", "false");
  });
  expect(current!.revealed).toBe(false);
});

it("does not retain the reveal for unrelated application menus", () => {
  const menu = document.createElement("div");
  menu.setAttribute("role", "menu");
  menu.setAttribute("data-state", "open");
  document.body.append(menu);
  const { result } = setup();
  act(() => result.current.handlers.onPointerEnter(pointer()));
  act(() => vi.advanceTimersByTime(500));
  act(() => result.current.handlers.onPointerLeave());
  expect(result.current.revealed).toBe(false);
  menu.remove();
});

it("drops stale focus when navigation replaces the revealed contents", () => {
  const { result, rerender } = setup();
  act(() => result.current.handlers.onPointerEnter(pointer()));
  act(() => vi.advanceTimersByTime(500));
  act(() => result.current.handlers.onFocusCapture());
  rerender({ collapsed: true, pathname: "/tasks" });
  act(() => result.current.handlers.onPointerLeave());
  act(() => result.current.handlers.onPointerEnter(pointer()));
  act(() => vi.advanceTimersByTime(500));
  act(() => result.current.handlers.onPointerLeave());
  expect(result.current.revealed).toBe(false);
});

// @covers AC-UI-SIDEBAR-HOVER-002.4
it("honors disabled hover, custom dwell and setting changes", () => {
  const { result, rerender } = renderHook(
    (settings) =>
      useSidebarHoverReveal({
        collapsed: true,
        pathname: "/",
        ...settings,
      }),
    { initialProps: { enabled: false, delayMs: 1200 } },
  );
  act(() => result.current.handlers.onPointerEnter(pointer()));
  act(() => vi.advanceTimersByTime(1500));
  expect(result.current.revealed).toBe(false);
  rerender({ enabled: true, delayMs: 1200 });
  act(() => result.current.handlers.onPointerLeave());
  act(() => result.current.handlers.onPointerEnter(pointer()));
  act(() => vi.advanceTimersByTime(1199));
  expect(result.current.revealed).toBe(false);
  act(() => vi.advanceTimersByTime(1));
  expect(result.current.revealed).toBe(true);
  rerender({ enabled: false, delayMs: 1200 });
  expect(result.current.revealed).toBe(false);
  rerender({ enabled: true, delayMs: 0 });
  act(() => result.current.handlers.onPointerLeave());
  act(() => result.current.handlers.onPointerEnter(pointer()));
  act(() => vi.advanceTimersByTime(0));
  expect(result.current.revealed).toBe(true);
});

it("cancels old timers when the delay changes and requires fresh entry", () => {
  const { result, rerender } = renderHook(
    (settings) =>
      useSidebarHoverReveal({
        collapsed: true,
        pathname: "/",
        ...settings,
      }),
    { initialProps: { enabled: true, delayMs: 500 } },
  );
  act(() => result.current.handlers.onPointerEnter(pointer()));
  act(() => vi.advanceTimersByTime(300));
  rerender({ enabled: true, delayMs: 1000 });
  act(() => vi.advanceTimersByTime(2000));
  expect(result.current.revealed).toBe(false);
});

it("returns focus to the visible toggle when a saved setting dismisses a reveal", () => {
  let current: ReturnType<typeof useSidebarHoverReveal>;
  function Harness({ enabled }: { enabled: boolean }) {
    current = useSidebarHoverReveal({ collapsed: true, pathname: "/", enabled, delayMs: 500 });
    return createElement(
      "aside",
      { ref: current.ref, ...current.handlers },
      createElement("button", { "data-sidebar-toggle": "" }, "Expand"),
      createElement("button", { "data-navigation": "" }, "Navigation"),
    );
  }
  const { container, rerender } = render(createElement(Harness, { enabled: true }));
  act(() => current.handlers.onPointerEnter(pointer()));
  act(() => vi.advanceTimersByTime(500));
  const navigation = container.querySelector<HTMLButtonElement>("[data-navigation]")!;
  act(() => navigation.focus());
  rerender(createElement(Harness, { enabled: false }));
  act(() => vi.advanceTimersByTime(20));
  expect(current!.revealed).toBe(false);
  expect(document.activeElement).toBe(container.querySelector("[data-sidebar-toggle]"));
});

it("does not restore stale focus after explicit collapse and retain a later hover", () => {
  let current: ReturnType<typeof useSidebarHoverReveal>;
  function Harness({ collapsed }: { collapsed: boolean }) {
    current = useSidebarHoverReveal({ collapsed, pathname: "/", enabled: true, delayMs: 500 });
    return createElement(
      "aside",
      { ref: current.ref, ...current.handlers },
      createElement("button", { "data-sidebar-toggle": "" }, "Toggle"),
      createElement("button", { "data-navigation": "" }, "Navigation"),
    );
  }
  const { container, rerender } = render(createElement(Harness, { collapsed: false }));
  act(() => container.querySelector<HTMLButtonElement>("[data-navigation]")!.focus());
  rerender(createElement(Harness, { collapsed: true }));
  act(() => vi.advanceTimersByTime(20));
  act(() => current.handlers.onPointerEnter(pointer()));
  act(() => vi.advanceTimersByTime(500));
  act(() => current.handlers.onPointerLeave());
  expect(current!.revealed).toBe(false);
});

it.each(["(hover: hover)", "(pointer: fine)"])(
  "restores focus when %s capability disappears while navigation is focused",
  (capabilityQuery) => {
    let hover = true;
    let changed = () => {};
    vi.stubGlobal(
      "matchMedia",
      vi.fn((query: string) => ({
        get matches() {
          return query === capabilityQuery ? hover : true;
        },
        addEventListener: (_name: string, callback: () => void) => {
          if (query === capabilityQuery) changed = callback;
        },
        removeEventListener: vi.fn(),
      })),
    );
    let current: ReturnType<typeof useSidebarHoverReveal>;
    function Harness() {
      current = useSidebarHoverReveal({
        collapsed: true,
        pathname: "/",
        enabled: true,
        delayMs: 500,
      });
      return createElement(
        "aside",
        { ref: current.ref, ...current.handlers },
        createElement("button", { "data-sidebar-toggle": "" }, "Expand"),
        current.revealed && createElement("button", { "data-navigation": "" }, "Navigation"),
      );
    }
    const { container } = render(createElement(Harness));
    act(() => current.handlers.onPointerEnter(pointer()));
    act(() => vi.advanceTimersByTime(500));
    act(() => container.querySelector<HTMLButtonElement>("[data-navigation]")!.focus());
    act(() => {
      hover = false;
      changed();
    });
    act(() => vi.advanceTimersByTime(20));
    expect(current!.revealed).toBe(false);
    expect(document.activeElement).toBe(container.querySelector("[data-sidebar-toggle]"));
  },
);
