import { afterEach, describe, expect, it, vi } from "vitest";

import {
  activateLocale,
  DEFAULT_LOCALE,
  i18n,
  isSupportedLocale,
  normalizeLocale,
  selectableLocales,
  SUPPORTED_LOCALES,
} from "./index";
import { LOCALE_COOKIE, readLocaleCookie } from "./cookie";

afterEach(async () => {
  document.cookie = `${LOCALE_COOKIE}=; path=/; max-age=0`;
  vi.clearAllMocks();
  // Leave the shared instance on the default locale for other suites.
  await activateLocale(DEFAULT_LOCALE);
});

describe("locale predicates", () => {
  it("recognizes supported locales", () => {
    expect(isSupportedLocale("en")).toBe(true);
    expect(isSupportedLocale("pseudo")).toBe(true);
    expect(isSupportedLocale("fr")).toBe(false);
    expect(isSupportedLocale(42)).toBe(false);
  });

  it("normalizes unknown values to the default", () => {
    expect(normalizeLocale("pseudo")).toBe("pseudo");
    expect(normalizeLocale("nope")).toBe(DEFAULT_LOCALE);
    expect(normalizeLocale(undefined)).toBe(DEFAULT_LOCALE);
  });

  it("exposes en as the default and lists both locales", () => {
    expect(DEFAULT_LOCALE).toBe("en");
    expect([...SUPPORTED_LOCALES]).toEqual(["en", "pseudo"]);
  });

  it("hides the pseudo locale in production builds only", () => {
    expect(selectableLocales(false)).toEqual(["en", "pseudo"]);
    expect(selectableLocales(true)).toEqual(["en"]);
  });
});

describe("activateLocale", () => {
  it("activates the locale, sets <html lang>, and writes the cookie", async () => {
    const result = await activateLocale("pseudo");
    expect(result).toBe("pseudo");
    expect(i18n.language).toBe("pseudo");
    expect(document.documentElement.lang).toBe("pseudo");
    expect(readLocaleCookie()).toBe("pseudo");
  });

  it("coerces an invalid locale to en", async () => {
    const result = await activateLocale("klingon");
    expect(result).toBe("en");
    expect(i18n.language).toBe("en");
    expect(document.documentElement.lang).toBe("en");
  });

  it("resolves real catalog messages for the active locale", async () => {
    await activateLocale("en");
    expect(i18n.t("settings:displayLanguage")).toBe("Display language");
    // The pseudo catalog is generated from `en`, so the same key is accented —
    // this is the completeness oracle the QA locale exists for.
    await activateLocale("pseudo");
    expect(i18n.t("settings:displayLanguage")).not.toBe("Display language");
    expect(i18n.t("settings:displayLanguage")).toMatch(/[À-ɏ]/);
  });
});
