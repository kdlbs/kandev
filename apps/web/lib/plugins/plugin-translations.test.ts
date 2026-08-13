import { afterEach, describe, expect, it } from "vitest";
import { i18n } from "@/lib/i18n";
import {
  pluginTranslationNamespace,
  registerPluginTranslations,
  unregisterPluginTranslations,
} from "./plugin-translations";

const PLUGIN_ID = "translation-test";

afterEach(() => unregisterPluginTranslations(PLUGIN_ID));

describe("plugin translations", () => {
  it("isolates catalogs by plugin and removes them on revocation", () => {
    const namespace = pluginTranslationNamespace(PLUGIN_ID);
    registerPluginTranslations(PLUGIN_ID, { en: { greeting: "Hello {{name}}" } });

    expect(i18n.t(`${namespace}:greeting`, { name: "Ada" })).toBe("Hello Ada");
    unregisterPluginTranslations(PLUGIN_ID);
    expect(i18n.hasResourceBundle("en", namespace)).toBe(false);
  });

  it("rejects unsupported locales and unsafe keys", () => {
    expect(() =>
      registerPluginTranslations(PLUGIN_ID, {
        en: { greeting: "Hello" },
        fr: { greeting: "Bonjour" },
      }),
    ).toThrow("unsupported translation locale");
    expect(() =>
      registerPluginTranslations(PLUGIN_ID, { en: { "__proto__.polluted": "unsafe" } }),
    ).toThrow("invalid translation message");
  });

  it("requires an English fallback catalog", () => {
    expect(() => registerPluginTranslations(PLUGIN_ID, { "pt-pt": { greeting: "Olá" } })).toThrow(
      "English fallback",
    );
  });

  it("keeps the active catalog when a replacement is invalid", () => {
    const namespace = pluginTranslationNamespace(PLUGIN_ID);
    registerPluginTranslations(PLUGIN_ID, { en: { greeting: "Hello" } });

    expect(() => registerPluginTranslations(PLUGIN_ID, { en: { "unsafe.key": "broken" } })).toThrow(
      "invalid translation message",
    );
    expect(i18n.t(`${namespace}:greeting`)).toBe("Hello");
  });
});
