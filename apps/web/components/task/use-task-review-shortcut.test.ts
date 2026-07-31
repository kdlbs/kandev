import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { KEYS, type KeyboardShortcut } from "@/lib/keyboard/constants";
import type { TaskReviewTarget } from "./task-pr-open";
import { useTaskReviewShortcut } from "./use-task-review-shortcut";

const defaultShortcut: KeyboardShortcut = {
  key: KEYS.G,
  modifiers: { ctrlOrCmd: true, shift: true },
};

const controlShortcut: KeyboardShortcut = {
  key: KEYS.G,
  modifiers: { ctrl: true, shift: true },
};

const targets: TaskReviewTarget[] = [
  { type: "pr", key: "pr:1", url: "https://github.com/acme/kandev/pull/1", review: {} as never },
  {
    type: "mr",
    key: "mr:2",
    url: "https://gitlab.example/acme/kandev/-/merge_requests/2",
    review: {} as never,
  },
];

function keydown(key: string, init: KeyboardEventInit = {}) {
  return new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true, ...init });
}

function keyup(key: string) {
  act(() => window.dispatchEvent(new KeyboardEvent("keyup", { key, bubbles: true })));
}

function invokeShortcut(
  handleShortcut: (event: KeyboardEvent) => void,
  key = "g",
  init: KeyboardEventInit = { ctrlKey: true, shiftKey: true },
) {
  act(() => handleShortcut(keydown(key, init)));
}

function useShortcut(
  reviewTargets = targets,
  shortcut = defaultShortcut,
  onOpenTarget = vi.fn(),
  onNoTargets = vi.fn(),
) {
  return renderHook(() =>
    useTaskReviewShortcut({
      targets: reviewTargets,
      shortcut,
      onNoTargets,
      onOpenTarget,
    }),
  );
}

afterEach(() => cleanup());

describe("useTaskReviewShortcut held selection", () => {
  it("ignores OS key repeat for a direct-open review", () => {
    const onOpenTarget = vi.fn();
    const { result } = useShortcut([targets[0]], defaultShortcut, onOpenTarget);

    act(() => {
      result.current.handleShortcut(keydown("g", { ctrlKey: true, shiftKey: true }));
      result.current.handleShortcut(keydown("g", { ctrlKey: true, shiftKey: true, repeat: true }));
    });

    expect(onOpenTarget).toHaveBeenCalledTimes(1);
  });

  it("cycles in displayed provider order, wraps, and commits on primary release", () => {
    const onOpenTarget = vi.fn();
    const { result } = useShortcut(targets, controlShortcut, onOpenTarget);

    invokeShortcut(result.current.handleShortcut);
    expect(result.current).toMatchObject({ pickerOpen: true, selectedIndex: 0 });

    invokeShortcut(result.current.handleShortcut);
    expect(result.current.selectedIndex).toBe(1);

    invokeShortcut(result.current.handleShortcut);
    expect(result.current.selectedIndex).toBe(0);

    keyup("Control");
    expect(onOpenTarget).toHaveBeenCalledWith(targets[0]);
    expect(result.current.pickerOpen).toBe(false);
  });

  it("does not commit when Shift is released before the primary modifier", () => {
    const onOpenTarget = vi.fn();
    const { result } = useShortcut(targets, controlShortcut, onOpenTarget);

    invokeShortcut(result.current.handleShortcut);
    keyup("Shift");

    expect(result.current.pickerOpen).toBe(true);
    expect(onOpenTarget).not.toHaveBeenCalled();

    invokeShortcut(result.current.handleShortcut, "g", { ctrlKey: true });
    expect(result.current.selectedIndex).toBe(1);

    keyup("Control");
    expect(onOpenTarget).toHaveBeenCalledWith(targets[1]);
  });

  it("uses Control when it opened a ctrlOrCmd shortcut on macOS", () => {
    const platformDescriptor = Object.getOwnPropertyDescriptor(navigator, "platform");
    Object.defineProperty(navigator, "platform", { configurable: true, value: "MacIntel" });

    try {
      const onOpenTarget = vi.fn();
      const { result } = useShortcut(targets, defaultShortcut, onOpenTarget);

      invokeShortcut(result.current.handleShortcut, "g", { ctrlKey: true, shiftKey: true });
      invokeShortcut(result.current.handleShortcut, "g", { ctrlKey: true });
      expect(result.current.selectedIndex).toBe(1);

      keyup("Control");
      expect(onOpenTarget).toHaveBeenCalledWith(targets[1]);
    } finally {
      if (platformDescriptor) Object.defineProperty(navigator, "platform", platformDescriptor);
      else delete (navigator as { platform?: string }).platform;
    }
  });
});

