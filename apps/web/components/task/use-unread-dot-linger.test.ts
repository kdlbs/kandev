import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useUnreadDotLinger } from "./use-unread-dot-linger";

type Props = { selected: boolean; showing: boolean; delayMs: number };

function render(initial: Props) {
  return renderHook(
    ({ selected, showing, delayMs }: Props) => useUnreadDotLinger(selected, showing, delayMs),
    { initialProps: initial },
  );
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("useUnreadDotLinger", () => {
  it("keeps the dot for the delay after an unread task is opened, then clears it", () => {
    const hook = render({ selected: false, showing: true, delayMs: 5000 });
    expect(hook.result.current).toBe(false);

    hook.rerender({ selected: true, showing: true, delayMs: 5000 });
    expect(hook.result.current).toBe(true);
    // The backend flag clears once the cursor advances; the linger outlives it.
    hook.rerender({ selected: true, showing: false, delayMs: 5000 });
    act(() => vi.advanceTimersByTime(4999));
    expect(hook.result.current).toBe(true);
    act(() => vi.advanceTimersByTime(1));
    expect(hook.result.current).toBe(false);
  });

  it("does not linger when the opened task had no dot or the delay is zero", () => {
    const read = render({ selected: false, showing: false, delayMs: 5000 });
    read.rerender({ selected: true, showing: false, delayMs: 5000 });
    expect(read.result.current).toBe(false);

    const immediate = render({ selected: false, showing: true, delayMs: 0 });
    immediate.rerender({ selected: true, showing: true, delayMs: 0 });
    expect(immediate.result.current).toBe(false);
  });

  it("does not start for a message that arrives while the task is already open", () => {
    const hook = render({ selected: true, showing: false, delayMs: 5000 });
    hook.rerender({ selected: true, showing: true, delayMs: 5000 });
    expect(hook.result.current).toBe(false);
  });

  it("stops lingering as soon as another task is opened", () => {
    const hook = render({ selected: false, showing: true, delayMs: 5000 });
    hook.rerender({ selected: true, showing: true, delayMs: 5000 });
    hook.rerender({ selected: false, showing: true, delayMs: 5000 });
    expect(hook.result.current).toBe(false);
  });
});
