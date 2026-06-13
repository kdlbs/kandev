import { beforeEach, describe, expect, it } from "vitest";
import {
  __MAX_CACHE_ENTRIES,
  __lruSize,
  __markdownParseCount,
  __resetMarkdownCounters,
  normalizeCached,
  normalizeMarkdown,
} from "./normalize-cache";

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
