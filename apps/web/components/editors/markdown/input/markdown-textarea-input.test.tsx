import { afterEach, describe, expect, it, vi } from "vitest";
import { createMarkdownTextareaInput } from "./markdown-textarea-input";
import type { MarkdownInputState } from "./markdown-input-transport";

afterEach(() => document.body.replaceChildren());
function setup(source = "\uFEFFA\r\n😀\nlast\r\n") {
  const host = document.createElement("div");
  host.tabIndex = 0;
  document.body.append(host);
  const onSelection = vi.fn();
  const input = createMarkdownTextareaInput(
    { host, text: source, getCaretRect: () => undefined, onSelection },
    "Markdown content",
  );
  input.attach(host);
  const state: MarkdownInputState = {
    source,
    anchor: 0,
    active: 0,
    readOnly: false,
    instanceId: Symbol(),
    sourceRevision: 1,
  };
  input.syncState(state);
  const textarea = host.shadowRoot!.querySelector("textarea")!;
  return { host, input, textarea, state, onSelection };
}

describe("scoped native Markdown input", () => {
  it("exposes normalized accessible text while retaining exact canonical context", () => {
    const { input, textarea } = setup();
    expect(textarea.value).toBe("\uFEFFA\n😀\nlast\n");
    expect(input.context.text).toBe("\uFEFFA\r\n😀\nlast\r\n");
    expect(textarea.getAttribute("aria-label")).toBe("Markdown content");
    input.dispose();
  });
  it("publishes only the exact canonical replacement and maps the native selection afterward", () => {
    const { input, textarea, state, onSelection } = setup();
    const edits = vi.fn();
    input.context.addEventListener("textupdate", edits);
    textarea.setSelectionRange(3, 5);
    textarea.dispatchEvent(
      new InputEvent("beforeinput", { inputType: "insertText", data: "x", bubbles: true }),
    );
    textarea.setRangeText("x", 3, 5, "end");
    // Happy DOM incorrectly uses the entire value length for setRangeText end mode.
    textarea.setSelectionRange(4, 4);
    textarea.dispatchEvent(
      new InputEvent("input", { inputType: "insertText", data: "x", bubbles: true }),
    );
    expect(edits).toHaveBeenCalledOnce();
    expect(edits.mock.calls[0][0]).toMatchObject({
      updateRangeStart: 4,
      updateRangeEnd: 6,
      text: "x",
    });
    expect(input.context.text).toBe("\uFEFFA\r\nx\nlast\r\n");
    expect(onSelection).toHaveBeenLastCalledWith({
      instanceId: state.instanceId,
      sourceRevision: 1,
      anchor: 5,
      active: 5,
    });
    input.dispose();
  });

  it("maps backward native selection without text edits, including read-only selection", () => {
    const { input, textarea, state, onSelection } = setup();
    input.syncState({ ...state, readOnly: true });
    const edits = vi.fn();
    input.context.addEventListener("textupdate", edits);
    textarea.setSelectionRange(3, 5, "backward");
    textarea.dispatchEvent(new Event("select"));
    expect(onSelection).toHaveBeenLastCalledWith({
      anchor: 6,
      active: 4,
      instanceId: state.instanceId,
      sourceRevision: 1,
    });
    expect(edits).not.toHaveBeenCalled();
    input.dispose();
  });

  it("synchronizes engine selection without publishing a native echo", () => {
    const { input, textarea, state, onSelection } = setup();
    input.context.updateSelection(4, 6);
    input.syncState({ ...state, anchor: 6, active: 4 });
    textarea.dispatchEvent(new Event("select"));
    expect([textarea.selectionStart, textarea.selectionEnd, textarea.selectionDirection]).toEqual([
      3,
      5,
      "backward",
    ]);
    expect(onSelection).not.toHaveBeenCalled();
    input.dispose();
  });

  it("routes native history intent through the host command owner and prevents native undo", () => {
    const { host, input, textarea } = setup("abc");
    const command = vi.fn((event: KeyboardEvent) => event.preventDefault());
    host.addEventListener("keydown", command);
    const event = new InputEvent("beforeinput", { inputType: "historyUndo", cancelable: true });
    textarea.dispatchEvent(event);
    expect(command).toHaveBeenCalledWith(expect.objectContaining({ key: "z", target: host }));
    expect(event.defaultPrevented).toBe(true);
    input.dispose();
  });
});

