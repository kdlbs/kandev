import { afterAll, beforeAll, describe, expect, it } from "vitest";

import {
  formatDate,
  formatNumber,
  formatRelative,
  formatRelativeTime,
  formatSidebarElapsedTime,
} from "./formats";
import { activateLocale } from "./index";

const FORMAT_DATE = "2026-07-27T00:00:00Z";
const FORMAT_NOW = "2026-07-27T12:00:00Z";

beforeAll(async () => {
  // Activate en so the relative-time buckets resolve from the real catalog,
  // matching the former timeAgo behavior byte-for-byte.
  await activateLocale("en");
});

describe("formatRelative (en, timeAgo-compatible)", () => {
  const now = new Date(FORMAT_NOW).getTime();
  const ago = (ms: number) => new Date(now - ms).toISOString();

  it("returns empty string for empty or invalid input", () => {
    expect(formatRelative("", now)).toBe("");
    expect(formatRelative("not-a-date", now)).toBe("");
  });

  it("returns 'just now' under a minute", () => {
    expect(formatRelative(ago(30_000), now)).toBe("just now");
  });

  it("formats minutes, hours, and days", () => {
    expect(formatRelative(ago(5 * 60_000), now)).toBe("5m ago");
    expect(formatRelative(ago(3 * 3_600_000), now)).toBe("3h ago");
    expect(formatRelative(ago(2 * 86_400_000), now)).toBe("2d ago");
  });
});

describe("formatSidebarElapsedTime", () => {
  const now = new Date(FORMAT_NOW).getTime();
  const ago = (seconds: number) => new Date(now - seconds * 1000).toISOString();

  it("returns an empty string for empty or invalid input", () => {
    expect(formatSidebarElapsedTime("", now)).toBe("");
    expect(formatSidebarElapsedTime("not-a-date", now)).toBe("");
  });

  it("clamps future timestamps to zero seconds", () => {
    expect(formatSidebarElapsedTime(new Date(now + 60_000).toISOString(), now)).toBe("0s");
  });

  it("uses the compact elapsed-time bucket boundaries", () => {
    expect(formatSidebarElapsedTime(ago(59), now)).toBe("59s");
    expect(formatSidebarElapsedTime(ago(60), now)).toBe("1m");
    expect(formatSidebarElapsedTime(ago(59 * 60 + 59), now)).toBe("59m");
    expect(formatSidebarElapsedTime(ago(60 * 60), now)).toBe("1h");
    expect(formatSidebarElapsedTime(ago(24 * 60 * 60 - 1), now)).toBe("23h");
    expect(formatSidebarElapsedTime(ago(24 * 60 * 60), now)).toBe("1d");
    expect(formatSidebarElapsedTime(ago(7 * 24 * 60 * 60 - 1), now)).toBe("6d");
    expect(formatSidebarElapsedTime(ago(7 * 24 * 60 * 60), now)).toBe("1w");
    expect(formatSidebarElapsedTime(ago(365 * 24 * 60 * 60 - 1), now)).toBe("52w");
    expect(formatSidebarElapsedTime(ago(365 * 24 * 60 * 60), now)).toBe("1y");
  });

  it("bounds the displayed years at 99+", () => {
    expect(formatSidebarElapsedTime(ago(99 * 365 * 24 * 60 * 60), now)).toBe("99y");
    expect(formatSidebarElapsedTime(ago(100 * 365 * 24 * 60 * 60), now)).toBe("99+y");
  });

  it.each([
    ["en", "3w"],
    ["pt-pt", "3sem"],
    ["zh-cn", "3周"],
    ["zh-hk", "3週"],
    ["zh-tw", "3週"],
    ["pseudo", "3ŵ"],
  ] as const)("uses the %s compact unit catalog", async (locale, expected) => {
    await activateLocale(locale);
    expect(formatSidebarElapsedTime(ago(3 * 7 * 24 * 60 * 60), now)).toBe(expected);
  });

  afterAll(async () => {
    await activateLocale("en");
  });
});

