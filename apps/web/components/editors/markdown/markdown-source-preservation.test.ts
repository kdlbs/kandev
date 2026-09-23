import { describe, expect, it } from "vitest";
import {
  applyMarkdownSourceEdit,
  type MarkdownSourceReplacement,
} from "./markdown-source-preservation";

describe("applyMarkdownSourceEdit", () => {
  it("applies multiple replacements from right to left without rewriting untouched source", () => {
    const source = [
      "\uFEFF---",
      "title: Keep exact bytes",
      "---",
      "",
      "# Heading",
      "",
      '<div onclick="alert(1)">raw HTML</div>',
      "",
      "```mdx",
      "<Widget value={1} />",
      "```",
      "",
    ].join("\n");
    const headingStart = source.indexOf("Heading");
    const htmlStart = source.indexOf("raw HTML");
    const replacements: MarkdownSourceReplacement[] = [
      { start: headingStart, endExclusive: headingStart + "Heading".length, newText: "Renamed" },
      { start: htmlStart, endExclusive: htmlStart + "raw HTML".length, newText: "edited HTML" },
    ];

    const result = applyMarkdownSourceEdit(source, replacements);

    expect(result).toBe(
      [
        "\uFEFF---",
        "title: Keep exact bytes",
        "---",
        "",
        "# Renamed",
        "",
        '<div onclick="alert(1)">edited HTML</div>',
        "",
        "```mdx",
        "<Widget value={1} />",
        "```",
        "",
      ].join("\n"),
    );
    expect(result.slice(0, source.indexOf("# Heading"))).toBe(
      source.slice(0, source.indexOf("# Heading")),
    );
    expect(result.slice(result.indexOf("```mdx"))).toBe(source.slice(source.indexOf("```mdx")));
  });

  it("allows an insertion at a source boundary and rejects overlapping ranges", () => {
    expect(
      applyMarkdownSourceEdit("alpha\nomega", [
        { start: 5, endExclusive: 5, newText: "\ninserted" },
      ]),
    ).toBe("alpha\ninserted\nomega");

    expect(() =>
      applyMarkdownSourceEdit("abcdef", [
        { start: 1, endExclusive: 4, newText: "x" },
        { start: 3, endExclusive: 5, newText: "y" },
      ]),
    ).toThrowError("Markdown source edits must not overlap");
  });

  it("keeps unsupported constructs unchanged when another block is edited", () => {
    const unsupported = ["<Component prop={value} />", "", "<!-- keep this comment -->"].join("\n");
    const source = `# Before\n\n${unsupported}\n\n# After`;
    const start = source.indexOf("Before");

    const result = applyMarkdownSourceEdit(source, [
      { start, endExclusive: start + "Before".length, newText: "Changed" },
    ]);

    expect(result).toContain(unsupported);
  });
});
