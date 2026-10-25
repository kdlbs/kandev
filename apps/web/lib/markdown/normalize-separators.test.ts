import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import ReactMarkdown from "react-markdown";
import remarkBreaks from "remark-breaks";
import remarkGemoji from "remark-gemoji";
import remarkGfm from "remark-gfm";
import { describe, expect, it } from "vitest";
import { normalizeCached, normalizeMarkdown, __resetMarkdownCounters } from "./normalize-cache";

const LONG_PROSE =
  "This paragraph explains the section in enough detail that the following separator must remain a rule.";
const BOUNDARY_WORDS = [
  "to",
  "the",
  "text",
  "words",
  "detail",
  "context",
  "section",
  "ordinary",
  "paragraph",
  "separator",
];

function proseWithLength(target: number): string {
  const memo = new Map<number, string[] | null>();

  function findWords(remaining: number): string[] | null {
    if (remaining === 0) return [];
    if (remaining < 3) return null;
    if (memo.has(remaining)) return memo.get(remaining) ?? null;

    for (const word of BOUNDARY_WORDS) {
      const additionLength = 1 + Array.from(word).length;
      if (additionLength > remaining) continue;
      const suffix = findWords(remaining - additionLength);
      if (suffix) {
        const result = [word, ...suffix];
        memo.set(remaining, result);
        return result;
      }
    }

    memo.set(remaining, null);
    return null;
  }

  const prefix = "Boundary";
  const suffix = findWords(target - Array.from(prefix).length);
  if (!suffix) throw new Error(`Unable to create prose with ${target} code points`);
  return [prefix, ...suffix].join(" ");
}

function countTag(html: string, tag: string): number {
  return html.match(new RegExp(`<${tag}(?:\\s|/?>)`, "g"))?.length ?? 0;
}

function renderMarkdown(source: string): string {
  return renderToStaticMarkup(
    createElement(
      ReactMarkdown,
      { remarkPlugins: [remarkGfm, remarkBreaks, remarkGemoji] },
      source,
    ),
  );
}

function expectMixedFenceLiteral(input: string): void {
  const normalized = normalizeMarkdown(input);
  const rendered = renderMarkdown(normalized);

  expect(normalized).toBe(input);
  expect(rendered).toContain("<pre><code");
  expect(rendered).toContain(LONG_PROSE);
  expect(rendered).toContain("---");
  expect(countTag(rendered, "h2")).toBe(0);
  expect(countTag(rendered, "hr")).toBe(0);
}