describe("locale-aware Intl wrappers", () => {
  it("formats numbers using the active locale", async () => {
    await activateLocale("en");
    expect(formatNumber(1234567.89)).toBe("1,234,567.89");
  });

  it("maps the pseudo locale to en for Intl", async () => {
    await activateLocale("pseudo");
    // pseudo has no CLDR data; wrappers fall back to en formatting.
    expect(formatNumber(1000)).toBe("1,000");
    expect(formatDate(FORMAT_DATE, { year: "numeric" })).toBe("2026");
    await activateLocale("en");
  });

  it("passes zh-cn to number and date formatters", async () => {
    await activateLocale("zh-cn");
    const numberOptions: Intl.NumberFormatOptions = {
      style: "currency",
      currency: "CNY",
      currencyDisplay: "name",
    };
    const dateOptions: Intl.DateTimeFormatOptions = {
      year: "numeric",
      month: "long",
      day: "numeric",
      timeZone: "UTC",
    };
    const date = FORMAT_DATE;

    expect(formatNumber(1234.5, numberOptions)).toBe(
      new Intl.NumberFormat("zh-cn", numberOptions).format(1234.5),
    );
    expect(formatDate(date, dateOptions)).toBe(
      new Intl.DateTimeFormat("zh-cn", dateOptions).format(new Date(date)),
    );
    await activateLocale("en");
  });

  it.each([
    ["zh-tw", "TWD"],
    ["zh-hk", "HKD"],
  ] as const)("passes %s to number and date formatters", async (locale, currency) => {
    await activateLocale(locale);
    const numberOptions: Intl.NumberFormatOptions = {
      style: "currency",
      currency,
      currencyDisplay: "code",
    };
    const dateOptions: Intl.DateTimeFormatOptions = {
      year: "numeric",
      month: "long",
      day: "numeric",
      timeZone: "UTC",
    };
    const date = FORMAT_DATE;

    expect(formatNumber(1234.5, numberOptions)).toBe(
      new Intl.NumberFormat(locale, numberOptions).format(1234.5),
    );
    expect(formatDate(date, dateOptions)).toBe(
      new Intl.DateTimeFormat(locale, dateOptions).format(new Date(date)),
    );
    await activateLocale("en");
  });

  it("passes pt-pt to number and date formatters", async () => {
    await activateLocale("pt-pt");
    const dateOptions: Intl.DateTimeFormatOptions = {
      year: "numeric",
      month: "long",
      day: "numeric",
      timeZone: "UTC",
    };
    const date = FORMAT_DATE;

    expect(formatNumber(1234.5)).toBe(new Intl.NumberFormat("pt-pt").format(1234.5));
    expect(formatDate(date, dateOptions)).toBe(
      new Intl.DateTimeFormat("pt-pt", dateOptions).format(new Date(date)),
    );
    await activateLocale("en");
  });
});

/**
 * `host.utils.formatRelativeTime` (docs/plans/plugins/PLUGIN-API.md). Unlike
 * `formatRelative` above, this one goes through `Intl.RelativeTimeFormat`
 * rather than the message catalog, so it covers the full range up to years
 * and localizes without any keys — which is the point: three published
 * plugins ship hand-rolled English-only versions of this.
 */
const THREE_HOURS_AGO = "3 hours ago";

