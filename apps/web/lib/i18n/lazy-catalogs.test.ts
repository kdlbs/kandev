import { beforeEach, describe, expect, it, vi } from "vitest";

/**
 * The eager-`en` / lazy-everything-else split.
 *
 * These assertions cannot be made against the shared instance: `vitest.setup.ts`
 * calls `loadAllLocalesForTests()`, so by the time any suite runs every catalog
 * is already registered and "is pseudo lazy?" would answer yes vacuously. Each
 * test therefore resets the module registry and imports a FRESH `lib/i18n`,
 * which brings a fresh `i18next` singleton with nothing loaded.
 *
 * What is being pinned:
 *   - `en` is present the instant `init()` returns, with no await anywhere. It
 *     is `fallbackLng`, and `returnNull: false` renders a missing key as the key
 *     itself, so an un-awaited boot without `en` paints `settings:displayLanguage`
 *     on screen rather than throwing. That is the silent failure this split has
 *     to not introduce.
 *   - No other locale is in the bundle until something asks for it.
 *   - `knownNamespaces()` is still a synchronous, complete answer. It is derived
 *     from `import.meta.glob` keys, and dropping `eager` makes the module bodies
 *     lazy but NOT the file list — this is the test that says so out loud.
 */

const SETTINGS_NS = "settings";
const DISPLAY_LANGUAGE = "displayLanguage";

async function freshI18n() {
  vi.resetModules();
  const module = await import("./index");
  module.initI18nForTests();
  return module;
}

beforeEach(() => {
  vi.resetModules();
});

describe("catalog loading", () => {
  it("has en in memory synchronously and no other locale", async () => {
    const { i18n } = await freshI18n();

    expect(i18n.isInitialized).toBe(true);
    expect(i18n.hasResourceBundle("en", SETTINGS_NS)).toBe(true);
    expect(i18n.t(`${SETTINGS_NS}:${DISPLAY_LANGUAGE}`)).toBe("Display language");

    for (const locale of ["pt-pt", "zh-cn", "pseudo"]) {
      expect(
        i18n.hasResourceBundle(locale, SETTINGS_NS),
        `${locale} must not be in the initial bundle`,
      ).toBe(false);
    }
  });

  it("lists every namespace before a single catalog has been fetched", async () => {
    const { knownNamespaces } = await freshI18n();

    const namespaces = knownNamespaces();
    // Sampled rather than snapshotted: the point is that the list is complete
    // and available with no await, not which namespaces exist this week.
    expect(namespaces).toContain("common");
    expect(namespaces).toContain(SETTINGS_NS);
    expect(namespaces).toContain("kanban");
    expect(namespaces.length).toBeGreaterThan(20);
  });

  it("registers a locale's messages on demand", async () => {
    const { i18n, loadLocale } = await freshI18n();

    await loadLocale("pt-pt");

    expect(i18n.hasResourceBundle("pt-pt", SETTINGS_NS)).toBe(true);
    expect(i18n.getResource("pt-pt", SETTINGS_NS, DISPLAY_LANGUAGE)).toBe("Idioma de apresentação");
    // Loading a catalog must not switch the active language on its own.
    expect(i18n.language).toBe("en");
  });

  it("shares one load between concurrent callers and is idempotent after", async () => {
    const { i18n, loadLocale } = await freshI18n();
    const register = vi.spyOn(i18n, "addResourceBundle");

    // Two switches in a row, before the first has landed.
    await Promise.all([loadLocale("zh-cn"), loadLocale("zh-cn")]);
    const afterFirstLoad = register.mock.calls.length;
    expect(afterFirstLoad).toBeGreaterThan(0);
    expect(i18n.hasResourceBundle("zh-cn", SETTINGS_NS)).toBe(true);

    // A third call once it is registered fetches nothing at all.
    await loadLocale("zh-cn");
    expect(register.mock.calls.length).toBe(afterFirstLoad);

    register.mockRestore();
  });

  it("resolves immediately for en, which is bundled", async () => {
    const { loadLocale } = await freshI18n();
    await expect(loadLocale("en")).resolves.toBeUndefined();
  });
});

