import i18next from "i18next";
import { initReactI18next } from "react-i18next";

import { writeLocaleCookie } from "./cookie";

/**
 * i18n runtime for the Kandev web SPA (i18next).
 *
 * `en` is the source locale; `pseudo` is a QA-only accented/padded locale used
 * to visually prove every string was externalized (any plain-ASCII text on
 * screen under `pseudo` is a literal that was never routed through `t`).
 * `pseudo` is filtered out of the language switcher in production builds, but
 * the runtime can still activate it if a `kandev_locale=pseudo` cookie is set.
 *
 * Catalogs live at `src/locales/<locale>/<namespace>.json` and are loaded
 * eagerly per locale via Vite's glob import, so a locale switch needs no
 * network round-trip.
 */
export const DEFAULT_LOCALE = "en";
export const DEFAULT_NAMESPACE = "common";
export const SUPPORTED_LOCALES = ["en", "pseudo"] as const;
export type SupportedLocale = (typeof SUPPORTED_LOCALES)[number];

/** Human-readable labels for the language switcher. */
export const LOCALE_LABELS: Record<SupportedLocale, string> = {
  en: "English",
  pseudo: "Pseudo (QA)",
};

export function isSupportedLocale(value: unknown): value is SupportedLocale {
  return typeof value === "string" && (SUPPORTED_LOCALES as readonly string[]).includes(value);
}

/** Coerce any value to a supported locale, defaulting to `en`. */
export function normalizeLocale(value: unknown): SupportedLocale {
  return isSupportedLocale(value) ? value : DEFAULT_LOCALE;
}

/**
 * Locales offered in the language switcher. The `pseudo` QA locale is hidden in
 * production builds.
 */
/**
 * Locales offered by the switcher. The pseudo QA locale is hidden in production.
 *
 * Takes "is this a production build?" from the caller rather than reading
 * `import.meta.env.PROD`: the e2e harness serves a production bundle, so that
 * constant is true there too and would hide the locale the QA tests need. The
 * boot payload's `nonProduction` flag is the authoritative signal.
 */
export function selectableLocales(isProd: boolean): SupportedLocale[] {
  return SUPPORTED_LOCALES.filter((locale) => locale !== "pseudo" || !isProd);
}

type CatalogModule = Record<string, unknown>;

// Vite resolves this at build time; each entry is `../../src/locales/<locale>/<ns>.json`.
const catalogModules = import.meta.glob<CatalogModule>("../../src/locales/*/*.json", {
  eager: true,
  import: "default",
});

function localeResources(locale: SupportedLocale): Record<string, CatalogModule> {
  const resources: Record<string, CatalogModule> = {};
  for (const [path, messages] of Object.entries(catalogModules)) {
    const match = /\/locales\/([^/]+)\/([^/]+)\.json$/.exec(path);
    if (!match) continue;
    const [, fileLocale, namespace] = match;
    if (fileLocale === locale) resources[namespace] = messages;
  }
  return resources;
}

/** Namespaces present in the source catalog; used to seed i18next. */
export function knownNamespaces(): string[] {
  const names = new Set<string>([DEFAULT_NAMESPACE]);
  for (const path of Object.keys(catalogModules)) {
    const match = /\/locales\/[^/]+\/([^/]+)\.json$/.exec(path);
    if (match) names.add(match[1]);
  }
  return [...names];
}

let initialized = false;

function ensureInitialized(locale: SupportedLocale) {
  if (initialized) return;
  void i18next.use(initReactI18next).init({
    lng: locale,
    fallbackLng: DEFAULT_LOCALE,
    defaultNS: DEFAULT_NAMESPACE,
    ns: knownNamespaces(),
    resources: Object.fromEntries(
      SUPPORTED_LOCALES.map((candidate) => [candidate, localeResources(candidate)]),
    ),
    interpolation: {
      // React already escapes rendered output; double-escaping mangles copy.
      escapeValue: false,
    },
    // Missing keys fall back to the key itself rather than throwing, so a
    // missed extraction degrades to visible-but-wrong instead of a crash.
    returnNull: false,
  });
  initialized = true;
}

/**
 * Activate a locale: switch i18next, reflect it on `<html lang>`, and persist
 * the cookie. Unknown locales coerce to `en`. Returns the locale activated.
 */
export async function activateLocale(locale: string): Promise<SupportedLocale> {
  const normalized = normalizeLocale(locale);
  ensureInitialized(normalized);
  if (i18next.language !== normalized) {
    await i18next.changeLanguage(normalized);
  }
  if (typeof document !== "undefined") {
    document.documentElement.lang = normalized;
  }
  writeLocaleCookie(normalized);
  return normalized;
}

/**
 * Initialize the shared instance for the app. Idempotent.
 *
 * This must run before the first `useTranslation()`: react-i18next suspends on
 * an uninitialized instance, and there is no Suspense boundary above the React
 * root, so the tree never commits — a blank page with no error and no rejection.
 * `I18nProvider` calls this at module load; nothing else may be relied on to.
 */
export function initI18n(locale: SupportedLocale): void {
  ensureInitialized(locale);
}

/**
 * Initialize i18next synchronously for unit tests. Tests never render the app
 * shell, so nothing else would set the instance up; react-i18next's
 * `useTranslation` then resolves against this default instance with no provider
 * required. Not used by the app itself (see `activateLocale`).
 */
export function initI18nForTests(locale: SupportedLocale = DEFAULT_LOCALE): void {
  ensureInitialized(locale);
}

/**
 * Translate outside React (plain helpers, `.ts` modules). Inside components
 * prefer `useTranslation()` so the tree re-renders on a locale switch.
 */
export function t(key: string, options?: Record<string, unknown>): string {
  ensureInitialized(DEFAULT_LOCALE);
  return i18next.t(key, options) as string;
}

export { i18next as i18n };
