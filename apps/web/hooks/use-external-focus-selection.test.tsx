import { useRef } from "react";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useExternalFocusSelection } from "./use-external-focus-selection";

function Input() {
  const inputRef = useRef<HTMLInputElement>(null);
  useExternalFocusSelection(inputRef);
  return <input ref={inputRef} defaultValue="Release notes" />;
}

beforeEach(() => vi.useFakeTimers());

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

function leaveWindow() {
  render(<Input />);
  const input = screen.getByRole("textbox") as HTMLInputElement;
  input.focus();
  input.setSelectionRange(2, 7, "backward");
  fireEvent(window, new Event("blur"));
  return input;
}

async function restoreWithFocusTrap(input: HTMLInputElement) {
  await act(async () => {
    fireEvent.focus(input);
    input.select();
    await vi.runAllTimersAsync();
  });
}

// @covers AC-UI-EDIT-FOCUS-001.2
it("restores selection after a focus trap selects the returning input", async () => {
  const input = leaveWindow();
  await restoreWithFocusTrap(input);
  expect([input.selectionStart, input.selectionEnd, input.selectionDirection]).toEqual([
    2,
    7,
    "backward",
  ]);
});

it.each(["keydown", "beforeinput"])(
  "restores the caret before %s when focus restoration is still pending",
  async (type) => {
    const input = leaveWindow();
    fireEvent.focus(input);
    input.select();

    fireEvent(input, new Event(type, { bubbles: true }));

    expect([input.selectionStart, input.selectionEnd, input.selectionDirection]).toEqual([
      2,
      7,
      "backward",
    ]);
    input.setSelectionRange(3, 3);
    await act(async () => vi.runAllTimersAsync());
    expect([input.selectionStart, input.selectionEnd]).toEqual([3, 3]);
  },
);

it("lets an explicit pointer action replace the previous selection", async () => {
  const input = leaveWindow();
  fireEvent.pointerDown(input);
  await restoreWithFocusTrap(input);
  expect([input.selectionStart, input.selectionEnd]).toEqual([0, 13]);
});

it("does not restore selection into a changed value", async () => {
  const input = leaveWindow();
  fireEvent.input(input, { target: { value: "New draft" } });
  await restoreWithFocusTrap(input);
  expect([input.selectionStart, input.selectionEnd]).toEqual([0, 9]);
});

it("discards a scheduled restoration when the input unmounts", async () => {
  const input = leaveWindow();
  await act(async () => {
    fireEvent.focus(input);
    input.select();
    cleanup();
  });
  expect([input.selectionStart, input.selectionEnd]).toEqual([0, 13]);
});

it("captures external input blur before another control becomes the active element", async () => {
  render(<Input />);
  const input = screen.getByRole("textbox") as HTMLInputElement;
  input.focus();
  input.setSelectionRange(7, 7);
  vi.spyOn(document, "hasFocus").mockReturnValue(false);
  fireEvent.blur(input, { relatedTarget: null });
  const trigger = document.createElement("button");
  document.body.append(trigger);
  trigger.focus();
  fireEvent(window, new Event("blur"));
  await act(async () => {
    input.focus();
    input.select();
    await vi.runAllTimersAsync();
  });
  expect([input.selectionStart, input.selectionEnd]).toEqual([7, 7]);
  trigger.remove();
});

it("restores after the dialog completes a deferred focus restoration", async () => {
  const input = leaveWindow();
  await act(async () => {
    fireEvent.focus(input);
    await Promise.resolve();
    input.select();
    await vi.runAllTimersAsync();
  });
  expect([input.selectionStart, input.selectionEnd]).toEqual([2, 7]);
});

it("keeps selection through multiple focus restorations until the next user action", async () => {
  const input = leaveWindow();
  await restoreWithFocusTrap(input);
  await restoreWithFocusTrap(input);
  expect([input.selectionStart, input.selectionEnd]).toEqual([2, 7]);
  fireEvent.keyDown(input, { key: "ArrowRight" });
  await restoreWithFocusTrap(input);
  expect([input.selectionStart, input.selectionEnd]).toEqual([0, 13]);
});
