import { afterEach, describe, it, expect, vi } from "vitest";
import { cleanup, render, fireEvent } from "@testing-library/react";
import { ClarificationCustomInput } from "./clarification-overlay-parts";

afterEach(cleanup);

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
    fireEvent.keyDown(getByTestId(INPUT_TESTID), { key: "Enter" });
    expect(onSubmit).toHaveBeenCalledTimes(1);
    expect(onSubmit).toHaveBeenCalledWith("hello");
  });

  it("does NOT submit on Shift+Enter — the newline falls through to the textarea", () => {
    const onSubmit = vi.fn();
    const onRequestFinalSubmit = vi.fn();
    const { getByTestId } = render(
      <ClarificationCustomInput
        {...makeProps({ draft: "line one", onSubmit, onRequestFinalSubmit })}
      />,
    );
    fireEvent.keyDown(getByTestId(INPUT_TESTID), { key: "Enter", shiftKey: true });
    expect(onSubmit).not.toHaveBeenCalled();
    expect(onRequestFinalSubmit).not.toHaveBeenCalled();
  });

  it("preserves inner newlines when submitting a multi-line draft (trims ends only)", () => {
    const onSubmit = vi.fn();
    const { getByTestId } = render(
      <ClarificationCustomInput {...makeProps({ draft: "\nline one\nline two\n", onSubmit })} />,
    );
    fireEvent.keyDown(getByTestId(INPUT_TESTID), { key: "Enter" });
    expect(onSubmit).toHaveBeenCalledWith("line one\nline two");
  });

  it("finalizes the bundle on Cmd/Ctrl+Enter without per-question submit", () => {
    const onSubmit = vi.fn();
    const onRequestFinalSubmit = vi.fn();
    const { getByTestId } = render(
      <ClarificationCustomInput
        {...makeProps({ draft: "answer", onSubmit, onRequestFinalSubmit })}
      />,
    );
    fireEvent.keyDown(getByTestId(INPUT_TESTID), { key: "Enter", metaKey: true });
    expect(onRequestFinalSubmit).toHaveBeenCalledTimes(1);
    expect(onSubmit).not.toHaveBeenCalled();
  });
});