describe("normalizeMarkdown prose separators", () => {
  it("@covers AC-UI-COMMENT-MARKDOWN-002.1 renders eligible prose before a rule", () => {
    const input = `${LONG_PROSE}\n---\nNext section`;

    const normalized = normalizeMarkdown(input);
    const rendered = renderMarkdown(normalized);

    expect(normalized).toBe(`${LONG_PROSE}\n\n---\nNext section`);
    expect(rendered).toContain(`<p>${LONG_PROSE}</p>`);
    expect(rendered).toContain("<hr/>");
    expect(rendered).toContain("<p>Next section</p>");
  });

  it("@covers AC-UI-COMMENT-MARKDOWN-002.1 and .2 applies the documented thresholds", () => {
    const punctuated39 = `${proseWithLength(38)}.`;
    const punctuated40 = `${proseWithLength(39)}.`;
    const unpunctuated69 = proseWithLength(69);
    const unpunctuated70 = proseWithLength(70);

    expect(Array.from(punctuated39).length).toBe(39);
    expect(Array.from(punctuated40).length).toBe(40);
    expect(Array.from(unpunctuated69).length).toBe(69);
    expect(Array.from(unpunctuated70).length).toBe(70);
    expect(normalizeMarkdown(`${punctuated39}\n---`)).toBe(`${punctuated39}\n---`);
    expect(normalizeMarkdown(`${punctuated40}\n---`)).toBe(`${punctuated40}\n\n---`);
    expect(normalizeMarkdown(`${unpunctuated69}\n---`)).toBe(`${unpunctuated69}\n---`);
    expect(normalizeMarkdown(`${unpunctuated70}\n---`)).toBe(`${unpunctuated70}\n\n---`);
    expect(normalizeMarkdown(`${proseWithLength(39)}.**\n---`)).toBe(
      `${proseWithLength(39)}.**\n\n---`,
    );
  });

  it("@covers AC-UI-COMMENT-MARKDOWN-002.2 preserves short setext titles", () => {
    const inputs = [
      "Skills to change\n---",
      "Done.\n---",
      "**Inline title**\n---",
      "First line\nsecond line\n---",
    ];

    for (const input of inputs) expect(normalizeMarkdown(input)).toBe(input);

    const rendered = renderMarkdown(normalizeMarkdown(inputs[0]));
    expect(countTag(rendered, "h2")).toBe(1);
    expect(rendered).toContain("Skills to change");
  });

  it("@covers AC-UI-COMMENT-MARKDOWN-002.1 repairs the supplied skills excerpt", () => {
    const input = [
      "## Skills to change",
      "**`/spec-delta` and `/spec-archive`**",
      "Living spec path becomes `docs/specs/<system>/requirements/<capability>.md`, not `go/cmd/<service>/specs/`. Deltas stay under `docs/intake/gsp/<id>/lld/specs/<system>/`. Add a domain column so a GSP that changes item resolution writes `lld/specs/catalogue/item-resolution.md`, not a copy under every consuming service.",
      "**`/lld` Phase 7**",
      "When the LLD touches `go/internal/catalogue` or `go/internal/composable`, emit a domain delta plus service deltas that only cover HTTP projection (status codes, cache headers, error envelope). Service deltas must reference domain REQ IDs, not restating the SHALL.",
      "**`/feature` Phase 3 and 7**",
      "Phase 3 should produce (or update) requirements + system design, not only a file map in chat. Phase 7 already calls `/spec-delta`; point it at `docs/specs/`. Skip only for refactors, perf, and config.",
      "**`/fix`**",
      "After root cause: if the bug is missing specified behaviour, add or correct the requirement before the regression test. If the requirement already exists, the test is the AC.",
      "**`/tdd`**",
      "CLIP's Red-Green-Refactor is fine. Add one line: implement against the current requirement / work order; name the REQ or scenario in the test.",
      "**Leave as-is**",
      "`/hld`, `/record`, `/task-spec`, `/code-review`, `/verify`. `/hld` is CLIP's GSP brief. `/record` already owns ADRs.",
      "---",
    ].join("\n");

    const normalized = normalizeMarkdown(input);
    const rendered = renderMarkdown(normalized);

    expect(normalized).toContain("ADRs.\n\n---");
    expect(countTag(rendered, "h2")).toBe(1);
    expect(countTag(rendered, "hr")).toBe(1);
    expect(rendered).toContain("CLIP&#x27;s Red-Green-Refactor");
  });
});

