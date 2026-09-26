import { describe, it, expect, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import {
  ClarificationEscapeGuardProvider,
  useClarificationEscapeGuard,
  type ClarificationEscapeGuardRegistry,
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

function fakeRegistry() {
  const entries = new Map<string, (event: KeyboardEvent) => boolean>();
  const registry: ClarificationEscapeGuardRegistry = {
    register: vi.fn((id, predicate) => entries.set(id, predicate)),
    unregister: vi.fn((id) => entries.delete(id)),
  };
  return { registry, entries };
}

describe("useClarificationEscapeGuard", () => {
  it("does nothing outside a provider, so non-modal callers can invoke it unconditionally", () => {
    expect(() => renderHook(() => useClarificationEscapeGuard(() => true))).not.toThrow();
  });

  it("registers and unregisters the predicate as it changes and on unmount", () => {
    const { registry, entries } = fakeRegistry();
    const predicateA = () => true;
    const predicateB = () => false;
    const { rerender, unmount } = renderHook(
      ({ predicate }) => useClarificationEscapeGuard(predicate),
      {
        initialProps: { predicate: predicateA as (() => boolean) | null },
        wrapper: ({ children }) => (
          <ClarificationEscapeGuardProvider value={registry}>
            {children}
          </ClarificationEscapeGuardProvider>
        ),
      },
    );

    expect(registry.register).toHaveBeenCalledTimes(1);
    expect([...entries.values()]).toEqual([predicateA]);

    rerender({ predicate: predicateB });
    expect([...entries.values()]).toEqual([predicateB]);

    rerender({ predicate: null });
    expect(entries.size).toBe(0);

    rerender({ predicate: predicateA });
    expect([...entries.values()]).toEqual([predicateA]);

    unmount();
    expect(entries.size).toBe(0);
  });

  it("two simultaneous callers each keep their own registered predicate (no single-slot clobbering)", () => {
    const { registry, entries } = fakeRegistry();
    const predicateA = vi.fn(() => true);
    const predicateB = vi.fn(() => false);

    renderHook(() => useClarificationEscapeGuard(predicateA), {
      wrapper: ({ children }) => (
        <ClarificationEscapeGuardProvider value={registry}>
          {children}
        </ClarificationEscapeGuardProvider>
      ),
    });
    renderHook(() => useClarificationEscapeGuard(predicateB), {
      wrapper: ({ children }) => (
        <ClarificationEscapeGuardProvider value={registry}>
          {children}
        </ClarificationEscapeGuardProvider>
      ),
    });

    expect(entries.size).toBe(2);
    expect([...entries.values()]).toEqual(expect.arrayContaining([predicateA, predicateB]));
  });

  it("the registered predicate is called with the real Escape event, not re-derived", () => {
    const { registry, entries } = fakeRegistry();
    const scope = document.createElement("div");
    const inside = document.createElement("button");
    scope.appendChild(inside);
    const predicate = vi.fn((event: KeyboardEvent) => scope.contains(event.target as Node));

    renderHook(() => useClarificationEscapeGuard(predicate), {
      wrapper: ({ children }) => (
        <ClarificationEscapeGuardProvider value={registry}>
          {children}
        </ClarificationEscapeGuardProvider>
      ),
    });

    const [registered] = [...entries.values()];
    const insideEvent = fakeEscape(inside);
    expect(registered(insideEvent)).toBe(true);
    expect(predicate).toHaveBeenCalledWith(insideEvent);

    const outside = document.createElement("button");
    expect(registered(fakeEscape(outside))).toBe(false);
  });
});
