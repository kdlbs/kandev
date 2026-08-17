/**
 * Inventory of the *sanctioned* waits in `apps/web/e2e` — the debt report's
 * data layer.
 *
 * Counting raw sleeps is not this module's job: those come from the eslint rule
 * (`eslint-rules/no-unsanctioned-sleep.mjs`), so there is exactly one definition
 * of what a banned sleep is. This module only reads the two legal forms, which
 * the rule never sees.
 *
 * ## What the numbers mean
 *
 * `dwell` carries a category, and the categories are not interchangeable:
 *
 *   - **`negative-assertion` is permanent and is NOT debt.** A test asserting
 *     that something never happens has no event to wait for. "Fixing" one by
 *     deleting the wait removes the window in which the regression could occur
 *     and leaves a test that passes for free. It is reported separately so
 *     nobody optimises it toward zero.
 *   - **`unverified` is the debt to drive to zero.** It means the author could
 *     not name the timer. Every other category names one.
 *   - The five named-timer categories are compromises of varying fixability,
 *     reported so a conversion effort can pick targets.
 *
 * `injectLatency` is counted entirely separately. It delays the system under
 * test on purpose — the delay is the stimulus, and removing it destroys the
 * scenario. Folding it into a sleep total would let deliberate fixture
 * configuration inflate a number that is supposed to be shrinking.
 */
// `typescript-eslint`, not `@typescript-eslint/typescript-estree` directly:
// pnpm's strict layout does not hoist the transitive package, so importing it
// fails with ERR_MODULE_NOT_FOUND. The umbrella package is a declared dependency
// and re-exports the same parser.
import tseslint from "typescript-eslint";

/**
 * The category list, read out of `causal-waits.ts` itself rather than restated.
 *
 * A restated copy drifts silently: a category added to the helper would be
 * counted as unknown here, and the report would under-count the very thing it
 * exists to track. Throws rather than falling back to a hardcoded list, because
 * a silently-empty category set makes every dwell read as uncategorised.
 */
export function readDwellCategories(source) {
  const match = /export const DWELL_CATEGORIES = \[([\s\S]*?)\] as const;/.exec(source);
  if (!match) {
    throw new Error(
      "e2e-wait-inventory: could not find `export const DWELL_CATEGORIES = [...] as const;` in " +
        "causal-waits.ts. The report derives categories from the helper so the two cannot drift; " +
        "update this parser rather than hardcoding a list.",
    );
  }
  const categories = [...match[1].matchAll(/"([a-z-]+)"/g)].map((entry) => entry[1]);
  if (categories.length === 0) {
    throw new Error("e2e-wait-inventory: DWELL_CATEGORIES parsed to an empty list");
  }
  return categories;
}

function walk(node, visit) {
  if (!node || typeof node.type !== "string") return;
  visit(node);
  for (const key of Object.keys(node)) {
    if (key === "parent") continue;
    const value = node[key];
    if (Array.isArray(value)) {
      for (const child of value) walk(child, visit);
    } else if (value && typeof value === "object") {
      walk(value, visit);
    }
  }
}

function calleeName(node) {
  return node.callee?.type === "Identifier" ? node.callee.name : null;
}

/**
 * The category argument of a `dwell()` call.
 *
 * `dwell(ms, category, reason)` and `dwell(page, ms, category, reason)` are
 * overloads on one name, so the category sits at index 1 or 2 depending on the
 * form. It is always the FIRST string-literal argument either way — `ms` is
 * numeric and `page` is an identifier — and `reason` is the second. Matching by
 * position would need the two forms told apart; matching the first string does
 * not, and `dwell` validates the value at runtime against the same list, so an
 * unrecognised one here is a real miscategorisation rather than a parse gap.
 */
function dwellCategory(node, categories) {
  const strings = node.arguments.filter(
    (argument) => argument.type === "Literal" && typeof argument.value === "string",
  );
  const category = strings[0]?.value;
  if (category === undefined) return "«no literal category»";
  return categories.includes(category) ? category : `«unknown: ${category}»`;
}

/**
 * Sanctioned waits in one file.
 *
 * @returns {{ dwells: Array<{category: string, line: number}>, injectLatency: number }}
 */
export function inventorySource(source, filePath, categories) {
  const { ast } = tseslint.parser.parseForESLint(source, { loc: true, range: true });
  const dwells = [];
  let injectLatency = 0;

  walk(ast, (node) => {
    if (node.type !== "CallExpression") return;
    const name = calleeName(node);
    if (name === "dwell") {
      dwells.push({ category: dwellCategory(node, categories), line: node.loc.start.line });
    } else if (name === "injectLatency") {
      injectLatency += 1;
    }
  });

  return { filePath, dwells, injectLatency };
}

/**
 * Fold per-file inventories into the report's totals.
 *
 * `negativeAssertion` is excluded from `debt` on purpose — see the module
 * comment. `debt` is the number a conversion effort is trying to move.
 */
export function summarize(inventories, categories) {
  const byCategory = Object.fromEntries(categories.map((category) => [category, 0]));
  let unknown = 0;
  let injectLatency = 0;

  for (const inventory of inventories) {
    injectLatency += inventory.injectLatency;
    for (const dwell of inventory.dwells) {
      if (dwell.category in byCategory) byCategory[dwell.category] += 1;
      else unknown += 1;
    }
  }

  const permanent = byCategory["negative-assertion"] ?? 0;
  const total = Object.values(byCategory).reduce((sum, count) => sum + count, 0) + unknown;

  return {
    byCategory,
    unknown,
    injectLatency,
    totalDwells: total,
    permanent,
    // Everything that is a compromise rather than a fact of the test.
    debt: total - permanent,
    unverified: byCategory.unverified ?? 0,
  };
}
