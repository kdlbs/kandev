import AxeBuilder from "@axe-core/playwright";
import type { Page } from "@playwright/test";

type AxeBuilderInstance = InstanceType<typeof AxeBuilder>;
export type AxeScanResults = Awaited<ReturnType<AxeBuilderInstance["analyze"]>>;
export type AxeViolation = AxeScanResults["violations"][number];

const DEFAULT_TAGS = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"];

export type AxeScanOptions = {
  /** axe-core rule tags to check against. Defaults to WCAG 2.1 A/AA. */
  tags?: string[];
  /** Rule IDs to skip, for a known/tracked false positive. */
  disableRules?: string[];
};

/** Runs an axe-core accessibility scan of the current page. */
export async function runAxeScan(
  page: Page,
  options: AxeScanOptions = {},
): Promise<AxeScanResults> {
  let builder = new AxeBuilder({ page }).withTags(options.tags ?? DEFAULT_TAGS);
  if (options.disableRules) builder = builder.disableRules(options.disableRules);
  return builder.analyze();
}

/** A one-line-per-violation summary, for a readable assertion failure message. */
export function summarizeAxeViolations(results: AxeScanResults): string {
  return results.violations
    .map(
      (violation) =>
        `${violation.id} (${violation.impact ?? "unknown"}): ${violation.help} - ${violation.nodes.length} node(s)`,
    )
    .join("\n");
}
