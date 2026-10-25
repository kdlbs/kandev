import { createRef } from "react";
import { afterEach, describe, it, expect, vi } from "vitest";
import { cleanup, render, fireEvent } from "@testing-library/react";
import {
  CLARIFICATION_CUSTOM_TEXT_MAX_RUNES,
  ClarificationCustomInput,
  countRunes,
} from "./clarification-overlay-parts";
import { routePanelMouseDown } from "./route-panel-mouse-down";

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
const ROW_TESTID = "clarification-custom-input";
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

function renderInPanel(overrides: Partial<Parameters<typeof ClarificationCustomInput>[0]> = {}) {
  const panelRef = createRef<HTMLDivElement>();
  return render(
    <div
      data-testid="panel"
      tabIndex={-1}
      ref={panelRef}
      onMouseDown={(event) => routePanelMouseDown(event, panelRef)}
    >
      <ClarificationCustomInput {...makeProps(overrides)} />
    </div>,
  );
}

describe("ClarificationCustomInput row focus", () => {
  it("renders the editor as one flush, transparent input surface", () => {
    const { getByTestId } = renderInPanel();
    const row = getByTestId(ROW_TESTID);
    const input = getByTestId(INPUT_TESTID);

    expect(input.className).toContain("dark:bg-transparent");
    expect(row.textContent).not.toContain("↳");
  });

  it("centers every direct row control without manual top offsets", () => {
    const { getByTestId } = renderInPanel({ active: true });
    const row = getByTestId(ROW_TESTID);

    expect(row.className).toContain("items-center");
    expect(row.className).not.toContain("items-start");
    expect(Array.from(row.children).every((child) => !child.className.includes("mt-0.5"))).toBe(
      true,
    );
  });

  it("focuses the textarea when the non-interactive row surface is pressed", () => {
    const { getByTestId } = renderInPanel();

    const notDefaulted = fireEvent.mouseDown(getByTestId(ROW_TESTID));

    expect(notDefaulted).toBe(false);
    expect(document.activeElement).toBe(getByTestId(INPUT_TESTID));
  });

  it("does not focus the disabled textarea from the row surface", () => {
    const { getByTestId } = renderInPanel({ isSubmitting: true });

    fireEvent.mouseDown(getByTestId(ROW_TESTID));

    expect(document.activeElement).not.toBe(getByTestId(INPUT_TESTID));
  });

  it("does not route a touch Send press through the textarea", () => {
    pointer.isFinePointer = false;
    const onSubmit = vi.fn();
    const { getByTestId } = renderInPanel({ draft: "answer", onSubmit });

    fireEvent.mouseDown(getByTestId(TOUCH_SUBMIT_TESTID));
    fireEvent.click(getByTestId(TOUCH_SUBMIT_TESTID));

    expect(document.activeElement).not.toBe(getByTestId(INPUT_TESTID));
    expect(onSubmit).toHaveBeenCalledOnce();
  });
});

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

// W4a: the rune counter must count Unicode code points, not UTF-16 code
// units — every astral character (emoji, much CJK Extension B) is two UTF-16
// units but one rune, so counting units would reject valid input early.
describe("countRunes", () => {
  it("counts an astral character (surrogate pair) as one rune, not two", () => {
    const emoji = "\u{1F600}"; // grinning face, outside the BMP
    expect(emoji.length).toBe(2); // UTF-16 code units
    expect(countRunes(emoji)).toBe(1); // Unicode code points
  });

  it("counts plain ASCII 1:1", () => {
    expect(countRunes("hello")).toBe(5);
  });
});