describe("native composition and exact source", () => {
  it("keeps the same native node, value and selection through composition echoes", () => {
    const { host, input, textarea, state } = setup("abc");
    textarea.setSelectionRange(1, 2);
    textarea.dispatchEvent(new CompositionEvent("compositionstart"));
    textarea.value = "a漢c";
    textarea.setSelectionRange(2, 2);
    textarea.dispatchEvent(
      new InputEvent("input", { inputType: "insertCompositionText", isComposing: true }),
    );
    input.syncState({ ...state, source: "a漢c", anchor: 0, active: 0, sourceRevision: 2 });
    expect(host.shadowRoot!.querySelector("textarea")).toBe(textarea);
    expect([textarea.value, textarea.selectionStart]).toEqual(["a漢c", 2]);
    textarea.dispatchEvent(new CompositionEvent("compositionend"));
    expect(input.context.text).toBe("a漢c");
    input.dispose();
  });

  it("delivers composed native clipboard and keyboard events to their owning host", () => {
    const { host, input, textarea } = setup();
    const clipboard = vi.fn();
    const keydown = vi.fn();
    host.addEventListener("paste", clipboard);
    host.addEventListener("keydown", keydown);
    textarea.dispatchEvent(new Event("paste", { bubbles: true, composed: true }));
    textarea.dispatchEvent(
      new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true, composed: true }),
    );
    expect(clipboard).toHaveBeenCalledOnce();
    expect(keydown).toHaveBeenCalledOnce();
    input.dispose();
  });

  it("anchors repeated normalized newline deletion to the native range", () => {
    const { input, textarea } = setup("a\n\r\nb");
    const edits = vi.fn();
    input.context.addEventListener("textupdate", edits);
    textarea.setSelectionRange(1, 2);
    textarea.dispatchEvent(new InputEvent("beforeinput", { inputType: "deleteContentBackward" }));
    textarea.value = "a\nb";
    textarea.setSelectionRange(1, 1);
    textarea.dispatchEvent(new InputEvent("input", { inputType: "deleteContentBackward" }));
    expect(edits).toHaveBeenCalledOnce();
    expect(edits.mock.calls[0][0]).toMatchObject({
      updateRangeStart: 1,
      updateRangeEnd: 2,
      text: "",
    });
    expect(input.context.text).toBe("a\r\nb");
    input.dispose();
  });

  it("defers the newest outside replacement until native composition settles", () => {
    const { input, textarea } = setup("abc");
    const stale = vi.fn();
    const newest = vi.fn();
    textarea.dispatchEvent(new CompositionEvent("compositionstart"));
    input.runWhenSettled(stale);
    input.runWhenSettled(newest);
    expect(newest).not.toHaveBeenCalled();
    textarea.dispatchEvent(new CompositionEvent("compositionend"));
    expect(stale).not.toHaveBeenCalled();
    expect(newest).toHaveBeenCalledOnce();
    input.dispose();
  });
});