describe("normalizeMarkdown protected separator contexts", () => {
  it("@covers AC-UI-COMMENT-MARKDOWN-002.3 and .5 preserves mixed fence markers", () => {
    const cases = [
      ["backtick fence", ["```text", "`~~", LONG_PROSE, "---", "```"].join("\n")],
      ["backtick info string", ["```~text", LONG_PROSE, "---", "```"].join("\n")],
      ["tilde fence", ["~~~text", "~``", LONG_PROSE, "---", "~~~"].join("\n")],
    ] as const;

    for (const [, input] of cases) expectMixedFenceLiteral(input);
  });

  it("@covers AC-UI-COMMENT-MARKDOWN-002.3 preserves protected contexts", () => {
    const long = proseWithLength(70);
    const inputs = [
      ["tilde fence", ["~~~markdown", long, "---"].join("\n")],
      ["longer enclosing fence", ["````", long, "---", "```"].join("\n")],
      ["mismatched fence", ["~~~", long, "```", "---"].join("\n")],
      ["unclosed fence", ["```", long, "---"].join("\n")],
      ["ATX heading", `# ${long}\n---`],
      ["list item", `- ${long}\n---`],
      ["blockquote", `> ${long}\n---`],
      ["table row", `| ${long} | value |\n---`],
      ["lazy list continuation", `- item\n${long}\n---`],
      ["lazy blockquote continuation", `> item\n${long}\n---`],
      ["HTML block", `<div>\n${long}\n---\n</div>`],
      ["four-space indentation", `    ${long}\n---`],
      ["tab indentation", `\t${long}\n---`],
    ] as const;

    for (const [, input] of inputs) expect(normalizeMarkdown(input)).toBe(input);
  });

  it("@covers AC-UI-COMMENT-MARKDOWN-002.3 resumes after definite container boundaries", () => {
    const cases = [
      [
        `- item\n## Next section\n${LONG_PROSE}\n---`,
        `- item\n## Next section\n${LONG_PROSE}\n\n---`,
      ],
      [
        `> item\n# Next section\n${LONG_PROSE}\n---`,
        `> item\n# Next section\n${LONG_PROSE}\n\n---`,
      ],
      [`- item\n---\n${LONG_PROSE}\n---`, `- item\n---\n${LONG_PROSE}\n\n---`],
      [`> item\n---\n${LONG_PROSE}\n---`, `> item\n---\n${LONG_PROSE}\n\n---`],
      [
        `> ~~~text\n> literal\n> ~~~\n${LONG_PROSE}\n---`,
        `> ~~~text\n> literal\n> ~~~\n${LONG_PROSE}\n\n---`,
      ],
      [`- item\n\n${LONG_PROSE}\n---`, `- item\n\n${LONG_PROSE}\n\n---`],
    ] as const;

    for (const [input, expected] of cases) expect(normalizeMarkdown(input)).toBe(expected);
  });
});

