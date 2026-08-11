import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { MockInstance } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useTaskTitleSelectionRestore } from "./use-task-title-selection-restore";

const LONG = "T".repeat(60);

type TitleInput = HTMLInputElement | HTMLTextAreaElement;

function Harness({ initial }: { initial: string }) {
  const [value, setValue] = useState(initial);
  const { inputRef, clampChange } = useTaskTitleSelectionRestore(value);
  return (
    <input
      ref={inputRef}
      data-testid="title"
      value={value}
      onChange={(e) => setValue(clampChange(e))}
    />
  );
}

function HarnessTextarea({ initial }: { initial: string }) {
  const [value, setValue] = useState(initial);
  const { inputRef, clampChange } = useTaskTitleSelectionRestore<HTMLTextAreaElement>(value);
  return (
    <textarea
      ref={inputRef}
      data-testid="title"
      value={value}
      onChange={(e) => setValue(clampChange(e))}
    />
  );
}

function ControlledHarness({
  value,
  onChange,
}: {
  value: string;
  onChange: (value: string) => void;
}) {
  const { inputRef, clampChange } = useTaskTitleSelectionRestore(value);
  return (
    <input
      ref={inputRef}
      data-testid="title"
      value={value}
      onChange={(e) => onChange(clampChange(e))}
    />
  );
}

/**
 * Simulate typing: write the DOM value through the prototype setter (React's
 * instance value tracker would otherwise swallow the change), place the caret,
 * then dispatch the change event React maps to onChange.
 */
