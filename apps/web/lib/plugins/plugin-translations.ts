import { i18n, SUPPORTED_LOCALES } from "@/lib/i18n";
import type { PluginTranslationCatalogs } from "./types";

const MESSAGE_KEY = /^[a-z][a-zA-Z0-9_-]*$/;
const MAX_MESSAGES_PER_LOCALE = 1_000;
const MAX_MESSAGE_LENGTH = 4_096;

const registeredLocales = new Map<string, Set<string>>();

export function pluginTranslationNamespace(pluginId: string): string {
  return `plugin-${pluginId}`;
}

export function registerPluginTranslations(
  pluginId: string,
  catalogs: PluginTranslationCatalogs,
): void {
  if (!Object.hasOwn(catalogs, "en")) {
    throw new Error("[plugins] translation catalogs require an English fallback");
  }
  const normalizedCatalogs = normalizeCatalogs(catalogs);
  unregisterPluginTranslations(pluginId);
  const namespace = pluginTranslationNamespace(pluginId);
  const locales = new Set<string>();
  for (const [locale, catalog] of normalizedCatalogs) {
    i18n.addResourceBundle(locale, namespace, catalog, false, true);
    locales.add(locale);
  }
  registeredLocales.set(pluginId, locales);
}

function normalizeCatalogs(
  catalogs: PluginTranslationCatalogs,
): Map<string, Record<string, string>> {
  const normalizedCatalogs = new Map<string, Record<string, string>>();
  for (const [locale, catalog] of Object.entries(catalogs)) {
    if (!(SUPPORTED_LOCALES as readonly string[]).includes(locale)) {
      throw new Error(`[plugins] unsupported translation locale "${locale}"`);
    }
    const entries = Object.entries(catalog);
    if (entries.length > MAX_MESSAGES_PER_LOCALE) {
      throw new Error(`[plugins] translation catalog "${locale}" is too large`);
    }
    const normalized: Record<string, string> = {};
    for (const [key, message] of entries) {
      if (
        !MESSAGE_KEY.test(key) ||
        typeof message !== "string" ||
        message.length > MAX_MESSAGE_LENGTH
      ) {
        throw new Error(`[plugins] invalid translation message "${locale}:${key}"`);
      }
      normalized[key] = message;
    }
    normalizedCatalogs.set(locale, normalized);
  }
  return normalizedCatalogs;
}

export function unregisterPluginTranslations(pluginId: string): boolean {
  const locales = registeredLocales.get(pluginId);
  if (!locales) return false;
  const namespace = pluginTranslationNamespace(pluginId);
  locales.forEach((locale) => i18n.removeResourceBundle(locale, namespace));
  registeredLocales.delete(pluginId);
  return true;
}
