import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useTaskIconTooltipState } from "./use-task-icon-tooltip-state";

describe("useTaskIconTooltipState", () => {
  it("leaves Escape available to an editable target", () => {
    const { result } = renderHook(() => useTaskIconTooltipState());
    act(() => result.current.onPointerEnter({ pointerType: "mouse" } as never));
    expect(result.current.open).toBe(true);

    const input = document.createElement("input");
    act(() => result.current.onEscapeKeyDown({ target: input } as unknown as Event));

    expect(result.current.open).toBe(true);
  });

  it("dismisses the tooltip for Escape from a non-editable target", () => {
    const { result } = renderHook(() => useTaskIconTooltipState());
    act(() => result.current.onPointerEnter({ pointerType: "mouse" } as never));

    const span = document.createElement("span");
    act(() => result.current.onEscapeKeyDown({ target: span } as unknown as Event));

    expect(result.current.open).toBe(false);
  });
});
