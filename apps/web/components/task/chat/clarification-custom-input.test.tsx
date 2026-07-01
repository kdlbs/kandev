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
      <ClarificationCustomInput {...makeProps({ draft: "\nline one\nline two\n", onSubmit })} />,
    );
    pressEnter(getByTestId(INPUT_TESTID));
    expect(onSubmit).toHaveBeenCalledWith("line one\nline two");
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

  it("on touch devices Enter inserts a newline instead of submitting", () => {
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
});
