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

const ZH_CN_LOCALE = "zh-cn";
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
    expect(isSupportedLocale(ZH_CN_LOCALE)).toBe(true);
    expect(isSupportedLocale("zh-CN")).toBe(true);
    expect(isSupportedLocale("  ZH-CN  ")).toBe(true);
    expect(isSupportedLocale("pseudo")).toBe(true);
    expect(isSupportedLocale("fr")).toBe(false);
    expect(isSupportedLocale(42)).toBe(false);
  });

  it("normalizes unknown values to the default", () => {
    expect(normalizeLocale(ZH_CN_LOCALE)).toBe(ZH_CN_LOCALE);
    expect(normalizeLocale("zh-CN")).toBe(ZH_CN_LOCALE);
    expect(normalizeLocale("  ZH-CN  ")).toBe(ZH_CN_LOCALE);
    expect(normalizeLocale("pseudo")).toBe("pseudo");
    expect(normalizeLocale("nope")).toBe(DEFAULT_LOCALE);
    expect(normalizeLocale(undefined)).toBe(DEFAULT_LOCALE);
  });

  it("exposes en as the default and lists every shipped locale", () => {
    expect(DEFAULT_LOCALE).toBe("en");
    expect([...SUPPORTED_LOCALES]).toEqual(["en", "zh-cn", "pseudo"]);
  });

  it("hides the pseudo locale in production builds only", () => {
    expect(selectableLocales(false)).toEqual(["en", "zh-cn", "pseudo"]);
    expect(selectableLocales(true)).toEqual(["en", "zh-cn"]);
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
    expect(i18n.t(DISPLAY_LANGUAGE_KEY)).toBe("Display language");
    // The pseudo catalog is generated from `en`, so the same key is accented —
    // this is the completeness oracle the QA locale exists for.
    await activateLocale("pseudo");
    expect(i18n.t(DISPLAY_LANGUAGE_KEY)).not.toBe("Display language");
    expect(i18n.t(DISPLAY_LANGUAGE_KEY)).toMatch(/[À-ɏ]/);
  });

  it("activates Simplified Chinese and resolves its real catalog", async () => {
    const result = await activateLocale(ZH_CN_LOCALE);
    expect(result).toBe(ZH_CN_LOCALE);
    expect(i18n.language).toBe(ZH_CN_LOCALE);
    expect(document.documentElement.lang).toBe(ZH_CN_LOCALE);
    expect(readLocaleCookie()).toBe(ZH_CN_LOCALE);
    expect(i18n.hasResourceBundle(ZH_CN_LOCALE, "settings")).toBe(true);
    expect(i18n.getResource(ZH_CN_LOCALE, "settings", "displayLanguage")).toBe("显示语言");
    expect(i18n.t(DISPLAY_LANGUAGE_KEY)).toBe("显示语言");
  });
});
