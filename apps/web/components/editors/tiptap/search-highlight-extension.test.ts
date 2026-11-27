import { Editor, Mark, Node, type JSONContent } from "@tiptap/core";
import StarterKit from "@tiptap/starter-kit";
import Link from "@tiptap/extension-link";
import Underline from "@tiptap/extension-underline";
import Highlight from "@tiptap/extension-highlight";
import { Table } from "@tiptap/extension-table";
import { TableRow } from "@tiptap/extension-table-row";
import { TableCell } from "@tiptap/extension-table-cell";
import { TableHeader } from "@tiptap/extension-table-header";
import type { DecorationSet } from "@tiptap/pm/view";
import { afterEach, describe, expect, it } from "vitest";
import { PlanSearchExtension, planSearchPluginKey } from "./search-highlight-extension";

const PHRASE = "release build";
const editors: Editor[] = [];
const Comment = Mark.create({
  name: "testComment",
  renderHTML: () => ["span", { "data-comment": "example" }, 0],
});
const InlineAtom = Node.create({
  name: "testAtom",
  inline: true,
  group: "inline",
  atom: true,
  renderHTML: () => ["span", { "data-atom": "example" }],
});
const InlineContainer = Node.create({
  name: "testContainer",
  inline: true,
  group: "inline",
  content: "text*",
  renderHTML: () => ["span", { "data-container": "example" }, 0],
});

function text(value: string, mark?: string): JSONContent {
  return {
    type: "text",
    text: value,
    ...(mark
      ? { marks: [{ type: mark, ...(mark === "link" ? { attrs: { href: "/" } } : {}) }] }
      : {}),
  };
}

function paragraph(...content: JSONContent[]): JSONContent {
  return { type: "paragraph", content };
}

function createEditor(...content: JSONContent[]): Editor {
  const editor = new Editor({
    extensions: [
      StarterKit.configure({ link: false, underline: false }),
      Link,
      Underline,
      Highlight,
      Table,
      TableRow,
      TableCell,
      TableHeader,
      Comment,
      InlineAtom,
      InlineContainer,
      PlanSearchExtension,
    ],
    content: { type: "doc", content },
  });
  editors.push(editor);
  return editor;
}

function searchState(editor: Editor) {
  return planSearchPluginKey.getState(editor.state)!;
}

function decorationRanges(editor: Editor) {
  const plugin = planSearchPluginKey.get(editor.state)!;
  const decorations = plugin.props.decorations?.call(plugin, editor.state) as DecorationSet | null;
  return decorations?.find().map(({ from, to }) => ({ from, to })) ?? [];
}

function currentText(editor: Editor): string {
  return Array.from(editor.view.dom.querySelectorAll(".search-highlight-current"))
    .map((element) => element.textContent)
    .join("");
}

afterEach(() => {
  editors.splice(0).forEach((editor) => editor.destroy());
});

describe("PlanSearchExtension marks", () => {
  // @covers AC-UI-PLAN-TEXT-SEARCH-001.1
  // @covers AC-UI-PLAN-TEXT-SEARCH-001.3
  it.each(["bold", "code", "italic", "link", "underline", "highlight", "testComment"])(
    "finds literal phrases across marks: %s",
    (mark) => {
      const editor = createEditor(paragraph(text("release "), text("build", mark)));
      const originalDocument = editor.getJSON();
      const originalSelection = editor.state.selection.toJSON();
      editor.commands.setPlanSearchQuery(PHRASE);
      expect(searchState(editor).matches).toEqual([{ from: 1, to: 14 }]);
      expect(decorationRanges(editor)).toEqual([{ from: 1, to: 14 }]);
      expect(currentText(editor)).toBe(PHRASE);
      expect(editor.getJSON()).toEqual(originalDocument);
      expect(editor.state.selection.toJSON()).toEqual(originalSelection);
    },
  );

  it("finds literal phrases across marks midway through a word and mixed marks", () => {
    const editor = createEditor(
      paragraph(text("release bu"), text("il", "italic"), text("d")),
      paragraph(
        text("re", "bold"),
        text("lease ", "italic"),
        text("bu", "code"),
        text("ild", "underline"),
      ),
    );
    editor.commands.setPlanSearchQuery(PHRASE);
    expect(searchState(editor).matches).toEqual([
      { from: 1, to: 14 },
      { from: 16, to: 29 },
    ]);
    expect(decorationRanges(editor)).toEqual(searchState(editor).matches);
  });
});

