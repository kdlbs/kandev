import { formatDistanceToNow } from "date-fns";
import { enUS } from "date-fns/locale/en-US";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { dateLocale, formatTimeDistance, primeDateLocale, resolveDateLocale } from "./date-locale";
import { activateLocale, DEFAULT_LOCALE } from "./index";

// A fixed instant so the distance buckets are deterministic; `formatTimeDistance`
// measures against `Date.now()`, so each case offsets from a stubbed now.
const NOW = new Date("2026-08-07T12:00:00.000Z").getTime();

function ago(ms: number): Date {
  return new Date(NOW - ms);
}

const MINUTE = 60_000;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

describe("formatTimeDistance", () => {
  beforeEach(async () => {
    vi.useFakeTimers();
    vi.setSystemTime(NOW);
    await activateLocale(DEFAULT_LOCALE);
    return () => {
      vi.useRealTimers();
    };
  });

  // The guard clause the three per-integration helpers used to each own.
  it.each([undefined, null, "", "not-a-date"])("returns empty string for %j", (value) => {
    expect(formatTimeDistance(value)).toBe("");
  });

  it("accepts a Date, an ISO string, and an epoch number alike", () => {
    const date = ago(2 * HOUR);
    expect(formatTimeDistance(date)).toBe("about 2 hours ago");
    expect(formatTimeDistance(date.toISOString())).toBe("about 2 hours ago");
    expect(formatTimeDistance(date.getTime())).toBe("about 2 hours ago");
  });

  /**
   * The English output must not move: e2e specs query these timestamps by
   * accessible name, and this project has broken specs exactly that way.
   * Pinning literals rather than comparing against date-fns would pass even if
   * we accidentally swapped the default locale, so this asserts both — the
   * exact strings AND equality with an unlocalized call.
   */
  it.each([
    // Under 30s reads as "less than a minute"; 30s already rounds up to one.
    [10_000, "less than a minute ago"],
    [30_000, "1 minute ago"],
    [5 * MINUTE, "5 minutes ago"],
    [HOUR, "about 1 hour ago"],
    [3 * DAY, "3 days ago"],
  ])("renders %j ms ago as %j under en", (offset, expected) => {
    expect(formatTimeDistance(ago(offset))).toBe(expected);
  });

  it("is byte-identical to an unlocalized date-fns call under en", () => {
    for (const offset of [30_000, 5 * MINUTE, HOUR, 3 * DAY, 400 * DAY]) {
      const date = ago(offset);
      expect(formatTimeDistance(date)).toBe(formatDistanceToNow(date, { addSuffix: true }));
    }
  });
});

describe("resolveDateLocale", () => {
  it.each([
    ["en", "en-US"],
    ["pt-pt", "pt"],
    ["zh-cn", "zh-CN"],
    ["zh-tw", "zh-TW"],
    ["zh-hk", "zh-HK"],
  ])("maps %j to the %j date-fns locale", async (locale, code) => {
    const resolved = await resolveDateLocale(locale as "en");
    expect(resolved.code).toBe(code);
  });

  /**
   * There is no pseudo date locale. Mapping it to `enUS` is a decision, not a
   * fallthrough — see the module comment. Asserted so a future "just let it be
   * undefined" change has to be deliberate.
   */
  it("maps pseudo to en-US rather than leaving it undefined", async () => {
    await expect(resolveDateLocale("pseudo")).resolves.toBe(enUS);
  });
});

describe("dateLocale", () => {
  // Restore in a hook rather than at the end of each test body: a failing
  // assertion would skip inline cleanup and leak `pt-pt` and fake timers into
  // whatever runs next.
  afterEach(async () => {
    vi.useRealTimers();
    await activateLocale(DEFAULT_LOCALE);
  });

  it("follows the active i18next language once primed", async () => {
    await activateLocale("pt-pt");
    await primeDateLocale("pt-pt");
    expect(dateLocale().code).toBe("pt");

    await activateLocale(DEFAULT_LOCALE);
    expect(dateLocale().code).toBe("en-US");
  });

  it("renders Portuguese distances once pt is active", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(NOW);
    await activateLocale("pt-pt");
    await primeDateLocale("pt-pt");

    // The gap this PR closes: previously "about 1 hour ago" in Portuguese prose.
    expect(formatTimeDistance(ago(HOUR))).toBe("há aproximadamente 1 hora");
    expect(formatTimeDistance(ago(3 * DAY))).toBe("há 3 dias");
  });
});
