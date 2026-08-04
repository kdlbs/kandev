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

const DISPLAY_LANGUAGE_KEY = "settings:displayLanguage";

afterEach(async () => {
  document.cookie = `${LOCALE_COOKIE}=; path=/; max-age=0`;
  vi.clearAllMocks();
  // Leave the shared instance on the default locale for other suites.
  await activateLocale(DEFAULT_LOCALE);
});

describe("locale predicates", () => {
  it("recognizes supported locales", () => {
    expect(isSupportedLocale("en")).toBe(true);
    expect(isSupportedLocale("zh-CN")).toBe(true);
    expect(isSupportedLocale("pseudo")).toBe(true);
    expect(isSupportedLocale("fr")).toBe(false);
    expect(isSupportedLocale(42)).toBe(false);
  });

  it("normalizes unknown values to the default", () => {
    expect(normalizeLocale("pseudo")).toBe("pseudo");
    expect(normalizeLocale("zh-CN")).toBe("zh-CN");
    expect(normalizeLocale("zh-cn")).toBe("zh-CN");
    expect(normalizeLocale("nope")).toBe(DEFAULT_LOCALE);
    expect(normalizeLocale(undefined)).toBe(DEFAULT_LOCALE);
  });

  it("exposes en as the default and lists all locales", () => {
    expect(DEFAULT_LOCALE).toBe("en");
    expect([...SUPPORTED_LOCALES]).toEqual(["en", "zh-CN", "pseudo"]);
  });

  it("hides the pseudo locale in production builds only", () => {
    expect(selectableLocales(false)).toEqual(["en", "zh-CN", "pseudo"]);
    expect(selectableLocales(true)).toEqual(["en", "zh-CN"]);
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

  it("activates Simplified Chinese from a case-insensitive locale tag", async () => {
    const result = await activateLocale("zh-cn");
    expect(result).toBe("zh-CN");
    expect(i18n.language).toBe("zh-CN");
    expect(document.documentElement.lang).toBe("zh-CN");
    expect(readLocaleCookie()).toBe("zh-CN");
    expect(i18n.t(DISPLAY_LANGUAGE_KEY)).toBe("显示语言");
  });

  it("resolves real catalog messages for the active locale", async () => {
    await activateLocale("en");
    expect(i18n.t(DISPLAY_LANGUAGE_KEY)).toBe("Display language");
    // The pseudo catalog is generated from `en`, so the same key is accented —
    // this is the completeness oracle the QA locale exists for.
    await activateLocale("pseudo");
    expect(i18n.t(DISPLAY_LANGUAGE_KEY)).not.toBe("Display language");
    expect(i18n.t(DISPLAY_LANGUAGE_KEY)).toMatch(/[À-ɏ]/);
  });
});
