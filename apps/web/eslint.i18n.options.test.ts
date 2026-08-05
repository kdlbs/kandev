import { createRequire } from "node:module";

import { describe, expect, it } from "vitest";

import { noLiteralStringOptions } from "./eslint.i18n.options.mjs";

/**
 * `words.exclude` decides what `i18next/no-literal-string` is allowed to stay
 * quiet about. A pattern there that is broader than its author intended does not
 * fail anything — it just stops reporting, for the whole repo, until somebody
 * notices copy shipping untranslated.
 *
 * That is not hypothetical. The id-fragment entry used to read
 *
 *     "[a-z0-9]+(?:[-_][a-z0-9]*)*|[-_][a-z0-9]+(?:[-_][a-z0-9]*)*"
 *
 * and the plugin wraps every entry as `^…$`. `|` binds looser than the anchors,
 * so that compiled to `(^A)|(B$)` — a first branch with NO end anchor, matching
 * any string that merely *starts* with a lowercase letter or digit. And the
 * token-list entry required nothing but lowercase words separated by spaces, so
 * it swallowed any unpunctuated lowercase sentence.
 *
 * Both holes are invisible from the rule's output, which is why they lasted. The
 * corpus below is the check the output cannot give us: real copy, asserted to
 * survive every pattern.
 */

const require_ = createRequire(import.meta.url);

/** The plugin's own wrapper, imported rather than reimplemented so this test
 *  cannot drift from what ESLint actually compiles. */
const generateFullMatchRegExp = require_(
  "eslint-plugin-i18next/lib/helper/generateFullMatchRegExp",
) as (source: string) => RegExp;

const wordExcludes = noLiteralStringOptions.words.exclude;

/** Copy that MUST reach the developer as a finding. */
const COPY = [
  // The three verbatim regressions this test exists for. The first two were
  // measured SKIPPED on main; the third was flagged only because it happens to
  // start with a capital.
  "query to the sidebar for quick access later.",
  "open pull requests assigned to you",
  "Save this query",
  // The template chunks the id-fragment entry's own comment claims are still
  // flagged because they "carry a capital, a space or punctuation".
  "Select task ",
  " tasks, over WIP limit",
  // More `<Trans>` split fragments: sentence-splitting produces this shape
  // constantly, and lowercase continuations are the majority of it.
  "and reopen it later",
  "to a different workspace",
  "in the settings above",
  // Lowercase sentences with no punctuation at all — the shape the token-list
  // entry used to accept wholesale.
  "no repositories match this filter",
  "this cannot be undone",
];

/** Non-copy the exclusions exist for. Listed so tightening a pattern cannot
 *  quietly re-flag the identifiers the guard is supposed to ignore. */
const NOT_COPY = [
  // Static chunks of an interpolated DOM id (`startup-page-${v}-label`).
  "startup-page-",
  "-label",
  "task-card-",
  // Prop enum values, classnames, identifiers.
  "ghost",
  "top",
  "work-items",
  // Tailwind class lists that reach the guard as an object property rather than
  // as a `className` JSX attribute.
  "h-4 w-4",
  "flex items-center gap-2",
  "text-xs text-muted-foreground",
  // Values and identifiers.
  "owner/repo",
  "ssh-host.example.com",
  "settings-page:editors",
  "https://kandev.ai/docs",
];

function matchingPatterns(value: string): string[] {
  return wordExcludes.filter((pattern) => generateFullMatchRegExp(pattern).test(value));
}

/**
 * True when `pattern` has a `|` that is not inside a group or character class.
 *
 * Such a branch escapes the `^`/`$` the plugin adds, so one side of the
 * alternation ends up unanchored and matches far more than it reads as. Wrapping
 * the alternation in `(?:…)` is the whole fix.
 */
function hasTopLevelAlternation(pattern: string): boolean {
  let depth = 0;
  let inClass = false;

  for (let i = 0; i < pattern.length; i += 1) {
    const char = pattern[i];
    if (char === "\\") {
      i += 1;
      continue;
    }
    if (inClass) {
      if (char === "]") inClass = false;
      continue;
    }
    if (char === "[") inClass = true;
    else if (char === "(") depth += 1;
    else if (char === ")") depth -= 1;
    else if (char === "|" && depth === 0) return true;
  }

  return false;
}

describe("words.exclude", () => {
  it.each(COPY)("does not exclude %j", (copy) => {
    expect(matchingPatterns(copy)).toEqual([]);
  });

  it.each(NOT_COPY)("still excludes %j", (value) => {
    expect(matchingPatterns(value).length).toBeGreaterThan(0);
  });

  it("has no alternation outside a group", () => {
    // `^`/`$` bind tighter than `|`, so a top-level alternation leaves one
    // branch unanchored and silently matching prefixes or suffixes.
    const unanchored = wordExcludes.filter(hasTopLevelAlternation);
    expect(unanchored).toEqual([]);
  });

  it("compiles every pattern", () => {
    for (const pattern of wordExcludes) {
      expect(() => generateFullMatchRegExp(pattern)).not.toThrow();
    }
  });
});

describe("hasTopLevelAlternation", () => {
  it("finds the bug this test was written for", () => {
    expect(hasTopLevelAlternation("[a-z0-9]+(?:[-_][a-z0-9]*)*|[-_][a-z0-9]+")).toBe(true);
  });

  it("accepts the grouped form", () => {
    expect(hasTopLevelAlternation("(?:[a-z0-9]+(?:[-_][a-z0-9]*)*|[-_][a-z0-9]+)")).toBe(false);
  });

  it("ignores a pipe inside a character class or escaped", () => {
    expect(hasTopLevelAlternation("^[·+\\-|/(),.:\\s]+$")).toBe(false);
    expect(hasTopLevelAlternation("a\\|b")).toBe(false);
  });
});

describe("jsx-attributes.exclude", () => {
  const attributeExcludes = noLiteralStringOptions["jsx-attributes"].exclude;

  function excludesAttribute(name: string): boolean {
    return attributeExcludes.some((pattern) => generateFullMatchRegExp(pattern).test(name));
  }

  it.each(["titleKey", "descriptionKey", "labelKey", "i18nKey", "key"])(
    "excludes the catalog-key prop %j",
    (name) => {
      expect(excludesAttribute(name)).toBe(true);
    },
  );

  it.each(["title", "placeholder", "aria-label", "label", "description", "tooltip"])(
    "still checks the display prop %j",
    (name) => {
      expect(excludesAttribute(name)).toBe(false);
    },
  );
});
