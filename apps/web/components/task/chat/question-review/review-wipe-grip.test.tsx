import { renderHook, act } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useWipeGrip } from "./review-wipe-grip";

describe("useWipeGrip", () => {
  it("ignores secondary mouse buttons on pointer down", () => {
    const hold = vi.fn();
    const reveal = { shown: false, hold, pulse: vi.fn(), clear: vi.fn() };
    const { result } = renderHook(() => useWipeGrip({ a: "Before", b: "After" }, reveal));

    const event = {
      button: 2,
      stopPropagation: vi.fn(),
      currentTarget: { setPointerCapture: vi.fn() },
    } as unknown as React.PointerEvent<HTMLElement>;

    act(() => {
      result.current.props.onPointerDown(event);
    });

    expect(hold).not.toHaveBeenCalled();
    expect(event.stopPropagation).not.toHaveBeenCalled();
  });
});