// W4/W4a: the free-text input SHALL stop a human at the server's 2000-rune
// cap (N8b) rather than letting a too-long answer reach the server and fail
// with an opaque 400.
describe("ClarificationCustomInput rune limit", () => {
  const overLimitDraft = "a".repeat(CLARIFICATION_CUSTOM_TEXT_MAX_RUNES + 1);
  const atLimitDraft = "a".repeat(CLARIFICATION_CUSTOM_TEXT_MAX_RUNES);

  it("sets maxLength no lower than 4000 (double the rune cap) as a coarse backstop only", () => {
    const { getByTestId } = render(<ClarificationCustomInput {...makeProps()} />);
    const textarea = getByTestId(INPUT_TESTID) as HTMLTextAreaElement;
    expect(textarea.maxLength).toBeGreaterThanOrEqual(4000);
  });

  it("disables the touch Send button once the draft exceeds the rune cap", () => {
    pointer.isFinePointer = false;
    const onSubmit = vi.fn();
    const { getByTestId } = render(
      <ClarificationCustomInput {...makeProps({ draft: overLimitDraft, onSubmit })} />,
    );
    const button = getByTestId(TOUCH_SUBMIT_TESTID) as HTMLButtonElement;
    expect(button.disabled).toBe(true);
    fireEvent.click(button);
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("still allows sending a draft at exactly the rune cap (boundary: 2000 accepted)", () => {
    pointer.isFinePointer = false;
    const onSubmit = vi.fn();
    const { getByTestId } = render(
      <ClarificationCustomInput {...makeProps({ draft: atLimitDraft, onSubmit })} />,
    );
    const button = getByTestId(TOUCH_SUBMIT_TESTID) as HTMLButtonElement;
    expect(button.disabled).toBe(false);
  });

  it("blocks plain Enter from submitting an over-limit draft", () => {
    const onSubmit = vi.fn();
    const { getByTestId } = render(
      <ClarificationCustomInput {...makeProps({ draft: overLimitDraft, onSubmit })} />,
    );
    pressEnter(getByTestId(INPUT_TESTID));
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("blocks Cmd+Enter from finalizing an over-limit draft", () => {
    const onSubmit = vi.fn();
    const onRequestFinalSubmit = vi.fn();
    const { getByTestId } = render(
      <ClarificationCustomInput
        {...makeProps({ draft: overLimitDraft, onSubmit, onRequestFinalSubmit })}
      />,
    );
    pressEnter(getByTestId(INPUT_TESTID), { metaKey: true });
    expect(onRequestFinalSubmit).not.toHaveBeenCalled();
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("surfaces the boundary to the user once the draft is over the limit", () => {
    const { getByTestId } = render(
      <ClarificationCustomInput {...makeProps({ draft: overLimitDraft })} />,
    );
    const counter = getByTestId("clarification-input-rune-counter");
    expect(counter.getAttribute("data-over-limit")).toBe("true");
    // i18next leaves an unsubstituted placeholder verbatim in the rendered
    // text when a t() call omits a key the string interpolates — assert the
    // actual text, not just the data-over-limit flag, so a call site missing
    // one of the string's placeholders (e.g. `max`) fails here instead of
    // only being visible in a live screenshot.
    expect(counter.textContent).toBe(
      `1 character over the ${CLARIFICATION_CUSTOM_TEXT_MAX_RUNES}-character limit.`,
    );
    expect(counter.textContent).not.toContain("{{");
  });

  it("places the limit counter on a full-width row for narrow layouts", () => {
    const { getByTestId } = render(
      <ClarificationCustomInput {...makeProps({ draft: overLimitDraft })} />,
    );
    expect(getByTestId(ROW_TESTID).className).toContain("flex-wrap");
    expect(getByTestId(INPUT_TESTID).className).toContain("min-w-0");
    expect(getByTestId("clarification-input-rune-counter").parentElement?.className).toContain(
      "w-full",
    );
  });

  it("does not show a counter for a short draft far from the limit", () => {
    const { queryByTestId } = render(
      <ClarificationCustomInput {...makeProps({ draft: "short answer" })} />,
    );
    expect(queryByTestId("clarification-input-rune-counter")).toBeNull();
  });
});
