import { afterEach, expect, it } from "vitest";
const inputModule =
  "monaco-editor/esm/vs/editor/browser/controller/editContext/textArea/textAreaEditContextInput.js";
const stateModule =
  "monaco-editor/esm/vs/editor/browser/controller/editContext/textArea/textAreaEditContextState.js";
const { TextAreaWrapper } = await import(inputModule);
const { TextAreaState } = await import(stateModule);

afterEach(() => document.body.replaceChildren());
for (const focused of [false, true]) {
  it(`preserves backward native selection when ${focused ? "already focused" : "taking focus"}`, () => {
    const textarea = document.createElement("textarea");
    textarea.value = "before Long after";
    document.body.append(textarea);
    if (focused) textarea.focus();
    const wrapper = new TextAreaWrapper(textarea);
    try {
      wrapper.setSelectionRange("restore", 11, 7);
      expect({
        start: textarea.selectionStart,
        end: textarea.selectionEnd,
        direction: textarea.selectionDirection,
      }).toEqual({ start: 7, end: 11, direction: "backward" });
      expect(wrapper.getSelectionStart()).toBe(11);
      expect(wrapper.getSelectionEnd()).toBe(7);
    } finally {
      wrapper.dispose();
    }
  });
}
for (const [anchor, active] of [
  [7, 11],
  [11, 7],
]) {
  it(`deduces native selection replacement ${anchor} -> ${active} as ordinary typing`, () => {
    const previous = new TextAreaState("before Long after", anchor, active, null, undefined);
    const current = new TextAreaState("before S after", 8, 8, null, undefined);
    expect(TextAreaState.deduceInput(previous, current, false)).toEqual({
      text: "S",
      replacePrevCharCnt: 0,
      replaceNextCharCnt: 0,
      positionDelta: 0,
    });
  });
}