describe("PlanSearchExtension literal controls", () => {
  // @covers AC-UI-PLAN-TEXT-SEARCH-001.1
  // @covers AC-UI-PLAN-TEXT-SEARCH-001.4
  it.each([
    [PHRASE, PHRASE, [{ from: 1, to: 14 }]],
    ["RELEASE BUILD", PHRASE, [{ from: 1, to: 14 }]],
    ["a+b.[x] aabZx", "a+b.[x]", [{ from: 1, to: 8 }]],
    [PHRASE, "absent", []],
    [PHRASE, "", []],
    ["release  build", PHRASE, []],
    ["release  build", "release  build", [{ from: 1, to: 15 }]],
    ["aaaa", "aaa", [{ from: 1, to: 4 }]],
  ])("keeps plain literal search controls: %s / %s", (value, query, expected) => {
    const editor = createEditor(paragraph(text(value)));
    editor.commands.setPlanSearchQuery(query);
    expect(searchState(editor).matches).toEqual(expected);
    expect(decorationRanges(editor)).toEqual(expected);
  });
});

describe("PlanSearchExtension boundaries", () => {
  // @covers AC-UI-PLAN-TEXT-SEARCH-001.2
  it.each([
    ["hard break", { type: "hardBreak" }],
    ["inline atom", { type: "testAtom" }],
    ["inline container", { type: "testContainer", content: [text("hidden")] }],
  ])("stops at block and non-text inline boundaries: %s", (_name, boundary) => {
    const editor = createEditor(paragraph(text("release "), boundary, text("build")));
    editor.commands.setPlanSearchQuery(PHRASE);
    expect(searchState(editor).matches).toEqual([]);
    editor.commands.setPlanSearchQuery("hidden");
    expect(searchState(editor).matches).toEqual([]);
    editor.commands.setPlanSearchQuery("release");
    expect(searchState(editor).matches).toEqual([{ from: 1, to: 8 }]);
    editor.commands.setPlanSearchQuery("build");
    const start = boundary.type === "testContainer" ? 17 : 10;
    expect(searchState(editor).matches).toEqual([{ from: start, to: start + 5 }]);
  });

  it("stops at block boundaries, including empty blocks, and retains hits on both sides", () => {
    const editor = createEditor(
      paragraph(text("release ")),
      paragraph(),
      paragraph(text("build")),
      paragraph(text(PHRASE), { type: "hardBreak" }, text(PHRASE, "bold")),
    );
    editor.commands.setPlanSearchQuery(PHRASE);
    expect(searchState(editor).matches).toEqual([
      { from: 20, to: 33 },
      { from: 34, to: 47 },
    ]);
  });
});

describe("PlanSearchExtension document positions", () => {
  // @covers AC-UI-PLAN-TEXT-SEARCH-001.2
  // @covers AC-UI-PLAN-TEXT-SEARCH-001.3
  it.each([
    [
      "heading",
      { type: "heading", attrs: { level: 2 }, content: [text("release "), text("build", "bold")] },
      1,
    ],
    [
      "list",
      {
        type: "bulletList",
        content: [
          { type: "listItem", content: [paragraph(text("release "), text("build", "italic"))] },
        ],
      },
      3,
    ],
    [
      "quotation",
      { type: "blockquote", content: [paragraph(text("release "), text("build", "code"))] },
      2,
    ],
    [
      "table cell",
      {
        type: "table",
        content: [
          {
            type: "tableRow",
            content: [
              { type: "tableCell", content: [paragraph(text("release "), text("build", "bold"))] },
            ],
          },
        ],
      },
      4,
    ],
  ])("uses nested textblock document positions: %s", (_name, block, start) => {
    const editor = createEditor(block);
    editor.commands.setPlanSearchQuery(PHRASE);
    expect(searchState(editor).matches).toEqual([{ from: start, to: start + 13 }]);
    expect(decorationRanges(editor)).toEqual([{ from: start, to: start + 13 }]);
  });

  it("uses code-block text newlines without joining separate code blocks", () => {
    const editor = createEditor(
      { type: "codeBlock", content: [text("release\nbuild")] },
      { type: "codeBlock", content: [text("release ")] },
      { type: "codeBlock", content: [text("build")] },
    );
    editor.commands.setPlanSearchQuery("release\nbuild");
    expect(searchState(editor).matches).toEqual([{ from: 1, to: 14 }]);
    editor.commands.setPlanSearchQuery(PHRASE);
    expect(searchState(editor).matches).toEqual([]);
  });

  // @covers AC-UI-PLAN-TEXT-SEARCH-001.3
  it.each([
    [[text("🧭 release "), text("build", "bold")], PHRASE, { from: 4, to: 17 }],
    [
      [text("release "), text("🧭", "italic"), text("build", "code")],
      "release 🧭build",
      { from: 1, to: 16 },
    ],
  ])("uses UTF-16 match positions: %s", (content, query, expected) => {
    const editor = createEditor(paragraph(...content));
    editor.commands.setPlanSearchQuery(query);
    expect(searchState(editor).matches).toEqual([expected]);
    expect(decorationRanges(editor)).toEqual([expected]);
    expect(currentText(editor)).toBe(query);
  });
});