describe("useTaskReviewShortcut cancellation", () => {
  it("cancels on Escape and does not commit after primary release", () => {
    const onOpenTarget = vi.fn();
    const { result } = useShortcut(targets, controlShortcut, onOpenTarget);

    invokeShortcut(result.current.handleShortcut);
    act(() => window.dispatchEvent(keydown("Escape")));
    keyup("Control");

    expect(result.current.pickerOpen).toBe(false);
    expect(onOpenTarget).not.toHaveBeenCalled();
  });
});

describe("useTaskReviewShortcut binding variants", () => {
  it("cancels on application blur and document hiding", () => {
    const onOpenTarget = vi.fn();
    const { result } = useShortcut(targets, controlShortcut, onOpenTarget);
    const visibilityDescriptor = Object.getOwnPropertyDescriptor(document, "visibilityState");

    invokeShortcut(result.current.handleShortcut);
    act(() => window.dispatchEvent(new Event("blur")));
    keyup("Control");

    expect(onOpenTarget).not.toHaveBeenCalled();

    invokeShortcut(result.current.handleShortcut);
    Object.defineProperty(document, "visibilityState", { configurable: true, value: "hidden" });
    act(() => document.dispatchEvent(new Event("visibilitychange")));
    keyup("Control");

    if (visibilityDescriptor)
      Object.defineProperty(document, "visibilityState", visibilityDescriptor);
    else delete (document as { visibilityState?: DocumentVisibilityState }).visibilityState;
    expect(onOpenTarget).not.toHaveBeenCalled();
  });

  it("commits custom modified shortcuts with their configured primary modifier", () => {
    const onOpenTarget = vi.fn();
    const shortcut: KeyboardShortcut = { key: KEYS.Y, modifiers: { alt: true, shift: true } };
    const { result } = useShortcut(targets, shortcut, onOpenTarget);

    invokeShortcut(result.current.handleShortcut, "y", { altKey: true, shiftKey: true });
    invokeShortcut(result.current.handleShortcut, "y", { altKey: true });
    keyup("Alt");

    expect(onOpenTarget).toHaveBeenCalledWith(targets[1]);
  });

  it("keeps a modifierless picker open until explicit activation", () => {
    const onOpenTarget = vi.fn();
    const shortcut: KeyboardShortcut = { key: KEYS.G };
    const { result } = useShortcut(targets, shortcut, onOpenTarget);

    invokeShortcut(result.current.handleShortcut, "g", {});
    invokeShortcut(result.current.handleShortcut, "g", {});
    keyup("Control");

    expect(result.current).toMatchObject({ pickerOpen: true, selectedIndex: 1 });
    expect(onOpenTarget).not.toHaveBeenCalled();

    act(() => result.current.openSelectedTarget());
    expect(onOpenTarget).toHaveBeenCalledWith(targets[1]);
    expect(result.current.pickerOpen).toBe(false);
  });

  it("uses the current target list if reviews update while the picker is open", () => {
    const onOpenTarget = vi.fn();
    const { result, rerender } = renderHook(
      ({ reviewTargets }) =>
        useTaskReviewShortcut({
          targets: reviewTargets,
          shortcut: controlShortcut,
          onNoTargets: vi.fn(),
          onOpenTarget,
        }),
      { initialProps: { reviewTargets: targets } },
    );

    invokeShortcut(result.current.handleShortcut);
    invokeShortcut(result.current.handleShortcut);
    rerender({ reviewTargets: [targets[0]] });

    expect(result.current.selectedIndex).toBe(0);
    keyup("Control");

    expect(onOpenTarget).toHaveBeenCalledWith(targets[0]);
  });

  it("does not open directly when targets shrink while the picker is held open", () => {
    const onOpenTarget = vi.fn();
    const { result, rerender } = renderHook(
      ({ reviewTargets }) =>
        useTaskReviewShortcut({
          targets: reviewTargets,
          shortcut: controlShortcut,
          onNoTargets: vi.fn(),
          onOpenTarget,
        }),
      { initialProps: { reviewTargets: targets } },
    );

    invokeShortcut(result.current.handleShortcut);
    rerender({ reviewTargets: [targets[0]] });

    invokeShortcut(result.current.handleShortcut, "g", { ctrlKey: true });
    expect(result.current.pickerOpen).toBe(true);
    expect(onOpenTarget).not.toHaveBeenCalled();

    keyup("Control");
    expect(onOpenTarget).toHaveBeenCalledOnce();
    expect(onOpenTarget).toHaveBeenCalledWith(targets[0]);
    expect(result.current.pickerOpen).toBe(false);
  });
});
