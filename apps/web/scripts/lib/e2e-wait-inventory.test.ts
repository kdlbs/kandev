import fs from "node:fs";
import path from "node:path";

import { describe, expect, it } from "vitest";

import { inventorySource, readDwellCategories, summarize } from "./e2e-wait-inventory.mjs";

const CAUSAL_WAITS = path.resolve(import.meta.dirname, "../../e2e/helpers/causal-waits.ts");
const CATEGORIES = readDwellCategories(fs.readFileSync(CAUSAL_WAITS, "utf8")) as string[];

/** Extracted because it is the one category that is permanent rather than debt. */
const NEGATIVE = "negative-assertion";

const inventory = (code: string) => inventorySource(code, "fixture.ts", CATEGORIES);

describe("readDwellCategories", () => {
  /**
   * Read from the helper rather than restated here. If `causal-waits.ts` grows a
   * category, this picks it up; if the export is renamed or reshaped, the parse
   * throws instead of silently reporting every dwell as uncategorised.
   */
  it("reads the live category list out of causal-waits.ts", () => {
    expect(CATEGORIES).toEqual([
      NEGATIVE,
      "product-timer",
      "library-timer",
      "clock-separation",
      "poll-interval",
      "browser-chrome",
      "unverified",
    ]);
  });

  it("throws rather than guessing when the export cannot be found", () => {
    expect(() => readDwellCategories("export const SOMETHING_ELSE = [];")).toThrow(
      /could not find/i,
    );
  });

  it("throws when the list parses to nothing", () => {
    expect(() => readDwellCategories("export const DWELL_CATEGORIES = [] as const;")).toThrow(
      /empty list/i,
    );
  });
});

describe("inventorySource", () => {
  /**
   * The two overloads put `category` at a different index — 1 without a page, 2
   * with one. Both must land in the same bucket, or the report splits one
   * category across two rows depending on which form the author used.
   */
  it("reads the category from the page-less form", () => {
    const found = inventory('await dwell(500, "poll-interval", "backend health poll");');
    expect(found.dwells).toEqual([{ category: "poll-interval", line: 1 }]);
  });

  it("reads the category from the page form", () => {
    const found = inventory('await dwell(page, 300, "library-timer", "Radix open delay");');
    expect(found.dwells).toEqual([{ category: "library-timer", line: 1 }]);
  });

  it("counts every category separately across a file", () => {
    const found = inventory(`
      await dwell(page, 300, "${NEGATIVE}", "asserting the toast never appears");
      await dwell(100, "unverified", "pre-existing spacing, no timer identified");
      await dwell(page, 50, "clock-separation", "keep the two writes distinguishable");
    `);
    expect(found.dwells.map((entry: { category: string }) => entry.category)).toEqual([
      NEGATIVE,
      "unverified",
      "clock-separation",
    ]);
  });

  it("flags a category that is not in the live list rather than dropping it", () => {
    const found = inventory('await dwell(500, "made-up-category", "why");');
    expect(found.dwells[0].category).toBe("«unknown: made-up-category»");
  });

  it("flags a dwell whose category is not a literal", () => {
    const found = inventory("await dwell(500, category, reason);");
    expect(found.dwells[0].category).toBe("«no literal category»");
  });

  it("counts injectLatency apart from dwell", () => {
    const found = inventory(`
      await injectLatency(800, "make the spinner observable");
      await dwell(100, "unverified", "no timer identified");
    `);
    expect(found.injectLatency).toBe(1);
    expect(found.dwells).toHaveLength(1);
  });

  it("parses TypeScript syntax without a type checker", () => {
    const found = inventory(`
      const wait = async (page: Page): Promise<void> => {
        await dwell(page, 10, "browser-chrome", "native print dialog");
      };
    `);
    expect(found.dwells).toHaveLength(1);
  });
});

describe("summarize", () => {
  const build = (categories: string[], injectLatency = 0) => [
    {
      filePath: "fixture.ts",
      dwells: categories.map((category) => ({ category, line: 1 })),
      injectLatency,
    },
  ];

  /**
   * The load-bearing assertion of the whole report: `negative-assertion` is
   * permanent and must never appear in the number a conversion effort is trying
   * to reduce. A test asserting something never happens has no event to wait
   * for, and deleting its wait leaves a test that passes for free.
   */
  it("excludes negative-assertion from debt", () => {
    const summary = summarize(build([NEGATIVE, NEGATIVE]), CATEGORIES);
    expect(summary.totalDwells).toBe(2);
    expect(summary.permanent).toBe(2);
    expect(summary.debt).toBe(0);
  });

  it("counts every other category as debt", () => {
    const summary = summarize(
      build([NEGATIVE, "unverified", "library-timer", "poll-interval"]),
      CATEGORIES,
    );
    expect(summary.totalDwells).toBe(4);
    expect(summary.permanent).toBe(1);
    expect(summary.debt).toBe(3);
    expect(summary.unverified).toBe(1);
  });

  /**
   * injectLatency is deliberate fixture configuration, correct by construction.
   * Folding it in would make adding a legitimate latency simulation read as a
   * regression in a number that is supposed to be shrinking.
   */
  it("keeps injectLatency out of both dwell totals", () => {
    const summary = summarize(build(["unverified"], 5), CATEGORIES);
    expect(summary.injectLatency).toBe(5);
    expect(summary.totalDwells).toBe(1);
    expect(summary.debt).toBe(1);
  });

  it("surfaces uncategorised dwells instead of silently dropping them", () => {
    const summary = summarize(build(["«unknown: nope»", "unverified"]), CATEGORIES);
    expect(summary.unknown).toBe(1);
    expect(summary.totalDwells).toBe(2);
    expect(summary.debt).toBe(2);
  });

  it("reports zeros for a suite with no dwells at all", () => {
    const summary = summarize([], CATEGORIES);
    expect(summary.totalDwells).toBe(0);
    expect(summary.debt).toBe(0);
    expect(summary.byCategory.unverified).toBe(0);
  });
});
