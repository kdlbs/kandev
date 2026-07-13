import { afterEach, describe, it, expect, vi } from "vitest";
import { cleanup, render, fireEvent } from "@testing-library/react";
import { ClarificationCustomInput } from "./clarification-overlay-parts";

// Mutable pointer state so individual tests can flip to a touch device without
// touching matchMedia internals.
const { pointer } = vi.hoisted(() => ({ pointer: { isFinePointer: true } }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => pointer,
}));

afterEach(() => {
  cleanup();
  pointer.isFinePointer = true;
});

const INPUT_TESTID = "clarification-input";
const TOUCH_SUBMIT_TESTID = "clarification-custom-submit";
const MULTILINE = "line one\nline two";

function makeProps(overrides: Partial<Parameters<typeof ClarificationCustomInput>[0]> = {}) {
  return {
    draft: "",
    isSubmitting: false,
    committedText: null,
    active: false,
    onChange: vi.fn(),
    onSubmit: vi.fn(),
    onRequestFinalSubmit: vi.fn(),
    ...overrides,
  };
}

// fireEvent.keyDown returns false when a handler called preventDefault.
function pressEnter(el: HTMLElement, init: Partial<KeyboardEventInit> = {}): boolean {
  return fireEvent.keyDown(el, { key: "Enter", ...init });
}