describe("formatRelativeTime (en)", () => {
  const now = new Date(FORMAT_NOW).getTime();
  const secondsAgo = (n: number) => new Date(now - n * 1000);
  const secondsAhead = (n: number) => new Date(now + n * 1000);

  it("returns an empty string for unparseable input", () => {
    expect(formatRelativeTime("not-a-date", now)).toBe("");
    expect(formatRelativeTime("", now)).toBe("");
  });

  it("collapses anything within ten seconds to 'now'", () => {
    expect(formatRelativeTime(secondsAgo(0), now)).toBe("now");
    expect(formatRelativeTime(secondsAgo(9), now)).toBe("now");
    expect(formatRelativeTime(secondsAhead(9), now)).toBe("now");
  });

  // The threshold is tested against raw milliseconds, not the rounded second
  // count. Rounding first moved the real cutoff to 9.5s and made it
  // asymmetric — Math.round(-9.5) is -9 but Math.round(9.5) is 10 — so 9.5s
  // ago read "now" while 9.5s ahead read "in 10 seconds". Reported by
  // @yattdev on #2410.
  it("uses a symmetric 10s 'now' cutoff that sub-second input cannot shift", () => {
    const ms = (n: number) => new Date(now - n);

    expect(formatRelativeTime(ms(9_400), now)).toBe("now");
    expect(formatRelativeTime(ms(9_600), now)).toBe("now");
    expect(formatRelativeTime(ms(9_900), now)).toBe("now");
    expect(formatRelativeTime(ms(9_999), now)).toBe("now");

    expect(formatRelativeTime(ms(10_000), now)).toBe("10 seconds ago");
    expect(formatRelativeTime(ms(10_100), now)).toBe("10 seconds ago");
  });

  it("applies the same cutoff to future timestamps as to past ones", () => {
    const ahead = (n: number) => new Date(now + n);

    expect(formatRelativeTime(ahead(9_500), now)).toBe("now");
    expect(formatRelativeTime(ahead(9_900), now)).toBe("now");
    expect(formatRelativeTime(ahead(10_000), now)).toBe("in 10 seconds");
  });

  // The other half of the report — 59.6s rendering as "1 minute ago" — is
  // intended. Unit selection rounds to the nearest unit, which describes
  // 59.6s better than "59 seconds ago" would.
  it("selects the nearest unit rather than truncating toward zero", () => {
    expect(formatRelativeTime(new Date(now - 59_600), now)).toBe("1 minute ago");
    expect(formatRelativeTime(new Date(now - 59_400), now)).toBe("59 seconds ago");
  });

  it("reports whole seconds below a minute", () => {
    expect(formatRelativeTime(secondsAgo(10), now)).toBe("10 seconds ago");
    expect(formatRelativeTime(secondsAgo(59), now)).toBe("59 seconds ago");
  });

  it("switches to minutes, hours, days and weeks at each boundary", () => {
    expect(formatRelativeTime(secondsAgo(60), now)).toBe("1 minute ago");
    expect(formatRelativeTime(secondsAgo(90), now)).toBe("1 minute ago");
    expect(formatRelativeTime(secondsAgo(60 * 60), now)).toBe("1 hour ago");
    expect(formatRelativeTime(secondsAgo(3 * 60 * 60), now)).toBe(THREE_HOURS_AGO);
    expect(formatRelativeTime(secondsAgo(2 * 24 * 60 * 60), now)).toBe("2 days ago");
    expect(formatRelativeTime(secondsAgo(3 * 7 * 24 * 60 * 60), now)).toBe("3 weeks ago");
    expect(formatRelativeTime(secondsAgo(800 * 24 * 60 * 60), now)).toBe("2 years ago");
  });

  // numeric: "auto" is deliberate — CLDR phrases the small magnitudes
  // idiomatically, which no interpolated number can do across locales.
  it("uses idiomatic phrasing for a single unit either side", () => {
    expect(formatRelativeTime(secondsAgo(24 * 60 * 60), now)).toBe("yesterday");
    expect(formatRelativeTime(secondsAhead(24 * 60 * 60), now)).toBe("tomorrow");
    expect(formatRelativeTime(secondsAgo(400 * 24 * 60 * 60), now)).toBe("last year");
  });

  it("phrases future timestamps as future", () => {
    expect(formatRelativeTime(secondsAhead(3 * 60 * 60), now)).toBe("in 3 hours");
    expect(formatRelativeTime(secondsAhead(5 * 24 * 60 * 60), now)).toBe("in 5 days");
  });

  it("accepts a Date, an epoch number, and an ISO string alike", () => {
    const date = secondsAgo(3 * 60 * 60);
    expect(formatRelativeTime(date, now)).toBe(THREE_HOURS_AGO);
    expect(formatRelativeTime(date.getTime(), now)).toBe(THREE_HOURS_AGO);
    expect(formatRelativeTime(date.toISOString(), now)).toBe(THREE_HOURS_AGO);
  });
});

/**
 * The reason this lives in the host rather than in each plugin. A hand-rolled
 * "3 hours ago" ladder is English-only by construction; Intl reads the active
 * locale, so the same call renders Portuguese for a pt-PT user.
 */
describe("formatRelativeTime (locale awareness)", () => {
  const now = new Date(FORMAT_NOW).getTime();
  const threeHoursAgo = new Date(now - 3 * 60 * 60 * 1000);

  afterAll(async () => {
    await activateLocale("en");
  });

  it("renders in the active locale, not English", async () => {
    const english = formatRelativeTime(threeHoursAgo, now);

    await activateLocale("pt-pt");
    const portuguese = formatRelativeTime(threeHoursAgo, now);

    expect(portuguese).not.toBe(english);
    expect(portuguese).toContain("3");
    expect(portuguese).toMatch(/horas/i);
  });
});