function simulateInsert(
  input: TitleInput,
  value: string,
  caret: number,
  setSelectionRange: MockInstance,
) {
  const setNativeValue = Object.getOwnPropertyDescriptor(
    input.constructor.prototype,
    "value",
  )!.set!;
  setNativeValue.call(input, value);
  input.setSelectionRange(caret, caret);
  setSelectionRange.mockClear();
  fireEvent.change(input);
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("useTaskTitleSelectionRestore - commit-path caret restore", () => {
  it("clamps the value to 60 code points on change", () => {
    render(<Harness initial={LONG.slice(0, 59)} />);
    const input = screen.getByTestId("title") as HTMLInputElement;
    input.focus();
    simulateInsert(input, LONG, 60, vi.spyOn(HTMLInputElement.prototype, "setSelectionRange"));
    expect(input.value).toHaveLength(60);
  });

  it.each([
    [0, 2],
    [6, 8],
    [58, 60],
  ])("pins the caret after inserting mid-title at the cap (insert at %i)", (caret, expected) => {
    const setSelectionRange = vi.spyOn(HTMLInputElement.prototype, "setSelectionRange");
    render(<Harness initial={LONG} />);
    const input = screen.getByTestId("title") as HTMLInputElement;
    input.focus();
    // Simulate the DOM right after typing "XY" at `caret`: 62 code points,
    // caret after the insert.
    simulateInsert(
      input,
      `${LONG.slice(0, caret)}XY${LONG.slice(caret)}`,
      caret + 2,
      setSelectionRange,
    );

    expect(input.value).toHaveLength(60);
    expect(input.value.slice(caret, caret + 2)).toBe("XY");
    // The caret must be re-pinned after React rewrites the truncated value.
    expect(setSelectionRange).toHaveBeenCalledWith(expected, expected);
  });

  it("leaves the caret alone when the clamp does not truncate", () => {
    const setSelectionRange = vi.spyOn(HTMLInputElement.prototype, "setSelectionRange");
    render(<Harness initial={LONG.slice(0, 30)} />);
    const input = screen.getByTestId("title") as HTMLInputElement;
    input.focus();
    simulateInsert(input, `${"T".repeat(30)}XY`, 32, setSelectionRange);
    expect(input.value).toHaveLength(32);
    expect(setSelectionRange).not.toHaveBeenCalled();
  });

  it("skips the restore when the input is not focused", () => {
    const setSelectionRange = vi.spyOn(HTMLInputElement.prototype, "setSelectionRange");
    render(<Harness initial={LONG} />);
    const input = screen.getByTestId("title") as HTMLInputElement;
    simulateInsert(input, `${LONG.slice(0, 6)}XY${LONG.slice(6)}`, 8, setSelectionRange);
    expect(setSelectionRange).not.toHaveBeenCalled();
  });

  it("does not replay a stale selection from an unfocused truncating change", () => {
    const setSelectionRange = vi.spyOn(HTMLInputElement.prototype, "setSelectionRange");
    render(<Harness initial={LONG} />);
    const input = screen.getByTestId("title") as HTMLInputElement;
    // Truncating change while the input is not focused: records then discards.
    simulateInsert(input, `${LONG.slice(0, 6)}XY${LONG.slice(6)}`, 8, setSelectionRange);
    // A later focused, non-truncating change must not restore the old caret.
    input.focus();
    simulateInsert(input, `${LONG.slice(0, 6)}XY${LONG.slice(6, 58)}`, 8, setSelectionRange);
    expect(setSelectionRange).not.toHaveBeenCalled();
  });
});

describe("useTaskTitleSelectionRestore - bail-out caret restore", () => {
  it("restores the caret when a truncating keystroke leaves the clamped value unchanged", async () => {
    const setSelectionRange = vi.spyOn(HTMLInputElement.prototype, "setSelectionRange");
    render(<Harness initial={LONG} />);
    const input = screen.getByTestId("title") as HTMLInputElement;
    input.focus();
    // Typing the same character into an all-same-char title at the cap: the
    // clamped value equals the committed value, so React bails out of the
    // render (no layout effect) but still restores the controlled DOM value
    // after the event, resetting the caret to the end. The hook must re-pin
    // it after that write.
    simulateInsert(input, `${LONG}T`, 6, setSelectionRange);
    await act(async () => {});
    expect(input.value).toHaveLength(60);
    expect(setSelectionRange).toHaveBeenCalledWith(6, 6);
  });

  it("does not replay the caret on a later commit after a same-result bail-out", async () => {
    const setSelectionRange = vi.spyOn(HTMLInputElement.prototype, "setSelectionRange");
    const onChange = vi.fn();
    const { rerender } = render(<ControlledHarness value={LONG} onChange={onChange} />);
    const input = screen.getByTestId("title") as HTMLInputElement;
    input.focus();
    // Typing the same character at the cap: the clamped value equals the
    // committed value, so the render bails out.
    simulateInsert(input, `${LONG}T`, 6, setSelectionRange);
    expect(onChange).toHaveBeenCalledWith(LONG);
    await act(async () => {});
    // The same-key restore happens exactly once, via the immediate path.
    expect(setSelectionRange).toHaveBeenCalledTimes(1);
    setSelectionRange.mockClear();
    // An unrelated value update must not replay the caret.
    rerender(<ControlledHarness value={"U".repeat(60)} onChange={onChange} />);
    expect(setSelectionRange).not.toHaveBeenCalled();
  });

  it("does not apply a pending commit-path caret to an unrelated value commit", async () => {
    const setSelectionRange = vi.spyOn(HTMLInputElement.prototype, "setSelectionRange");
    const onChange = vi.fn();
    const { rerender } = render(<ControlledHarness value={LONG} onChange={onChange} />);
    const input = screen.getByTestId("title") as HTMLInputElement;
    input.focus();
    // A truncating change records the caret for the commit path...
    const next = `${LONG.slice(0, 6)}XY${LONG.slice(6, 58)}`;
    simulateInsert(input, `${LONG.slice(0, 6)}XY${LONG.slice(6)}`, 8, setSelectionRange);
    expect(onChange).toHaveBeenCalledWith(next);
    // ...but the parent rejects it (no state update), and an unrelated value
    // commit arrives later: the stale caret must not be applied to it.
    rerender(<ControlledHarness value={"U".repeat(60)} onChange={onChange} />);
    await act(async () => {});
    expect(setSelectionRange).not.toHaveBeenCalled();
  });

  it("does not restore a stale caret when the value changes in the same turn as a bail-out", async () => {
    const setSelectionRange = vi.spyOn(HTMLInputElement.prototype, "setSelectionRange");
    const onChange = vi.fn();
    const { rerender } = render(<ControlledHarness value={LONG} onChange={onChange} />);
    const input = screen.getByTestId("title") as HTMLInputElement;
    input.focus();
    // Schedules the bail-out restore for the old value...
    simulateInsert(input, `${LONG}T`, 6, setSelectionRange);
    // ...but the controlled value changes synchronously before the microtask
    // runs, so the restore must be superseded.
    rerender(<ControlledHarness value={"U".repeat(60)} onChange={onChange} />);
    await act(async () => {});
    expect(setSelectionRange).not.toHaveBeenCalled();
  });
});

describe("useTaskTitleSelectionRestore - textarea path", () => {
  it("pins the caret after a truncating change in a textarea", () => {
    const setSelectionRange = vi.spyOn(HTMLTextAreaElement.prototype, "setSelectionRange");
    render(<HarnessTextarea initial={LONG} />);
    const textarea = screen.getByTestId("title") as HTMLTextAreaElement;
    textarea.focus();
    simulateInsert(textarea, `${LONG.slice(0, 6)}XY${LONG.slice(6)}`, 8, setSelectionRange);

    expect(textarea.value).toHaveLength(60);
    expect(textarea.value.slice(6, 8)).toBe("XY");
    expect(setSelectionRange).toHaveBeenCalledWith(8, 8);
  });

  it("restores the caret on a same-result bail-out in a textarea", async () => {
    const setSelectionRange = vi.spyOn(HTMLTextAreaElement.prototype, "setSelectionRange");
    render(<HarnessTextarea initial={LONG} />);
    const textarea = screen.getByTestId("title") as HTMLTextAreaElement;
    textarea.focus();
    simulateInsert(textarea, `${LONG}T`, 6, setSelectionRange);
    await act(async () => {});
    expect(textarea.value).toHaveLength(60);
    expect(setSelectionRange).toHaveBeenCalledWith(6, 6);
  });
});
