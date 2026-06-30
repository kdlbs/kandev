/* eslint-disable max-lines */
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
    const input = [MARKDOWN_FENCE, "# Title", "```", "prose", "```", "code", "```"].join("\n");

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
    const input = [MARKDOWN_FENCE, "# Title", "", "```", "prose", "```", "code", "```"].join("\n");

    expect(normalizeMarkdown(input)).toBe(input);
  });

  it("preserves non-heading markdown samples before separate untagged blocks", () => {
    const input = [MARKDOWN_FENCE, "plain text", "```", "prose", "```", "code", "```"].join("\n");

    expect(normalizeMarkdown(input)).toBe(input);
  });

  it("preserves separate untagged blocks that start blank", () => {
    const input = [MARKDOWN_FENCE, "plain text", "```", "prose", "```", "", "code", "```"].join(
      "\n",
    );

    expect(normalizeMarkdown(input)).toBe(input);
  });
});

describe("normalizeMarkdown blank-padded untagged wrapper edge cases", () => {
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
