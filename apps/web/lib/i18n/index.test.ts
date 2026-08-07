import { afterEach, describe, expect, it, vi } from "vitest";

import {
  activateLocale,
  DEFAULT_LOCALE,
  i18n,
  isSupportedLocale,
  normalizeLocale,
  PSEUDO_LOCALE_BUNDLED,
  selectableLocales,
  SUPPORTED_LOCALES,
} from "./index";
import { LOCALE_COOKIE, readLocaleCookie } from "./cookie";

const ZH_CN_LOCALE = "zh-cn";
const PT_PT_LOCALE = "pt-pt";
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
    expect(isSupportedLocale(PT_PT_LOCALE)).toBe(true);
    expect(isSupportedLocale("pt-PT")).toBe(true);
    expect(isSupportedLocale("pseudo")).toBe(true);
    expect(isSupportedLocale("fr")).toBe(false);
    expect(isSupportedLocale(42)).toBe(false);
  });

  it("normalizes unknown values to the default", () => {
    expect(normalizeLocale(ZH_CN_LOCALE)).toBe(ZH_CN_LOCALE);
    expect(normalizeLocale("zh-CN")).toBe(ZH_CN_LOCALE);
    expect(normalizeLocale("  ZH-CN  ")).toBe(ZH_CN_LOCALE);
    expect(normalizeLocale("pt-PT")).toBe(PT_PT_LOCALE);
    expect(normalizeLocale("pseudo")).toBe("pseudo");
    expect(normalizeLocale("nope")).toBe(DEFAULT_LOCALE);
    expect(normalizeLocale(undefined)).toBe(DEFAULT_LOCALE);
  });

  it("exposes en as the default and lists every shipped locale", () => {
    expect(DEFAULT_LOCALE).toBe("en");
    expect([...SUPPORTED_LOCALES]).toEqual(["en", "pt-pt", "zh-cn", "pseudo"]);
  });

  // Only the `isProd` half of the contract. `selectableLocales` also requires
  // `PSEUDO_LOCALE_BUNDLED`, and that half cannot be exercised here: the constant
  // is fixed to `true` for the whole vitest run (config resolves in serve mode).
  // `bundling.test.ts` covers the decision that produces it, in both directions.
  it("hides the pseudo locale from production builds", () => {
    expect(selectableLocales(false)).toEqual(["en", "pt-pt", "zh-cn", "pseudo"]);
    expect(selectableLocales(true)).toEqual(["en", "pt-pt", "zh-cn"]);
  });

  /**
   * The include half of the bundling switch, asserted end to end.
   *
   * `shouldBundlePseudoLocale` is unit-tested in `bundling.test.ts`, but that
   * only covers the decision — not that `vite.config.ts` actually defines
   * `__KANDEV_PSEUDO_LOCALE_BUNDLED__` and that the constant reaches this module.
   * If that wiring breaks in the "exclude" direction, dev and e2e lose the
   * pseudo oracle silently: every screen simply falls back to `en` and reads as
   * un-externalized. This pins it, and "resolves real catalog messages for the
   * active locale" below is what proves the catalog itself came with it.
   */
  it("bundles the pseudo catalog outside a production build", () => {
    expect(PSEUDO_LOCALE_BUNDLED).toBe(true);
    expect(selectableLocales(false)).toContain("pseudo");
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

  // `task:deleteTasksCount` shipped with a plural noun in its `_one` form
  // ("Delete {{count}} tasks"). The only call site is behind a `length > 1`
  // guard, so English never selected `_one` and no gate could see it — but the
  // form is not dead for translators: Russian routes 21/31/101 through the
  // `one` category, so every locale generated from this source needs it right.
  it("inflects the singular of a count-driven label", async () => {
    await activateLocale("en");
    expect(i18n.t("task:deleteTasksCount", { count: 1 })).toBe("Delete 1 task");
    expect(i18n.t("task:deleteTasksCount", { count: 2 })).toBe("Delete 2 tasks");
  });

  // `cliModeYourPromptWillBe` was keyed in both `common` and `task` and had
  // already drifted by one character (em dash vs hyphen). `common` is the only
  // home; a reintroduced `task:` twin would start the drift over.
  it("keeps the CLI-mode hint in a single namespace", async () => {
    await activateLocale("en");
    expect(i18n.t("common:cliModeYourPromptWillBe")).toBe(
      "CLI mode — your prompt will be auto-injected into the terminal",
    );
    expect(i18n.getResource("en", "task", "cliModeYourPromptWillBe")).toBeUndefined();
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

  it("activates European Portuguese and resolves its real catalog", async () => {
    const result = await activateLocale("pt-PT");
    expect(result).toBe(PT_PT_LOCALE);
    expect(i18n.language).toBe(PT_PT_LOCALE);
    expect(document.documentElement.lang).toBe(PT_PT_LOCALE);
    expect(readLocaleCookie()).toBe(PT_PT_LOCALE);
    expect(i18n.hasResourceBundle(PT_PT_LOCALE, "settings")).toBe(true);
    expect(i18n.getResource(PT_PT_LOCALE, "settings", "displayLanguage")).toBe(
      "Idioma de apresentação",
    );
    expect(i18n.t(DISPLAY_LANGUAGE_KEY)).toBe("Idioma de apresentação");
  });
});
