/* eslint-disable max-lines */
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import ReactMarkdown from "react-markdown";
import remarkBreaks from "remark-breaks";
import remarkGemoji from "remark-gemoji";
import remarkGfm from "remark-gfm";
import { beforeEach, describe, expect, it } from "vitest";
import {
  __MAX_CACHE_ENTRIES,
  __lruSize,
  __markdownParseCount,
  __resetMarkdownCounters,
  normalizeCached,
  normalizeMarkdown,
} from "./normalize-cache";

const MARKDOWN_FENCE = "```markdown";
const STRENGTHENED_MARKDOWN_FENCE = "````markdown";
const INTRO_TEXT = "Intro:";
const INNER_PROMPT_TEXT = "nested prompt";
const AFTER_NESTED_PROMPT = "After nested prompt.";
const SAMPLE_PROSE_TEXT = "some explanation";
const INTRO_AND_RULE_TEXT = "Intro and rule";
const SELF_CONTAINED_TEXT = "That is self-contained.";
const FIRST_FUNCTION_TEXT = "func first() {}";
const SECOND_FUNCTION_TEXT = "func second() {}";
const BARE_WRAPPED_DOCUMENT = [
  INTRO_AND_RULE_TEXT,
  "",
  "---",
  "",
  "```",
  "Open a pull request.",
  "## Why",
  "```go",
  FIRST_FUNCTION_TEXT,
  "```",
  "More prose.",
  "```go",
  SECOND_FUNCTION_TEXT,
  "```",
  "Final prose.",
  "```",
  "",
  "---",
  "",
  SELF_CONTAINED_TEXT,
].join("\n");
const INDEPENDENT_TAGGED_BLOCKS = [
  "```go",
  FIRST_FUNCTION_TEXT,
  "```",
  "This paragraph separates two examples.",
  "```go",
  SECOND_FUNCTION_TEXT,
  "```",
].join("\n");
const RULE_DELIMITED_BARE_BLOCK_WITHOUT_NESTED_FENCES = [
  INTRO_AND_RULE_TEXT,
  "",
  "---",
  "",
  "```",
  "plain content",
  "```",
  "",
  "---",
  "",
  SELF_CONTAINED_TEXT,
].join("\n");
const RULE_BOUNDED_INDEPENDENT_BARE_BLOCKS = [
  "---",
  "```",
  "## Fence syntax",
  "```go",
  FIRST_FUNCTION_TEXT,
  "```",
  "This paragraph separates two examples.",
  "```",
  SECOND_FUNCTION_TEXT,
  "```",
  "---",
].join("\n");
const FIVE_BACKTICK_LITERAL_DOCUMENT = ["`````text", BARE_WRAPPED_DOCUMENT, "`````"].join("\n");
const TWO_BARE_WRAPPED_DOCUMENTS = [BARE_WRAPPED_DOCUMENT, BARE_WRAPPED_DOCUMENT].join("\n\n");

function renderNormalizedMarkdown(input: string): string {
  return renderToStaticMarkup(
    createElement(
      ReactMarkdown,
      { remarkPlugins: [remarkGfm, remarkBreaks, remarkGemoji] },
      normalizeMarkdown(input),
    ),
  );
}

function countOccurrences(input: string, value: string): number {
  return input.split(value).length - 1;
}