describe("PlanSearchExtension navigation and edits", () => {
  // @covers AC-UI-PLAN-TEXT-SEARCH-001.3
  // @covers AC-UI-PLAN-TEXT-SEARCH-001.4
  // @covers AC-UI-PLAN-TEXT-SEARCH-001.6
  it("counts logical matches and wraps navigation", () => {
    const editor = createEditor(
      paragraph(
        text("release "),
        text("build", "bold"),
        text(" release bu"),
        text("ild", "italic"),
      ),
    );
    editor.commands.setPlanSearchQuery(PHRASE);
    expect(searchState(editor).matches).toEqual([
      { from: 1, to: 14 },
      { from: 15, to: 28 },
    ]);
    expect(searchState(editor).current).toBe(0);
    for (const [command, index] of [
      ["planSearchPrev", 1],
      ["planSearchNext", 0],
      ["planSearchNext", 1],
      ["planSearchNext", 0],
    ] as const) {
      expect(editor.commands[command]()).toBe(true);
      expect(searchState(editor).current).toBe(index);
      expect(currentText(editor)).toBe(PHRASE);
    }
    editor.commands.planSearchNext();
    editor.commands.setPlanSearchQuery("BUILD");
    expect(searchState(editor).current).toBe(0);
    editor.commands.clearPlanSearch();
    expect(searchState(editor)).toEqual({ query: "", matches: [], current: 0 });
    expect(decorationRanges(editor)).toEqual([]);
    expect(currentText(editor)).toBe("");
    expect(editor.commands.planSearchNext()).toBe(false);
    expect(editor.commands.planSearchPrev()).toBe(false);
  });

  // @covers AC-UI-PLAN-TEXT-SEARCH-001.5
  it("recomputes after document edits and bounds the selected index", () => {
    const editor = createEditor(paragraph(text("release "), text("build", "bold")));
    editor.commands.setPlanSearchQuery(PHRASE);
    editor.view.dispatch(editor.state.tr.insertText(" release build", 14));
    expect(searchState(editor).matches).toEqual([
      { from: 1, to: 14 },
      { from: 15, to: 28 },
    ]);
    editor.commands.planSearchNext();
    editor.view.dispatch(editor.state.tr.insertText("🧭 ", 1));
    expect(searchState(editor).matches).toEqual([
      { from: 4, to: 17 },
      { from: 18, to: 31 },
    ]);
    expect(searchState(editor).current).toBe(1);
    expect(decorationRanges(editor)).toEqual(searchState(editor).matches);
    editor.view.dispatch(editor.state.tr.delete(18, 31));
    expect(searchState(editor).matches).toEqual([{ from: 4, to: 17 }]);
    expect(searchState(editor).current).toBe(0);
    expect(currentText(editor)).toBe(PHRASE);
    editor.commands.setContent({ type: "doc", content: [paragraph(text("new release build"))] });
    expect(searchState(editor).matches).toEqual([{ from: 5, to: 18 }]);
    expect(decorationRanges(editor)).toEqual([{ from: 5, to: 18 }]);
    editor.commands.setContent({ type: "doc", content: [paragraph()] });
    expect(searchState(editor)).toEqual({ query: PHRASE, matches: [], current: 0 });
    expect(decorationRanges(editor)).toEqual([]);
  });

  it("recomputes after formatting-only edits", () => {
    const editor = createEditor(paragraph(text(PHRASE)));
    editor.commands.setPlanSearchQuery(PHRASE);
    editor.view.dispatch(editor.state.tr.addMark(10, 12, editor.schema.marks.italic.create()));
    expect(searchState(editor).matches).toEqual([{ from: 1, to: 14 }]);
    expect(currentText(editor)).toBe(PHRASE);
    editor.view.dispatch(editor.state.tr.removeMark(10, 12, editor.schema.marks.italic));
    expect(searchState(editor).matches).toEqual([{ from: 1, to: 14 }]);
    expect(decorationRanges(editor)).toEqual([{ from: 1, to: 14 }]);
  });
});
