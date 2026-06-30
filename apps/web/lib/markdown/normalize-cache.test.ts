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
      "Intro:",
      "",
      "```text",
      INNER_PROMPT_TEXT,
      "```",
      "",
      AFTER_NESTED_PROMPT,
      "```",
    ].join("\n");
    const expected = [
      "````markdown",
      "Intro:",
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
      "Intro:",
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
      "Intro:\r",
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

  it("leaves markdown wrappers with trailing prose outside the wrapper unchanged", () => {
    const input = [
      MARKDOWN_FENCE,
      "Intro:",
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
