import { expect, it } from "vitest";
import { EditorState, Compartment } from "@codemirror/state";
import { codeMirrorTheme } from "./codemirror-theme";
it("reconfigures an existing document and selection with a contributed palette", () => {
  const slot = new Compartment();
  const make = (digest: string, base: "light" | "dark") =>
    codeMirrorTheme({
      reference: "theme:test/night",
      base,
      digest,
      status: "ready",
      definition: {
        schema_version: 1,
        base,
        editor: { background: "#272822" },
        syntax: { keyword: { foreground: "#f92672" } },
      },
    });
  let state = EditorState.create({
    doc: 'const value = "hello";',
    selection: { anchor: 6, head: 11 },
    extensions: [slot.of(make("a", "dark"))],
  });
  state = state.update({ changes: { from: 11, insert: "Changed" } }).state;
  const before = state.doc.toString(),
    selection = state.selection;
  state = state.update({ effects: slot.reconfigure(make("b", "light")) }).state;
  expect(state.doc.toString()).toBe(before);
  expect(state.selection).toEqual(selection);
});
