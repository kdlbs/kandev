import { render, screen } from "@testing-library/react";
import { beforeAll, describe, expect, it } from "vitest";

import { CookieExpiry, formatExpiry } from "./jira-secret-help";
import { activateLocale, i18n } from "@/lib/i18n";

// The shared instance's own `t`, rather than the string-typed re-export from
// `@/lib/i18n`, so it satisfies the branded `TFunction` the helper takes.
const t = i18n.t;

beforeAll(async () => {
  // The labels come from the real `jira` catalog, so the expected strings below
  // double as a check that each key exists and carries the right plural forms.
  await activateLocale("en");
});

const IN = (ms: number) => new Date(Date.now() + ms).toISOString();
const HOUR = 60 * 60 * 1000;
const DAY = 24 * HOUR;

describe("formatExpiry", () => {
  it("reports an unknown expiry for an unparseable timestamp", () => {
    expect(formatExpiry(t, "not-a-date")).toEqual({ label: "Expiry unknown", tone: "warn" });
  });

  it("reports an expired cookie once the timestamp is in the past", () => {
    expect(formatExpiry(t, IN(-HOUR))).toEqual({
      label: "Cookie expired — paste a fresh one",
      tone: "danger",
    });
  });

  it("counts down in hours under a day, rounding up to at least one", () => {
    expect(formatExpiry(t, IN(5 * HOUR))).toEqual({
      label: "Cookie expires in 5h",
      tone: "danger",
    });
    // Anything still valid is at least "1h", never "0h".
    expect(formatExpiry(t, IN(60_000)).label).toBe("Cookie expires in 1h");
  });

  // The day count goes through i18next `count`, so the singular form proves the
  // _one/_other split is wired up rather than an English "s" being appended.
  it("counts down in days past 24h, and inflects the singular", () => {
    expect(formatExpiry(t, IN(30 * HOUR))).toEqual({
      label: "Cookie expires in 1 day",
      tone: "warn",
    });
    expect(formatExpiry(t, IN(3 * DAY)).label).toBe("Cookie expires in 3 days");
  });

  it("escalates the tone as the deadline approaches", () => {
    expect(formatExpiry(t, IN(30 * DAY)).tone).toBe("ok");
    expect(formatExpiry(t, IN(3 * DAY)).tone).toBe("warn");
    expect(formatExpiry(t, IN(2 * HOUR)).tone).toBe("danger");
  });
});

describe("CookieExpiry", () => {
  it("puts the absolute timestamp in the title attribute", () => {
    const expiresAt = IN(3 * DAY);
    render(<CookieExpiry expiresAt={expiresAt} />);
    const el = screen.getByText("Cookie expires in 3 days");
    expect(el.getAttribute("title")).toBeTruthy();
  });

  // `Intl.DateTimeFormat().format()` throws RangeError on an invalid date, where
  // the `toLocaleString()` this replaced returned "Invalid Date". A legacy or
  // corrupted `secretExpiresAt` would otherwise take the whole settings card
  // down — `formatExpiry` has a branch for exactly that value.
  it("renders the unknown-expiry label without a title for an invalid timestamp", () => {
    render(<CookieExpiry expiresAt="not-a-date" />);
    const el = screen.getByText("Expiry unknown");
    expect(el.getAttribute("title")).toBeNull();
  });
});
