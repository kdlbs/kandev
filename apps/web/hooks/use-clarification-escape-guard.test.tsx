import { describe, it, expect, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import {
  ClarificationEscapeGuardProvider,
  useClarificationEscapeGuard,
  type ClarificationEscapeGuardEntry,
} from "./use-clarification-escape-guard";

function fakeEscape(target: EventTarget): KeyboardEvent {
  return {
    key: "Escape",
    target,
    metaKey: false,
    ctrlKey: false,
    altKey: false,
    shiftKey: false,
  } as unknown as KeyboardEvent;
}

describe("useClarificationEscapeGuard", () => {
  it("does nothing outside a provider, so non-modal callers can invoke it unconditionally", () => {
    expect(() => renderHook(() => useClarificationEscapeGuard(() => true))).not.toThrow();
  });

  it("wraps a registered predicate in an entry object, so useState never mistakes the function for an updater", () => {
    const setEntry = vi.fn();
    const predicateA = () => true;
    const predicateB = () => false;
    const { rerender, unmount } = renderHook(
      ({ predicate }) => useClarificationEscapeGuard(predicate),
      {
        initialProps: { predicate: predicateA as (() => boolean) | null },
        wrapper: ({ children }) => (
          <ClarificationEscapeGuardProvider value={setEntry}>
            {children}
          </ClarificationEscapeGuardProvider>
        ),
      },
    );

    expect(setEntry).toHaveBeenLastCalledWith({ test: predicateA });

    rerender({ predicate: predicateB });
    expect(setEntry).toHaveBeenLastCalledWith({ test: predicateB });

    rerender({ predicate: null });
    expect(setEntry).toHaveBeenLastCalledWith(null);

    unmount();
    expect(setEntry).toHaveBeenLastCalledWith(null);
  });

  it("the registered predicate is called with the real Escape event, not re-derived", () => {
    // A holder object, not a reassigned `let`, so TS doesn't narrow the
    // read below to the initializer's type across the closure boundary.
    const holder: { entry: ClarificationEscapeGuardEntry } = { entry: null };
    const setEntry = vi.fn((entry: ClarificationEscapeGuardEntry) => {
      holder.entry = entry;
    });
    const scope = document.createElement("div");
    const inside = document.createElement("button");
    scope.appendChild(inside);
    const predicate = vi.fn((event: KeyboardEvent) => scope.contains(event.target as Node));

    renderHook(() => useClarificationEscapeGuard(predicate), {
      wrapper: ({ children }) => (
        <ClarificationEscapeGuardProvider value={setEntry}>
          {children}
        </ClarificationEscapeGuardProvider>
      ),
    });

    const insideEvent = fakeEscape(inside);
    expect(holder.entry?.test(insideEvent)).toBe(true);
    expect(predicate).toHaveBeenCalledWith(insideEvent);

    const outside = document.createElement("button");
    expect(holder.entry?.test(fakeEscape(outside))).toBe(false);
  });
});
