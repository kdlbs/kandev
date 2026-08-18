import { describe, it, expect, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import {
  ClarificationEscapeGuardProvider,
  useClarificationEscapeGuard,
} from "./use-clarification-escape-guard";

describe("useClarificationEscapeGuard", () => {
  it("does nothing outside a provider, so non-modal callers can invoke it unconditionally", () => {
    expect(() => renderHook(() => useClarificationEscapeGuard(true))).not.toThrow();
  });

  it("reports guarded state changes to the provider, and clears the guard on unmount", () => {
    const setGuarded = vi.fn();
    const { rerender, unmount } = renderHook(
      ({ guarded }) => useClarificationEscapeGuard(guarded),
      {
        initialProps: { guarded: false },
        wrapper: ({ children }) => (
          <ClarificationEscapeGuardProvider value={setGuarded}>
            {children}
          </ClarificationEscapeGuardProvider>
        ),
      },
    );

    expect(setGuarded).toHaveBeenLastCalledWith(false);

    rerender({ guarded: true });
    expect(setGuarded).toHaveBeenLastCalledWith(true);

    unmount();
    expect(setGuarded).toHaveBeenLastCalledWith(false);
  });
});