describe("preloadLocale", () => {
  it("makes the boot locale active with its real messages", async () => {
    const { i18n, preloadLocale } = await freshI18n();

    await preloadLocale("zh-cn");

    expect(i18n.language).toBe("zh-cn");
    expect(i18n.t(`${SETTINGS_NS}:${DISPLAY_LANGUAGE}`)).toBe("显示语言");
  });

  /**
   * The regression this whole sequencing exists to prevent: `main.tsx` awaits
   * `preloadLocale` before `createRoot().render()`, so the first render must
   * never see a key string. Asserting on the resolved state is the closest a
   * unit test gets to "look at it" — the visual check is the pseudo-locale.
   */
  it("never leaves a key rendering as its own name", async () => {
    const { i18n, preloadLocale } = await freshI18n();

    await preloadLocale("pt-pt");

    const translated = i18n.t(`${SETTINGS_NS}:${DISPLAY_LANGUAGE}`);
    expect(translated).not.toContain(":");
    expect(translated).toBe("Idioma de apresentação");
  });

  it("falls back to English rather than blocking the boot on a missing namespace", async () => {
    const { i18n, preloadLocale } = await freshI18n();

    // zh-cn ships fewer namespaces than en; the ones it lacks must resolve
    // through fallbackLng instead of rendering the key.
    await preloadLocale("zh-cn");

    expect(i18n.language).toBe("zh-cn");
    expect(i18n.t("common:save")).not.toBe("common:save");
  });
});

/**
 * Two rapid selections race, because activation now awaits a fetch.
 *
 * Eager bundling had no window here: the catalog was already in memory, so
 * `activateLocale` reached `changeLanguage` before the next click could land.
 * Lazy loading opens one — and the loser of the race is whichever chunk resolves
 * LAST, not whichever the user picked last. Without a guard, choosing `zh-cn`
 * then `pt-pt` can leave the app in `zh-cn` and persist a cookie the user never
 * selected, so the wrong language survives a reload too.
 */
describe("concurrent activation", () => {
  it("ignores a stale activation that resolves after a newer one", async () => {
    // The race only exists because the catalog is fetched. Delaying zh-cn's
    // chunk forces the ordering a fast local run never produces: the user's
    // FIRST pick resolves LAST. Without the sequence guard the late zh-cn
    // continuation wins and persists a cookie the user did not choose.
    vi.doMock("../../src/locales/zh-cn/settings.json", async () => {
      const actual = await vi.importActual<Record<string, unknown>>(
        "../../src/locales/zh-cn/settings.json",
      );
      await new Promise((resolve) => setTimeout(resolve, 50));
      return actual;
    });

    const { i18n, activateLocale } = await freshI18n();

    const stale = activateLocale("zh-cn");
    const current = activateLocale("pt-pt");
    const [staleResult] = await Promise.all([stale, current]);

    expect(i18n.language).toBe("pt-pt");
    // The abandoned call reports the winner, not what it was asked for. The
    // switcher does `setLocale(await activateLocale(value))` and the stale call
    // settles LAST, so returning "zh-cn" here would leave the dropdown reading
    // 简体中文 over a Portuguese app — the race surviving in the one place the
    // user looks to confirm it did not happen.
    expect(staleResult).toBe("pt-pt");
    vi.doUnmock("../../src/locales/zh-cn/settings.json");
  });

  it("still activates normally when calls do not overlap", async () => {
    const { i18n, activateLocale } = await freshI18n();

    await activateLocale("zh-cn");
    expect(i18n.language).toBe("zh-cn");

    await activateLocale("pt-pt");
    expect(i18n.language).toBe("pt-pt");
  });
});