describe("normalizeMarkdown protected HTML and list contexts", () => {
  it("@covers AC-UI-COMMENT-MARKDOWN-002.3 and .4 applies HTML block termination rules", () => {
    const afterBasicBlocks = [
      [`<hr>\n\n${LONG_PROSE}\n---`, `<hr>\n\n${LONG_PROSE}\n\n---`],
      [`<div>\n\n${LONG_PROSE}\n---`, `<div>\n\n${LONG_PROSE}\n\n---`],
      [`<pre>literal</pre>\n${LONG_PROSE}\n---`, `<pre>literal</pre>\n${LONG_PROSE}\n\n---`],
    ] as const;
    for (const [input, expected] of afterBasicBlocks)
      expect(normalizeMarkdown(input)).toBe(expected);

    const basicUntilBlank = `<div>\n${LONG_PROSE}\n</div>\n${LONG_PROSE}\n---`;
    expect(normalizeMarkdown(basicUntilBlank)).toBe(basicUntilBlank);

    const completeHtmlBlocks = [
      `<custom-element>\n${LONG_PROSE}\n---`,
      `</div>\n${LONG_PROSE}\n---`,
      `<div></div>\n${LONG_PROSE}\n---`,
      `<?instruction\n${LONG_PROSE}\n---\n?>`,
      `<!DOCTYPE\n${LONG_PROSE}\n---\n>`,
    ];
    for (const input of completeHtmlBlocks) expect(normalizeMarkdown(input)).toBe(input);

    const rawAndComment = [
      [
        `<pre>\n\n${LONG_PROSE}\n---\n</pre>\n\n${LONG_PROSE}\n---`,
        `<pre>\n\n${LONG_PROSE}\n---\n</pre>\n\n${LONG_PROSE}\n\n---`,
      ],
      [
        `<!--\n\n${LONG_PROSE}\n---\n-->\n\n${LONG_PROSE}\n---`,
        `<!--\n\n${LONG_PROSE}\n---\n-->\n\n${LONG_PROSE}\n\n---`,
      ],
    ] as const;
    for (const [input, expected] of rawAndComment) {
      expect(normalizeMarkdown(input)).toBe(expected);
    }
  });

  it("@covers AC-UI-COMMENT-MARKDOWN-002.3 and .5 protects list-owned fences across blank lines", () => {
    const input = ["- ~~~text", "", `  ${LONG_PROSE}`, "  ---", "  ~~~"].join("\n");
    const normalized = normalizeMarkdown(input);
    const rendered = renderMarkdown(normalized);

    expect(normalized).toBe(input);
    expect(rendered).toContain("<pre><code");
    expect(rendered).toContain(LONG_PROSE);
    expect(rendered).toContain("---");
    expect(countTag(rendered, "hr")).toBe(0);
  });

  it("@covers AC-UI-COMMENT-MARKDOWN-002.3 preserves loose-list ownership after a blank", () => {
    const input = ["- item", "", `  ${LONG_PROSE}`, "  ---"].join("\n");

    expect(normalizeMarkdown(input)).toBe(input);
  });

  it("@covers AC-UI-COMMENT-MARKDOWN-002.4 preserves existing rule forms and front matter", () => {
    const long = proseWithLength(70);
    const rule = "---";
    const frontMatterTitle = "title: example";
    const unchanged = [
      `${long}\n\n${rule}`,
      `${long}\n===`,
      `${long}\n***`,
      `${long}\n___`,
      `${long}\n- - -`,
      `${long}\n    ---`,
      `${long}\n\t---`,
      [rule, frontMatterTitle, long, "----"].join("\n"),
    ];

    for (const input of unchanged) expect(normalizeMarkdown(input)).toBe(input);

    const withClosedFrontMatter = [rule, frontMatterTitle, long, rule, long, rule].join("\n");
    expect(normalizeMarkdown(withClosedFrontMatter)).toBe(
      [rule, frontMatterTitle, long, rule, long, "", rule].join("\n"),
    );

    const indentedFrontMatter = [`  ${rule}`, frontMatterTitle, long, rule].join("\n");
    expect(normalizeMarkdown(indentedFrontMatter)).toBe(
      [`  ${rule}`, frontMatterTitle, long, "", rule].join("\n"),
    );

    const spacedRuleBoundary = ["- - -", long, rule].join("\n");
    expect(normalizeMarkdown(spacedRuleBoundary)).toBe(["- - -", long, "", rule].join("\n"));
  });
});

describe("normalizeMarkdown separator preservation", () => {
  it("@covers AC-UI-COMMENT-MARKDOWN-002.5 preserves line endings and is idempotent", () => {
    const long = proseWithLength(70);
    const cases = [
      [`${long}\n---\nnext`, `${long}\n\n---\nnext`],
      [`${long}\r\n---\r\nnext`, `${long}\r\n\r\n---\r\nnext`],
      [`${long}\r\n---\nnext`, `${long}\r\n\r\n---\nnext`],
      [`\n\n${long}\n---\n\n`, `\n\n${long}\n\n---\n\n`],
      [`${long}\n---`, `${long}\n\n---`],
    ] as const;

    for (const [input, expected] of cases) {
      const normalized = normalizeMarkdown(input);
      expect(normalized).toBe(expected);
      expect(normalizeMarkdown(normalized)).toBe(expected);
      __resetMarkdownCounters();
      expect(normalizeCached(input)).toBe(expected);
    }
  });

  it("@covers AC-UI-COMMENT-MARKDOWN-002.5 normalizes streaming values independently", () => {
    const long = proseWithLength(70);
    const values = [`${long}\n-`, `${long}\n--`, `${long}\n---`, `${long}\n---\nNext`];

    expect(normalizeMarkdown(values[0])).toBe(values[0]);
    expect(normalizeMarkdown(values[1])).toBe(values[1]);
    expect(normalizeMarkdown(values[2])).toBe(`${long}\n\n---`);
    expect(normalizeMarkdown(values[3])).toBe(`${long}\n\n---\nNext`);
    expect(normalizeMarkdown(values[3])).toBe(normalizeMarkdown(values[3]));
  });
});
