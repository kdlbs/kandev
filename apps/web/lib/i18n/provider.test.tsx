import { describe, expect, it, vi } from "vitest";

/**
 * `I18nProvider` must initialize the shared instance as a side effect of being
 * imported, because nothing else in the boot path does.
 *
 * react-i18next suspends when `useTranslation()` runs against an uninitialized
 * instance. There is no Suspense boundary above the React root, so a suspended
 * first render never commits: the app shows a blank page with no console error,
 * no page error and no unhandled rejection — every asset returns 200 and
 * `#root` is simply empty. It is a genuinely silent failure.
 *
 * Initialization used to happen by accident. `activateLocale()` only runs from
 * the language switcher, so at boot the instance was set up purely because some
 * module called the module-level `t()` while evaluating — `t()` calls
 * `ensureInitialized`. Removing the last such call (a module-scope config
 * constant) blanked the whole app and every e2e test failed on an empty page.
 * See docs/i18n.md ("How it's wired").
 *
 * NOTE: this cannot be asserted by rendering. `vitest.setup.ts` calls
 * `initI18nForTests()` for every suite, so `i18n.isInitialized` is already true
 * and any render-based check would pass even with the boot call deleted. The
 * import side effect itself is therefore what gets verified here.
 */
describe("I18nProvider boot contract", () => {
  it("initializes the shared instance when the module is imported", async () => {
    vi.resetModules();
    const initI18n = vi.fn();

    vi.doMock("./index", async () => ({
      ...(await vi.importActual<typeof import("./index")>("./index")),
      initI18n,
    }));

    await import("./provider");

    expect(
      initI18n,
      "provider.tsx must call initI18n() at module scope — without it the app " +
        "renders a blank page with no error",
    ).toHaveBeenCalledTimes(1);
    vi.doUnmock("./index");
  });
});
