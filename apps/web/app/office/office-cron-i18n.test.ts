import { describe, expect, it } from "vitest";

import { t } from "@/lib/i18n";
import enOffice from "@/src/locales/en/office.json";
import pseudoOffice from "@/src/locales/pseudo/office.json";

/**
 * A cron expression is syntax, not copy. Baked into a catalog message a
 * translator can reorder its fields, localise `MON`, or insert a space and ship
 * an example no cron parser accepts — and the pseudo-locale mangles it outright.
 * Both office cron examples therefore carry the expression as a `values` entry.
 *
 * The two live in different files — `standardCronExpressionExample` in
 * `routines/create-routine-dialog.tsx`, `aCronScheduleExample` in
 * `agents/[id]/layout.tsx` — so the guard sits at the `office` namespace root
 * rather than beside either call site.
 */
describe("office cron example messages", () => {
  const CRON_KEYS = ["standardCronExpressionExample", "aCronScheduleExample"] as const;

  /** Five whitespace-separated cron fields — what must never appear in a message. */
  const CRON_SHAPED = /[\d*/,-]+\s+[\d*/,-]+\s+[\d*/,-]+\s+[\d*/,-]+\s+[\dA-Z*/,-]+/;

  it.each(CRON_KEYS)("keeps the cron expression out of the en message for %s", (key) => {
    const message: string = enOffice[key];
    expect(message).toContain("{{cron}}");
    expect(message).not.toMatch(CRON_SHAPED);
  });

  it.each(CRON_KEYS)("keeps {{cron}} verbatim in the pseudo message for %s", (key) => {
    // The placeholder surviving transliteration is the proof the value is
    // substituted at render rather than living inside the translated text.
    expect(pseudoOffice[key]).toContain("{{cron}}");
  });

  it("renders the dialog hint byte-identically to the pre-extraction English", () => {
    expect(t("office:standardCronExpressionExample", { cron: "0 9 * * MON" })).toBe(
      "Standard cron expression (e.g. 0 9 * * MON for every Monday at 9am)",
    );
  });
});