describe("ClarificationCustomInput multiline", () => {
  it("renders a textarea so answers can span multiple lines", () => {
    const { getByTestId } = render(<ClarificationCustomInput {...makeProps()} />);
    expect(getByTestId(INPUT_TESTID).tagName).toBe("TEXTAREA");
  });

  it("submits the trimmed draft on plain Enter", () => {
    const onSubmit = vi.fn();
    const { getByTestId } = render(
      <ClarificationCustomInput {...makeProps({ draft: "  hello  ", onSubmit })} />,
    );
    const notDefaulted = pressEnter(getByTestId(INPUT_TESTID));
    expect(notDefaulted).toBe(false); // preventDefault fired
    expect(onSubmit).toHaveBeenCalledTimes(1);
    expect(onSubmit).toHaveBeenCalledWith("hello");
  });

  it("swallows plain Enter on an empty draft without inserting a stray newline", () => {
    const onSubmit = vi.fn();
    const { getByTestId } = render(
      <ClarificationCustomInput {...makeProps({ draft: "   ", onSubmit })} />,
    );
    const notDefaulted = pressEnter(getByTestId(INPUT_TESTID));
    expect(notDefaulted).toBe(false); // preventDefault fired → no phantom newline
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("does NOT submit on Shift+Enter — the newline falls through to the textarea", () => {
    const onSubmit = vi.fn();
    const onRequestFinalSubmit = vi.fn();
    const { getByTestId } = render(
      <ClarificationCustomInput
        {...makeProps({ draft: "line one", onSubmit, onRequestFinalSubmit })}
      />,
    );
    const notDefaulted = pressEnter(getByTestId(INPUT_TESTID), { shiftKey: true });
    expect(notDefaulted).toBe(true); // default not prevented → newline inserted
    expect(onSubmit).not.toHaveBeenCalled();
    expect(onRequestFinalSubmit).not.toHaveBeenCalled();
  });

  it("preserves inner newlines when submitting a multi-line draft (trims ends only)", () => {
    const onSubmit = vi.fn();
    const { getByTestId } = render(
      <ClarificationCustomInput {...makeProps({ draft: `\n${MULTILINE}\n`, onSubmit })} />,
    );
    pressEnter(getByTestId(INPUT_TESTID));
    expect(onSubmit).toHaveBeenCalledWith(MULTILINE);
  });

  it("finalizes the bundle on Cmd+Enter without per-question submit", () => {
    const onSubmit = vi.fn();
    const onRequestFinalSubmit = vi.fn();
    const { getByTestId } = render(
      <ClarificationCustomInput
        {...makeProps({ draft: "answer", onSubmit, onRequestFinalSubmit })}
      />,
    );
    pressEnter(getByTestId(INPUT_TESTID), { metaKey: true });
    expect(onRequestFinalSubmit).toHaveBeenCalledTimes(1);
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("finalizes the bundle on Ctrl+Enter without per-question submit", () => {
    const onSubmit = vi.fn();
    const onRequestFinalSubmit = vi.fn();
    const { getByTestId } = render(
      <ClarificationCustomInput
        {...makeProps({ draft: "answer", onSubmit, onRequestFinalSubmit })}
      />,
    );
    pressEnter(getByTestId(INPUT_TESTID), { ctrlKey: true });
    expect(onRequestFinalSubmit).toHaveBeenCalledTimes(1);
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("ignores auto-repeat Enter but still suppresses the newline (no double submit)", () => {
    const onSubmit = vi.fn();
    const { getByTestId } = render(
      <ClarificationCustomInput {...makeProps({ draft: "answer", onSubmit })} />,
    );
    const notDefaulted = pressEnter(getByTestId(INPUT_TESTID), { repeat: true });
    // preventDefault still fires so a held key can't leak a newline into this or
    // the next question's textarea, but onSubmit does not run again.
    expect(notDefaulted).toBe(false);
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("does not submit while an IME candidate is being composed", () => {
    const onSubmit = vi.fn();
    const { getByTestId } = render(
      <ClarificationCustomInput {...makeProps({ draft: "候補", onSubmit })} />,
    );
    const notDefaulted = pressEnter(getByTestId(INPUT_TESTID), { isComposing: true });
    expect(notDefaulted).toBe(true); // default not prevented → IME confirms candidate
    expect(onSubmit).not.toHaveBeenCalled();
  });
});

describe("ClarificationCustomInput on touch devices", () => {
  it("Enter inserts a newline instead of submitting", () => {
    pointer.isFinePointer = false;
    const onSubmit = vi.fn();
    const { getByTestId } = render(
      <ClarificationCustomInput {...makeProps({ draft: "line one", onSubmit })} />,
    );
    const notDefaulted = pressEnter(getByTestId(INPUT_TESTID));
    expect(notDefaulted).toBe(true); // default not prevented → newline inserted
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("hides the keyboard hints on touch devices", () => {
    pointer.isFinePointer = false;
    const { queryByText } = render(<ClarificationCustomInput {...makeProps()} />);
    expect(queryByText("⇧↵ newline")).toBeNull();
  });

  it("shows a Send button on touch that submits the trimmed draft", () => {
    pointer.isFinePointer = false;
    const onSubmit = vi.fn();
    const { getByTestId } = render(
      <ClarificationCustomInput {...makeProps({ draft: `${MULTILINE} `, onSubmit })} />,
    );
    fireEvent.click(getByTestId(TOUCH_SUBMIT_TESTID));
    expect(onSubmit).toHaveBeenCalledWith(MULTILINE);
  });

  it("disables the touch Send button for an empty draft", () => {
    pointer.isFinePointer = false;
    const onSubmit = vi.fn();
    const { getByTestId } = render(
      <ClarificationCustomInput {...makeProps({ draft: "   ", onSubmit })} />,
    );
    const button = getByTestId(TOUCH_SUBMIT_TESTID) as HTMLButtonElement;
    expect(button.disabled).toBe(true);
    fireEvent.click(button);
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("disables the touch Send button while a submit is in flight", () => {
    pointer.isFinePointer = false;
    const onSubmit = vi.fn();
    const { getByTestId } = render(
      <ClarificationCustomInput
        {...makeProps({ draft: "answer", isSubmitting: true, onSubmit })}
      />,
    );
    const button = getByTestId(TOUCH_SUBMIT_TESTID) as HTMLButtonElement;
    expect(button.disabled).toBe(true);
    fireEvent.click(button);
    expect(onSubmit).not.toHaveBeenCalled();
  });
});
