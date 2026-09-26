import { render } from "@testing-library/react";
import i18next from "i18next";
import { I18nextProvider, initReactI18next, useTranslation } from "react-i18next";
import { describe, expect, it } from "vitest";

/**
 * `initI18n()` is fire-and-forget: `ensureInitialized` calls `i18next.init()`
 * without awaiting it and immediately flips its `initialized` flag. That is only
 * safe because init with INLINE resources and NO backend completes
 * synchronously — if it did not, `I18nProvider` would mount before the instance
 * was ready, `useTranslation()` would suspend, and with no Suspense boundary
 * above the React root the tree would never commit: a blank page with no console
 * error, no page error and no unhandled rejection.
 *
 * This was raised in review on the grounds that i18next's `initAsync` defaults
 * to `true`. That option governs when a BACKEND's resource loading is scheduled;
 * with resources passed inline there is nothing to load. These tests pin the
 * behaviour we actually depend on, so a dependency bump fails here instead of
 * shipping a blank app.
 *
 * Catalogs ARE lazy now — every locale but `en` is a fetched chunk — and this
 * guarantee survives it precisely because that move kept `en` inline and added
 * the rest through `addResourceBundle`. `resources` is still populated and there
 * is still no backend, which is the shape asserted below. Registering a locale
 * later does not make init asynchronous; passing catalogs through an i18next
 * BACKEND would, and that is the change these tests exist to stop.
 *
 * A fresh instance is used deliberately: `vitest.setup.ts` pre-initializes the
 * shared one, so asserting against it would be vacuous.
 */

const RESOURCES = { en: { common: { hi: "Hello" } } };

function freshInstance() {
  const instance = i18next.createInstance();
  void instance.use(initReactI18next).init({
    lng: "en",
    fallbackLng: "en",
    defaultNS: "common",
    ns: ["common"],
    resources: RESOURCES,
    interpolation: { escapeValue: false },
    returnNull: false,
  });
  return instance;
}

function Label() {
  const { t } = useTranslation();
  return <span data-testid="label">{t("common:hi")}</span>;
}

describe("i18next initializes synchronously with inline resources", () => {
  it("is initialized and translating before init() is awaited", () => {
    const instance = freshInstance();
    // Read immediately — no await, no tick.
    expect(instance.isInitialized).toBe(true);
    expect(instance.t("common:hi")).toBe("Hello");
  });

  it("does not suspend when rendered with no Suspense boundary", () => {
    const instance = freshInstance();
    // No <Suspense> anywhere in the tree, mirroring the real app root. If
    // useTranslation suspended, this render would throw instead of committing.
    const { getByTestId } = render(
      <I18nextProvider i18n={instance}>
        <Label />
      </I18nextProvider>,
    );
    expect(getByTestId("label").textContent).toBe("Hello");
  });
});