describe("native accessibility and geometry", () => {
  it("keeps decorative text hidden from accessibility while retaining native input and controls", async () => {
    const { host, input, textarea } = setup();
    const paragraph = document.createElement("p");
    paragraph.className = "md-text";
    paragraph.textContent = "Rendered decorative text";
    const button = document.createElement("button");
    const buttonLabel = document.createElement("span");
    buttonLabel.className = "md-text";
    buttonLabel.textContent = "Table action";
    button.append(buttonLabel);
    host.append(paragraph, button);
    await Promise.resolve();
    expect(paragraph.getAttribute("aria-hidden")).toBe("true");
    expect(button.closest('[aria-hidden="true"]')).toBeNull();
    expect(buttonLabel.getAttribute("aria-hidden")).toBeNull();
    expect(host.getAttribute("aria-hidden")).toBeNull();
    expect(textarea.closest('[aria-hidden="true"]')).toBeNull();
    input.dispose();
  });

  it("positions native input using the engine overlay transform and updates after scrolling", () => {
    const host = document.createElement("div");
    document.body.append(host);
    const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    svg.setAttribute("class", "md-selection-layer");
    let clientTop = 200;
    Object.defineProperty(svg, "getScreenCTM", {
      value: () => ({ a: 2, d: 2, e: 100, f: clientTop }),
    });
    host.append(svg);
    const input = createMarkdownTextareaInput(
      {
        host,
        text: "abc",
        getCaretRect: () => ({ left: 10, top: 20, width: 0, height: 18 }) as never,
        onSelection: vi.fn(),
      },
      "Markdown content",
    );
    input.attach(host);
    input.syncState({
      source: "abc",
      anchor: 0,
      active: 0,
      readOnly: false,
      instanceId: Symbol(),
      sourceRevision: 1,
    });
    const textarea = host.shadowRoot!.querySelector("textarea")!;
    expect([textarea.style.left, textarea.style.top, textarea.style.height]).toEqual([
      "120px",
      "240px",
      "36px",
    ]);
    clientTop = 150;
    document.dispatchEvent(new Event("scroll"));
    expect(textarea.style.top).toBe("190px");
    textarea.dispatchEvent(new CompositionEvent("compositionstart"));
    clientTop = 120;
    input.syncState({
      source: "abc",
      anchor: 1,
      active: 1,
      readOnly: false,
      instanceId: Symbol(),
      sourceRevision: 1,
    });
    expect(textarea.style.top).toBe("160px");
    input.dispose();
  });
});

describe("native transport lifetime", () => {
  it("ignores native selection while suspended and publishes selection again after resume", () => {
    const { input, textarea, state, onSelection } = setup();
    input.setSuspended(true);
    textarea.setSelectionRange(3, 5, "backward");
    textarea.dispatchEvent(new Event("select"));
    expect(onSelection).not.toHaveBeenCalled();
    input.setSuspended(false);
    textarea.setSelectionRange(1, 2);
    textarea.dispatchEvent(new Event("select"));
    expect(onSelection).toHaveBeenLastCalledWith({
      anchor: 1,
      active: 2,
      instanceId: state.instanceId,
      sourceRevision: 1,
    });
    input.dispose();
  });

  it("isolates two input instances and removes late input, selection and deferred callbacks on disposal", () => {
    const first = setup("first");
    const second = setup("second");
    const edits = vi.fn();
    first.input.context.addEventListener("textupdate", edits);
    first.textarea.dispatchEvent(new CompositionEvent("compositionstart"));
    const pending = vi.fn();
    first.input.runWhenSettled(pending);
    first.input.dispose();
    first.input.dispose();
    first.textarea.value = "late";
    first.textarea.dispatchEvent(new InputEvent("input", { inputType: "insertText" }));
    first.textarea.dispatchEvent(new Event("select"));
    first.textarea.dispatchEvent(new CompositionEvent("compositionend"));
    expect([edits.mock.calls.length, pending.mock.calls.length]).toEqual([0, 0]);
    expect(first.input.context.text).toBe("first");
    expect(second.textarea.value).toBe("second");
    second.input.dispose();
  });

  it("settles independent document and read-only changes without dropping either owner", () => {
    const { input, textarea } = setup("abc");
    const replacement = vi.fn();
    const readOnly = vi.fn();
    textarea.dispatchEvent(new CompositionEvent("compositionstart"));
    input.runWhenSettled(replacement, Symbol());
    input.runWhenSettled(readOnly, Symbol());
    textarea.dispatchEvent(new CompositionEvent("compositionend"));
    expect(replacement).toHaveBeenCalledOnce();
    expect(readOnly).toHaveBeenCalledOnce();
    input.dispose();
  });
});
