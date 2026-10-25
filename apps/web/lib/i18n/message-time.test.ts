import { afterEach, describe, expect, it, vi } from "vitest";
import { activateLocale } from "./index";
import { formatMessageTime, resolveMessageTimeLocale } from "./message-time";

const NOW = Date.parse("2026-10-03T12:00:00Z");
const DATE = "2026-10-03T10:15:00Z";
const SHORT_OPTIONS: Intl.DateTimeFormatOptions = { dateStyle: "short", timeStyle: "short" };
const LONG_OPTIONS: Intl.DateTimeFormatOptions = {
  dateStyle: "long",
  timeStyle: "medium",
};

afterEach(async () => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  await activateLocale("en");
});

describe("formatMessageTime", () => {
  it.each([
    ["relative", "relative"],
    ["absolute_short", "absolute_short"],
    ["absolute_long", "absolute_long"],
  ] as const)("pairs label and counterpart for %s", (display, expectedLabel) => {
    vi.stubGlobal("navigator", { languages: ["en-US"] });
    vi.useFakeTimers();
    vi.setSystemTime(NOW);
    activateLocale("en-US");
    const result = formatMessageTime(DATE, display);
    expect(result).not.toBeNull();
    const short = new Intl.DateTimeFormat("en-US", SHORT_OPTIONS).format(new Date(DATE));
    if (expectedLabel === "relative") {
      expect(result?.counterpart).toBe(short);
    } else {
      expect(result?.label).toBe(
        new Intl.DateTimeFormat(
          "en-US",
          expectedLabel === "absolute_short" ? SHORT_OPTIONS : LONG_OPTIONS,
        ).format(new Date(DATE)),
      );
      expect(result?.counterpart).toMatch(/ago$/);
    }
    if (display === "absolute_long") expect(result?.label).not.toBe(result?.counterpart);
  });

  it("uses the supplied reference time for compact relative labels", async () => {
    await activateLocale("en");
    vi.useFakeTimers();
    vi.setSystemTime(NOW + 8 * 86_400_000);
    vi.stubGlobal("navigator", { languages: ["en-US"] });
    const result = formatMessageTime(DATE, "relative", NOW);
    expect(result?.label).toMatch(/ago$/);
  });

  it("uses a resolved-locale calendar date at exactly seven days, but a relative label one millisecond earlier", () => {
    vi.useFakeTimers();
    vi.setSystemTime(NOW);
    activateLocale("en");
    vi.stubGlobal("navigator", { languages: ["en-GB"] });
    const atBoundary = formatMessageTime(new Date(NOW - 7 * 86_400_000).toISOString(), "relative");
    const beforeBoundary = formatMessageTime(
      new Date(NOW - 7 * 86_400_000 + 1).toISOString(),
      "relative",
    );
    expect(atBoundary?.label).toBe(
      new Intl.DateTimeFormat("en-GB", {
        year: "numeric",
        month: "numeric",
        day: "numeric",
      }).format(new Date(NOW - 7 * 86_400_000)),
    );
    expect(beforeBoundary?.label).toMatch(/ago$/);
    expect(atBoundary?.counterpart).not.toMatch(/^\d{1,2}[/.]\d{1,2}[/.]\d{2,4}$/);
  });

  it.each(["", "0", "2026-02-30T10:00:00Z", "2026-10-03T12:00:00"])(
    "returns no time for invalid timestamp %j",
    (createdAt) => expect(formatMessageTime(createdAt, "relative")).toBeNull(),
  );
});

describe("resolveMessageTimeLocale", () => {
  it("keeps regional interface locales authoritative", async () => {
    await activateLocale("pt-PT");
    vi.stubGlobal("navigator", { languages: ["pt-BR", "pt-PT"] });
    expect(resolveMessageTimeLocale()).toBe("pt-pt");
  });

  it("uses the first matching browser region for regionless interface locales", async () => {
    await activateLocale("en");
    vi.stubGlobal("navigator", { languages: ["de-DE", "en-GB", "en-US"] });
    expect(resolveMessageTimeLocale()).toBe("en-GB");
  });

  it("falls back to interface locale when no browser language matches", async () => {
    await activateLocale("en");
    vi.stubGlobal("navigator", { languages: ["de-DE"] });
    expect(resolveMessageTimeLocale()).toBe("en");
  });
});