describe("normalizeMarkdown", () => {
  it("leaves a markdown wrapper without nested fences unchanged", () => {
    const input = [MARKDOWN_FENCE, "# Title", "```"].join("\n");

    expect(normalizeMarkdown(input)).toBe(input);
  });

  it("strengthens a markdown wrapper that contains nested code fences", () => {
    const input = [
      MARKDOWN_FENCE,
      INTRO_TEXT,
      "",
      "```text",
      INNER_PROMPT_TEXT,
      "```",
      "",
      AFTER_NESTED_PROMPT,
      "```",
    ].join("\n");
    const expected = [
      STRENGTHENED_MARKDOWN_FENCE,
      INTRO_TEXT,
      "",
      "```text",
      INNER_PROMPT_TEXT,
      "```",
      "",
      AFTER_NESTED_PROMPT,
      "````",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(expected);
  });

  it("leaves an ambiguous bare document between rules unchanged", () => {
    expect(normalizeMarkdown(BARE_WRAPPED_DOCUMENT)).toBe(BARE_WRAPPED_DOCUMENT);
  });

  it("renders an ambiguous bare document according to Markdown boundaries", () => {
    const rendered = renderNormalizedMarkdown(BARE_WRAPPED_DOCUMENT);

    expect(countOccurrences(rendered, "<pre>")).toBe(3);
    expect(countOccurrences(rendered, "<code")).toBe(3);
    expect(rendered).toContain("func first() {}");
    expect(rendered).toContain("func second() {}");
    expect(rendered).toContain("## Why");
    expect(rendered).toContain("```go");
    expect(rendered).toContain("Intro and rule");
    expect(rendered).toContain("That is self-contained.");
  });
});

describe("normalizeMarkdown bare-wrapper contract", () => {
  it("preserves independent tagged blocks without rule boundaries", () => {
    expect(normalizeMarkdown(INDEPENDENT_TAGGED_BLOCKS)).toBe(INDEPENDENT_TAGGED_BLOCKS);
  });

  it("renders independent tagged blocks as two code blocks", () => {
    const rendered = renderNormalizedMarkdown(INDEPENDENT_TAGGED_BLOCKS);

    expect(countOccurrences(rendered, "<pre>")).toBe(2);
    expect(rendered).toContain("This paragraph separates two examples.");
  });

  it("preserves independent bare blocks inside rule boundaries", () => {
    expect(normalizeMarkdown(RULE_BOUNDED_INDEPENDENT_BARE_BLOCKS)).toBe(
      RULE_BOUNDED_INDEPENDENT_BARE_BLOCKS,
    );
  });

  it("preserves a valid five-backtick code block containing bare wrapper syntax", () => {
    expect(normalizeMarkdown(FIVE_BACKTICK_LITERAL_DOCUMENT)).toBe(FIVE_BACKTICK_LITERAL_DOCUMENT);
  });

  it("renders a valid five-backtick code block as literal text", () => {
    const rendered = renderNormalizedMarkdown(FIVE_BACKTICK_LITERAL_DOCUMENT);

    expect(countOccurrences(rendered, "<pre>")).toBe(1);
    expect(countOccurrences(rendered, "<code")).toBe(1);
    expect(rendered).toContain("## Why");
    expect(rendered).toContain("```go");
    expect(rendered).toContain("---");
  });

  it("preserves and remains idempotent for two rule-delimited bare regions", () => {
    const normalized = normalizeMarkdown(TWO_BARE_WRAPPED_DOCUMENTS);

    expect(normalized).toBe(TWO_BARE_WRAPPED_DOCUMENTS);
    expect(normalizeMarkdown(normalized)).toBe(TWO_BARE_WRAPPED_DOCUMENTS);
  });

  it("leaves a rule-delimited bare block without nested fences unchanged", () => {
    expect(normalizeMarkdown(RULE_DELIMITED_BARE_BLOCK_WITHOUT_NESTED_FENCES)).toBe(
      RULE_DELIMITED_BARE_BLOCK_WITHOUT_NESTED_FENCES,
    );
  });

  it("preserves CRLF line endings in an unchanged bare wrapper", () => {
    const input = BARE_WRAPPED_DOCUMENT.replaceAll("\n", "\r\n");

    expect(normalizeMarkdown(input)).toBe(input);
  });

  it("preserves an ambiguous glued outer close while retaining CRLF line endings", () => {
    const inputLines = BARE_WRAPPED_DOCUMENT.replaceAll("\n", "\r\n").split("\n");
    inputLines.splice(14, 2, "Final prose.```\r");
    const input = inputLines.join("\n");

    expect(normalizeMarkdown(input)).toBe(input);
  });

  it("preserves leading and trailing blank lines", () => {
    const input = `\n\n${BARE_WRAPPED_DOCUMENT}\n\n`;

    expect(normalizeMarkdown(input)).toBe(input);
  });

  it("preserves three-space indentation on an unchanged bare wrapper", () => {
    const input = BARE_WRAPPED_DOCUMENT.split("\n")
      .map((line, index) => ([2, 4, 15, 17].includes(index) ? `   ${line}` : line))
      .join("\n");

    expect(normalizeMarkdown(input)).toBe(input);
  });

  it("preserves longer nested fences in an unchanged bare wrapper", () => {
    const input = BARE_WRAPPED_DOCUMENT.split("\n");
    input[7] = "````go";
    input[9] = "````";
    input[11] = "````go";
    input[13] = "````";

    expect(normalizeMarkdown(input.join("\n"))).toBe(input.join("\n"));
  });

  it("leaves a rule-delimited wrapper with no outer close unchanged", () => {
    const input = BARE_WRAPPED_DOCUMENT.split("\n");
    input.splice(15, 1);

    expect(normalizeMarkdown(input.join("\n"))).toBe(input.join("\n"));
  });

  it("is idempotent for preserved bare-wrapper inputs", () => {
    for (const input of [
      BARE_WRAPPED_DOCUMENT,
      INDEPENDENT_TAGGED_BLOCKS,
      RULE_DELIMITED_BARE_BLOCK_WITHOUT_NESTED_FENCES,
    ]) {
      const normalized = normalizeMarkdown(input);
      expect(normalizeMarkdown(normalized)).toBe(normalized);
    }
  });
});

describe("normalizeMarkdown wrapper edge cases", () => {
  it("strengthens a markdown wrapper that contains multiple nested code fences", () => {
    const input = [
      MARKDOWN_FENCE,
      INTRO_TEXT,
      "",
      "```text",
      INNER_PROMPT_TEXT,
      "```",
      "",
      "```json",
      '{"ok": true}',
      "```",
      "",
      AFTER_NESTED_PROMPT,
      "```",
    ].join("\n");
    const expected = [
      STRENGTHENED_MARKDOWN_FENCE,
      INTRO_TEXT,
      "",
      "```text",
      INNER_PROMPT_TEXT,
      "```",
      "",
      "```json",
      '{"ok": true}',
      "```",
      "",
      AFTER_NESTED_PROMPT,
      "````",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(expected);
  });

  it("strengthens a markdown wrapper that contains spaced nested info strings", () => {
    const input = [
      MARKDOWN_FENCE,
      INTRO_TEXT,
      "",
      "``` text",
      INNER_PROMPT_TEXT,
      "```",
      "",
      AFTER_NESTED_PROMPT,
      "```",
    ].join("\n");
    const expected = [
      STRENGTHENED_MARKDOWN_FENCE,
      INTRO_TEXT,
      "",
      "``` text",
      INNER_PROMPT_TEXT,
      "```",
      "",
      AFTER_NESTED_PROMPT,
      "````",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(expected);
  });
});

describe("normalizeMarkdown untagged wrapper edge cases", () => {
  it("strengthens a markdown wrapper that contains untagged nested code fences", () => {
    const input = [
      MARKDOWN_FENCE,
      INTRO_TEXT,
      "",
      "```",
      INNER_PROMPT_TEXT,
      "```",
      "",
      AFTER_NESTED_PROMPT,
      "```",
    ].join("\n");
    const expected = [
      STRENGTHENED_MARKDOWN_FENCE,
      INTRO_TEXT,
      "",
      "```",
      INNER_PROMPT_TEXT,
      "```",
      "",
      AFTER_NESTED_PROMPT,
      "````",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(expected);
  });

  it("preserves adjacent untagged blocks after a markdown sample", () => {
    const input = [MARKDOWN_FENCE, "# Title", "```", SAMPLE_PROSE_TEXT, "```", "code", "```"].join(
      "\n",
    );

    expect(normalizeMarkdown(input)).toBe(input);
  });

  it("strengthens same-length untagged fences after prose inside wrappers", () => {
    const input = [
      MARKDOWN_FENCE,
      INTRO_TEXT,
      "```",
      "",
      INNER_PROMPT_TEXT,
      "```",
      AFTER_NESTED_PROMPT,
      "```",
    ].join("\n");
    const expected = [
      STRENGTHENED_MARKDOWN_FENCE,
      INTRO_TEXT,
      "```",
      "",
      INNER_PROMPT_TEXT,
      "```",
      AFTER_NESTED_PROMPT,
      "````",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(expected);
  });
});

describe("normalizeMarkdown advanced untagged wrapper edge cases", () => {
  it("preserves markdown samples whose close follows a blank line", () => {
    const input = [
      MARKDOWN_FENCE,
      "# Title",
      "",
      "```",
      SAMPLE_PROSE_TEXT,
      "```",
      "code",
      "```",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(input);
  });

  it("preserves non-heading markdown samples before separate untagged blocks", () => {
    const input = [
      MARKDOWN_FENCE,
      "plain text",
      "```",
      SAMPLE_PROSE_TEXT,
      "```",
      "code",
      "```",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(input);
  });

  it("preserves separate untagged blocks that start blank", () => {
    const input = [
      MARKDOWN_FENCE,
      "plain text",
      "```",
      SAMPLE_PROSE_TEXT,
      "```",
      "",
      "code",
      "```",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(input);
  });

  it("preserves markdown samples before separate tagged blocks", () => {
    const input = [
      MARKDOWN_FENCE,
      "plain text",
      "```",
      SAMPLE_PROSE_TEXT,
      "```js",
      "code",
      "```",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(input);
  });
});

describe("normalizeMarkdown blank-padded untagged wrapper edge cases", () => {
  it("strengthens same-length bare fences after headings inside wrappers", () => {
    const input = [
      MARKDOWN_FENCE,
      "# Prompt",
      "```",
      "code",
      "```",
      AFTER_NESTED_PROMPT,
      "```",
    ].join("\n");
    const expected = [
      STRENGTHENED_MARKDOWN_FENCE,
      "# Prompt",
      "```",
      "code",
      "```",
      AFTER_NESTED_PROMPT,
      "````",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(expected);
  });

  it("strengthens same-length bare fences after non-colon prose inside wrappers", () => {
    const input = [
      MARKDOWN_FENCE,
      "Here is an example",
      "```",
      "code",
      "```",
      AFTER_NESTED_PROMPT,
      "```",
    ].join("\n");
    const expected = [
      STRENGTHENED_MARKDOWN_FENCE,
      "Here is an example",
      "```",
      "code",
      "```",
      AFTER_NESTED_PROMPT,
      "````",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(expected);
  });

  it("strengthens blank-padded bare nested fences", () => {
    const input = [
      MARKDOWN_FENCE,
      INTRO_TEXT,
      "```",
      "",
      "code",
      "",
      "```",
      AFTER_NESTED_PROMPT,
      "```",
    ].join("\n");
    const expected = [
      STRENGTHENED_MARKDOWN_FENCE,
      INTRO_TEXT,
      "```",
      "",
      "code",
      "",
      "```",
      AFTER_NESTED_PROMPT,
      "````",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(expected);
  });

  it("strengthens heading samples inside spaced untagged nested fences", () => {
    const input = [
      MARKDOWN_FENCE,
      INTRO_TEXT,
      "",
      "```",
      "# Title",
      "",
      "```",
      "",
      AFTER_NESTED_PROMPT,
      "```",
    ].join("\n");
    const expected = [
      STRENGTHENED_MARKDOWN_FENCE,
      INTRO_TEXT,
      "",
      "```",
      "# Title",
      "",
      "```",
      "",
      AFTER_NESTED_PROMPT,
      "````",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(expected);
  });
});

describe("normalizeMarkdown tagged-looking untagged wrapper edge cases", () => {
  it("strengthens tagged nested fences after headings inside wrappers", () => {
    const input = [
      MARKDOWN_FENCE,
      "# Prompt",
      "```text",
      INNER_PROMPT_TEXT,
      "```",
      AFTER_NESTED_PROMPT,
      "```",
    ].join("\n");
    const expected = [
      STRENGTHENED_MARKDOWN_FENCE,
      "# Prompt",
      "```text",
      INNER_PROMPT_TEXT,
      "```",
      AFTER_NESTED_PROMPT,
      "````",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(expected);
  });

  it("strengthens bare fences that contain tagged-looking content", () => {
    const input = [
      MARKDOWN_FENCE,
      INTRO_TEXT,
      "```",
      "````text",
      INNER_PROMPT_TEXT,
      "```",
      AFTER_NESTED_PROMPT,
      "```",
    ].join("\n");
    const expected = [
      STRENGTHENED_MARKDOWN_FENCE,
      INTRO_TEXT,
      "```",
      "````text",
      INNER_PROMPT_TEXT,
      "```",
      AFTER_NESTED_PROMPT,
      "````",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(expected);
  });

  it("keeps scanning unmatched tagged-looking content after a nested pair", () => {
    const input = [
      MARKDOWN_FENCE,
      INTRO_TEXT,
      "",
      "```text",
      INNER_PROMPT_TEXT,
      "```",
      "",
      "```not-a-block",
      AFTER_NESTED_PROMPT,
      "```",
    ].join("\n");
    const expected = [
      STRENGTHENED_MARKDOWN_FENCE,
      INTRO_TEXT,
      "",
      "```text",
      INNER_PROMPT_TEXT,
      "```",
      "",
      "```not-a-block",
      AFTER_NESTED_PROMPT,
      "````",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(expected);
  });

  it("keeps scanning unmatched bare fence content after a nested pair", () => {
    const input = [
      MARKDOWN_FENCE,
      INTRO_TEXT,
      "",
      "```text",
      INNER_PROMPT_TEXT,
      "```",
      "",
      "```",
      AFTER_NESTED_PROMPT,
      "```",
    ].join("\n");
    const expected = [
      STRENGTHENED_MARKDOWN_FENCE,
      INTRO_TEXT,
      "",
      "```text",
      INNER_PROMPT_TEXT,
      "```",
      "",
      "```",
      AFTER_NESTED_PROMPT,
      "````",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(expected);
  });
});

describe("normalizeMarkdown longer-close wrapper edge cases", () => {
  it("strengthens a longer wrapper when shorter tagged fences use longer closes", () => {
    const input = [
      STRENGTHENED_MARKDOWN_FENCE,
      INTRO_TEXT,
      "```text",
      INNER_PROMPT_TEXT,
      "````",
      AFTER_NESTED_PROMPT,
      "````",
    ].join("\n");
    const expected = [
      "`````markdown",
      INTRO_TEXT,
      "```text",
      INNER_PROMPT_TEXT,
      "````",
      AFTER_NESTED_PROMPT,
      "`````",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(expected);
  });

  it("strengthens a longer wrapper when shorter untagged fences use longer closes", () => {
    const input = [
      STRENGTHENED_MARKDOWN_FENCE,
      INTRO_TEXT,
      "```",
      INNER_PROMPT_TEXT,
      "````",
      AFTER_NESTED_PROMPT,
      "````",
    ].join("\n");
    const expected = [
      "`````markdown",
      INTRO_TEXT,
      "```",
      INNER_PROMPT_TEXT,
      "````",
      AFTER_NESTED_PROMPT,
      "`````",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(expected);
  });
});

describe("normalizeMarkdown long untagged wrapper edge cases", () => {
  it("strengthens a markdown wrapper when longer untagged fences contain shorter examples", () => {
    const input = [
      MARKDOWN_FENCE,
      INTRO_TEXT,
      "````",
      "```text",
      INNER_PROMPT_TEXT,
      "```",
      "````",
      AFTER_NESTED_PROMPT,
      "```",
    ].join("\n");
    const expected = [
      "`````markdown",
      INTRO_TEXT,
      "````",
      "```text",
      INNER_PROMPT_TEXT,
      "```",
      "````",
      AFTER_NESTED_PROMPT,
      "`````",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(expected);
  });

  it("strengthens a markdown wrapper when nested fences use glued closes", () => {
    const input = [
      MARKDOWN_FENCE,
      INTRO_TEXT,
      "",
      "```text",
      `${INNER_PROMPT_TEXT}\`\`\``,
      "",
      AFTER_NESTED_PROMPT,
      "```",
    ].join("\n");
    const expected = [
      STRENGTHENED_MARKDOWN_FENCE,
      INTRO_TEXT,
      "",
      "```text",
      `${INNER_PROMPT_TEXT}\`\`\``,
      "",
      AFTER_NESTED_PROMPT,
      "````",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(expected);
  });
});

describe("normalizeMarkdown wrapper boundaries", () => {
  it("strengthens markdown wrappers after leading blank lines", () => {
    const input = [
      "",
      MARKDOWN_FENCE,
      INTRO_TEXT,
      "",
      "```text",
      INNER_PROMPT_TEXT,
      "```",
      "",
      AFTER_NESTED_PROMPT,
      "```",
    ].join("\n");
    const expected = [
      "",
      STRENGTHENED_MARKDOWN_FENCE,
      INTRO_TEXT,
      "",
      "```text",
      INNER_PROMPT_TEXT,
      "```",
      "",
      AFTER_NESTED_PROMPT,
      "````",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(expected);
  });

  it("strengthens CRLF markdown wrappers that contain nested code fences", () => {
    const input = [
      MARKDOWN_FENCE,
      INTRO_TEXT,
      "",
      "```text",
      INNER_PROMPT_TEXT,
      "```",
      "",
      AFTER_NESTED_PROMPT,
      "```",
    ].join("\r\n");
    const expected = [
      "````markdown\r",
      `${INTRO_TEXT}\r`,
      "\r",
      "```text\r",
      `${INNER_PROMPT_TEXT}\r`,
      "```\r",
      "\r",
      `${AFTER_NESTED_PROMPT}\r`,
      "````",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(expected);
  });

  it("strengthens markdown wrappers with glued closing fences", () => {
    const input = [
      MARKDOWN_FENCE,
      INTRO_TEXT,
      "",
      "```text",
      INNER_PROMPT_TEXT,
      "```",
      "",
      `${AFTER_NESTED_PROMPT}\`\`\``,
    ].join("\n");
    const expected = [
      STRENGTHENED_MARKDOWN_FENCE,
      INTRO_TEXT,
      "",
      "```text",
      INNER_PROMPT_TEXT,
      "```",
      "",
      AFTER_NESTED_PROMPT,
      "````",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(expected);
  });
});

describe("normalizeMarkdown wrapper unchanged boundaries", () => {
  it("leaves a valid markdown sample followed by another fenced block unchanged", () => {
    const input = [
      MARKDOWN_FENCE,
      "# Title",
      "```",
      "",
      "prose between blocks",
      "",
      "```js",
      "const value = 1;",
      "```",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(input);
  });

  it("leaves a markdown sample with a literal tagged fence followed by another block unchanged", () => {
    const input = [
      MARKDOWN_FENCE,
      "# Title",
      "",
      "```text",
      "literal nested sample",
      "```",
      "```",
      "",
      "prose between blocks",
      "",
      "```js",
      "const value = 1;",
      "```",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(input);
  });

  it("leaves a valid markdown sample followed by an untagged fenced block unchanged", () => {
    const input = [
      MARKDOWN_FENCE,
      "# Title",
      "```",
      "",
      "prose between blocks",
      "",
      "```",
      "const value = 1;",
      "```",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(input);
  });

  it("leaves markdown wrappers with trailing prose outside the wrapper unchanged", () => {
    const input = [
      MARKDOWN_FENCE,
      INTRO_TEXT,
      "",
      "```text",
      INNER_PROMPT_TEXT,
      "```",
      "```",
      "",
      "Trailing prose outside the wrapper.",
    ].join("\n");

    expect(normalizeMarkdown(input)).toBe(input);
  });
});

describe("normalizeCached", () => {
  beforeEach(() => {
    __resetMarkdownCounters();
  });

  it("parses once for repeated identical input (cache hit)", () => {
    const input = "```go\nfunc f() {}```\nprose";
    const first = normalizeCached(input);
    const second = normalizeCached(input);
    expect(second).toBe(first);
    expect(__markdownParseCount()).toBe(1);
  });

  it("parses again for different input", () => {
    normalizeCached("alpha");
    normalizeCached("beta");
    expect(__markdownParseCount()).toBe(2);
  });

  it("produces output byte-identical to normalizeMarkdown", () => {
    const inputs = [
      "```go\nfunc f() {}```\nprose",
      "Use `code` inline.",
      "",
      "single line",
      "```go\nx```\nprose\n",
    ];
    for (const input of inputs) {
      expect(normalizeCached(input)).toBe(normalizeMarkdown(input));
    }
  });

  it("evicts oldest entries past the cap (bounded LRU)", () => {
    for (let i = 0; i < __MAX_CACHE_ENTRIES + 50; i++) {
      normalizeCached(`unique-content-${i}`);
    }
    expect(__lruSize()).toBeLessThanOrEqual(__MAX_CACHE_ENTRIES);
  });

  it("keeps a recently-used entry warm despite overflow", () => {
    normalizeCached("keep-me");
    for (let i = 0; i < __MAX_CACHE_ENTRIES; i++) {
      normalizeCached(`filler-${i}`);
      normalizeCached("keep-me"); // refresh recency each round
    }
    const before = __markdownParseCount();
    normalizeCached("keep-me");
    expect(__markdownParseCount()).toBe(before); // still cached, no new parse
  });

  it("__resetMarkdownCounters clears cache and counter", () => {
    normalizeCached("something");
    __resetMarkdownCounters();
    expect(__markdownParseCount()).toBe(0);
    expect(__lruSize()).toBe(0);
  });
});
